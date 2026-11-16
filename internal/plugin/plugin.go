// Package plugin loads and prepares k9s-compatible command plugins.
package plugin

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/p10node/k10s/internal/config"
	"sigs.k8s.io/yaml"
)

// Plugin is the k9s plugins.yaml command shape supported by k10s.
type Plugin struct {
	Scopes      []string `yaml:"scopes" json:"scopes"`
	Args        []string `yaml:"args" json:"args"`
	ShortCut    string   `yaml:"shortCut" json:"shortCut"`
	Override    bool     `yaml:"override" json:"override"`
	Description string   `yaml:"description" json:"description"`
	Command     string   `yaml:"command" json:"command"`
	Confirm     bool     `yaml:"confirm" json:"confirm"`
	Background  bool     `yaml:"background" json:"background"`
	Dangerous   bool     `yaml:"dangerous" json:"dangerous"`
}

// Named retains the map key used to identify a plugin in diagnostics.
type Named struct {
	Name string
	Plugin
}

type fileSet struct {
	Plugins map[string]Plugin `yaml:"plugins" json:"plugins"`
}

// Vars are the k9s-compatible values available to command arguments.
type Vars struct {
	Name            string
	Namespace       string
	Context         string
	ResourceGroup   string
	ResourceVersion string
	ResourceName    string
	Container       string
	Filter          string
	Cluster         string
	Kubeconfig      string
	User            string
	Groups          string
	Pod             string
	Columns         map[string]string
}

// Path is the main plugin file. It sits beside config.yaml unless overridden.
func Path() string {
	if path := os.Getenv("K10S_PLUGINS"); path != "" {
		return path
	}
	if path := config.Path(); path != "" {
		return filepath.Join(filepath.Dir(path), "plugins.yaml")
	}
	return ""
}

// Dir is the recursively scanned plugin snippet directory.
func Dir() string {
	if path := os.Getenv("K10S_PLUGIN_DIR"); path != "" {
		return path
	}
	if path := Path(); path != "" {
		return filepath.Join(filepath.Dir(path), "plugins")
	}
	return ""
}

// Load merges plugins.yaml with every .yaml/.yml file under plugins/.
// Directory snippets override plugins with the same name from the main file.
func Load() ([]Named, error) {
	all := map[string]Plugin{}
	var errs error
	if path := Path(); path != "" {
		if err := loadFile(path, all); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if dir := Dir(); dir != "" {
		var fileErrs error
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
				return nil
			}
			if err := loadFile(path, all); err != nil {
				fileErrs = errors.Join(fileErrs, err)
			}
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = errors.Join(errs, err)
		}
		errs = errors.Join(errs, fileErrs)
	}

	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Named, 0, len(names))
	for _, name := range names {
		out = append(out, Named{Name: name, Plugin: all[name]})
	}
	return out, errs
}

func loadFile(path string, dst map[string]Plugin) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("plugins %s: %w", path, err)
	}

	var envelope map[string]any
	if err := yaml.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("plugins %s: %w", path, err)
	}
	_, hasPluginsKey := envelope["plugins"]

	var wrapped fileSet
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return fmt.Errorf("plugins %s: %w", path, err)
	}
	found := wrapped.Plugins
	if !hasPluginsKey {
		var single Plugin
		if err := yaml.Unmarshal(data, &single); err != nil {
			return fmt.Errorf("plugins %s: %w", path, err)
		}
		if single.ShortCut != "" || single.Command != "" || len(single.Scopes) > 0 {
			found = map[string]Plugin{strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)): single}
		} else {
			if err := yaml.Unmarshal(data, &found); err != nil {
				return fmt.Errorf("plugins %s: %w", path, err)
			}
		}
	}
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	// Validate the whole file before changing dst. Otherwise Go's randomized
	// map iteration could merge a different valid subset before encountering
	// a bad entry on each startup.
	for _, name := range names {
		item := found[name]
		if err := item.validate(); err != nil {
			return fmt.Errorf("plugin %q in %s: %w", name, path, err)
		}
	}
	for _, name := range names {
		item := found[name]
		dst[name] = item
	}
	return nil
}

func (p Plugin) validate() error {
	switch {
	case strings.TrimSpace(p.ShortCut) == "":
		return errors.New("shortCut is required")
	case len(p.Scopes) == 0:
		return errors.New("at least one scope is required")
	case strings.TrimSpace(p.Command) == "":
		return errors.New("command is required")
	}
	return nil
}

// NormalizeShortcut converts k9s notation to bubbletea KeyMsg.String notation.
func NormalizeShortcut(shortcut string) string {
	shortcut = strings.TrimSpace(shortcut)
	parts := strings.FieldsFunc(shortcut, func(r rune) bool { return r == '-' || r == '+' || r == ' ' })
	if len(parts) == 1 {
		return parts[0]
	}
	if len(parts) != 2 {
		return strings.ToLower(shortcut)
	}
	modifier, key := strings.ToLower(parts[0]), parts[1]
	switch modifier {
	case "ctrl", "control":
		return "ctrl+" + strings.ToLower(key)
	case "alt", "option", "meta":
		return "alt+" + strings.ToLower(key)
	case "shift":
		return strings.ToUpper(key)
	default:
		return strings.ToLower(shortcut)
	}
}

// MatchesScope reports whether any configured scope names the current view.
func (p Plugin) MatchesScope(aliases ...string) bool {
	for _, scope := range p.Scopes {
		for _, alias := range aliases {
			if strings.EqualFold(scope, "all") || strings.EqualFold(scope, alias) {
				return true
			}
		}
	}
	return false
}

// Expand replaces only k10s/k9s plugin variables, leaving shell variables such
// as $HOME untouched for commands that deliberately invoke sh -c.
func Expand(value string, vars Vars) string {
	values := map[string]string{
		"$RESOURCE_GROUP": vars.ResourceGroup, "$RESOURCE_VERSION": vars.ResourceVersion,
		"$RESOURCE_NAME": vars.ResourceName, "$NAMESPACE": vars.Namespace,
		"$NAME": vars.Name, "$CONTAINER": vars.Container, "$FILTER": vars.Filter,
		"$CLUSTER": vars.Cluster, "$CONTEXT": vars.Context,
		"$KUBECONFIG": vars.Kubeconfig, "$USER": vars.User,
		"$GROUPS": vars.Groups, "$POD": vars.Pod,
	}
	for column, columnValue := range vars.Columns {
		values["$COL-"+column] = columnValue
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	pairs := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		pairs = append(pairs, key, values[key])
	}
	return strings.NewReplacer(pairs...).Replace(value)
}

func columnEnvName(column string) string {
	return "K10S_COL_" + strings.ToUpper(hex.EncodeToString([]byte(column)))
}

// programName is how a command is matched against the shells below: its base
// name in lower case, without a Windows .exe.
func programName(command string) string {
	return strings.TrimSuffix(strings.ToLower(filepath.Base(command)), ".exe")
}

func isPOSIXShell(command string) bool {
	switch programName(command) {
	case "sh", "bash", "zsh", "dash", "ksh":
		return true
	default:
		return false
	}
}

// shellStart finds a POSIX shell on a plugin's command line and returns the
// index in args of the shell's own first argument: 0 when the command is the
// shell, 1 for busybox's shell (busybox sh), and after env's options and
// NAME=value arguments for env (env sh, env -i PATH=... sh). env -S is not
// a way to a shell: it builds a command line of its own from a string and
// expands ${VAR} in it, so it is a program that reads its own command line,
// as python3 -c is. unsure is true when env has an option k10s does not
// know, which may hide a shell.
func shellStart(command string, args []string) (start int, shell, unsure bool) {
	switch name := programName(command); {
	case isPOSIXShell(command):
		return 0, true, false
	case name == "busybox":
		if len(args) > 0 {
			switch strings.ToLower(args[0]) {
			case "sh", "ash", "hush", "bash":
				return 1, true, false
			}
		}
	case name == "env":
		i := 0
	options:
		for i < len(args) {
			switch arg := args[i]; {
			case arg == "--":
				i++
				break options
			case arg == "-" || arg == "--ignore-environment":
				i++
			case arg == "--unset" || arg == "--chdir":
				i += 2
			case strings.HasPrefix(arg, "--unset=") || strings.HasPrefix(arg, "--chdir="):
				i++
			case strings.HasPrefix(arg, "--split-string"):
				return 0, false, false
			case strings.HasPrefix(arg, "--"):
				return 0, false, true
			case strings.HasPrefix(arg, "-"):
				// A group of short options, read as getopt does: GNU's and
				// the BSDs' flags, and the options that take a value, which
				// is the rest of the group or else the next argument.
				next := 1
			group:
				for j := 1; j < len(arg); j++ {
					switch arg[j] {
					case 'i', 'v', '0':
					case 'u', 'C', 'P', 'L', 'U':
						if j == len(arg)-1 {
							next = 2
						}
						break group
					case 'S':
						return 0, false, false
					default:
						return 0, false, true
					}
				}
				i += next
			default:
				break options
			}
		}
		for i < len(args) && strings.Contains(args[i], "=") {
			i++
		}
		if i < len(args) {
			start, shell, unsure := shellStart(args[i], args[i+1:])
			return i + 1 + start, shell, unsure
		}
	}
	return 0, false, false
}

// plainOptions are the option letters that take no value in any of the
// shells above.
const plainOptions = "aCefhilmnprsuvx"

// optionLike reports whether a shell reads arg as a group of options.
func optionLike(arg string) bool {
	return len(arg) > 1 && (arg[0] == '-' || arg[0] == '+')
}

// shellScript reads a POSIX shell's arguments the way the shells do and
// returns the index of the script that -c runs: the first argument after the
// options, or -1 when there is no -c. Options are the arguments that start
// with - or +, up to a -- or a lone -, which ends them; c anywhere in a group
// turns on -c, and an o that ends its group takes the next argument as its
// value (-o pipefail, -eo pipefail). So -x, -o errexit or -- after -c come
// before the script, not in its place. unsure is true for an option the
// shells read differently or k10s does not know, such as bash's -O, an o
// inside a group, a lone +, or a long option other than bash's.
func shellScript(args []string) (script int, unsure bool) {
	command := false
	i := 0
options:
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-" || arg == "--":
			i++
			break options
		case arg == "+":
			return -1, true // zsh reads it as --, bash and dash as an empty group
		case strings.HasPrefix(arg, "--"):
			switch arg {
			case "--rcfile", "--init-file":
				if i+1 >= len(args) || optionLike(args[i+1]) {
					return -1, true
				}
				i += 2
			case "--debug", "--debugger", "--dump-po-strings", "--dump-strings",
				"--help", "--login", "--noediting", "--noprofile", "--norc",
				"--posix", "--pretty-print", "--restricted", "--verbose", "--version":
				i++
			default:
				return -1, true
			}
		case optionLike(arg):
			group := arg[1:]
			for j := 0; j < len(group); j++ {
				switch letter := group[j]; {
				case letter == 'c':
					command = true
				case letter == 'o' && j == len(group)-1:
					if i+1 >= len(args) || optionLike(args[i+1]) {
						return -1, true
					}
					i++
				case strings.IndexByte(plainOptions, letter) < 0:
					return -1, true
				}
			}
			i++
		default:
			break options
		}
	}
	if !command || i >= len(args) {
		return -1, false
	}
	return i, false
}

// shellCode marks the arguments that a POSIX shell on the plugin's command
// line may run as code: the -c script, and the options before it. Those get
// plugin values through the environment, never as text. When k10s cannot
// tell which argument is the script, every argument is marked; a value meant
// for an argument after the script, which the shell passes as $0, $1 and so
// on, then arrives as the variable's name, which is safe if not what was
// meant.
func shellCode(command string, args []string) []bool {
	code := make([]bool, len(args))
	start, shell, unsure := shellStart(command, args)
	last := -1
	switch {
	case unsure:
		start, last = 0, len(args)-1
	case shell:
		script, unsure := shellScript(args[start:])
		switch {
		case unsure:
			last = len(args) - 1
		case script >= 0:
			last = start + script
		}
	}
	for i := start; i <= last; i++ {
		code[i] = true
	}
	return code
}

// expandShellColumns maps k9s's $COL-NAME syntax (not a valid POSIX variable
// name) to private environment variables. Other plugin variables stay intact
// and are expanded by the shell from the environment. Shell expansion does
// not reparse command substitutions or metacharacters contained in a value.
func expandShellColumns(value string, vars Vars) string {
	keys := make([]string, 0, len(vars.Columns))
	for column := range vars.Columns {
		keys = append(keys, column)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	pairs := make([]string, 0, len(keys)*2)
	for _, column := range keys {
		pairs = append(pairs, "$COL-"+column, "${"+columnEnvName(column)+"}")
	}
	return strings.NewReplacer(pairs...).Replace(value)
}

// DisplayCommand returns a quoted, fully resolved command for confirmations.
// It is informational only; ExecCommand still passes an argv slice directly.
func (p Plugin) DisplayCommand(vars Vars) string {
	parts := make([]string, 0, len(p.Args)+1)
	parts = append(parts, strconv.Quote(Expand(p.Command, vars)))
	for _, arg := range p.Args {
		parts = append(parts, strconv.Quote(Expand(arg, vars)))
	}
	return strings.Join(parts, " ")
}

// ExecCommand constructs a process without involving a shell. Shell behavior
// remains available by configuring command: sh and args: [-c, ...], as in k9s;
// shellCode decides which arguments get plugin values through the environment.
func (p Plugin) ExecCommand(vars Vars) *exec.Cmd {
	code := shellCode(p.Command, p.Args)
	args := make([]string, len(p.Args))
	for i, arg := range p.Args {
		if code[i] {
			args[i] = expandShellColumns(arg, vars)
		} else {
			args[i] = Expand(arg, vars)
		}
	}
	cmd := exec.Command(Expand(p.Command, vars), args...)
	cmd.Env = append(os.Environ(),
		"NAME="+vars.Name,
		"NAMESPACE="+vars.Namespace,
		"CONTEXT="+vars.Context,
		"CLUSTER="+vars.Cluster,
		"RESOURCE_GROUP="+vars.ResourceGroup,
		"RESOURCE_VERSION="+vars.ResourceVersion,
		"RESOURCE_NAME="+vars.ResourceName,
		"CONTAINER="+vars.Container,
		"FILTER="+vars.Filter,
		"KUBECONFIG="+vars.Kubeconfig,
		"USER="+vars.User,
		"GROUPS="+vars.Groups,
		"POD="+vars.Pod,
	)
	for column, value := range vars.Columns {
		cmd.Env = append(cmd.Env, columnEnvName(column)+"="+value)
	}
	return cmd
}
