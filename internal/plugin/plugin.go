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

func isPOSIXShell(command string) bool {
	switch strings.ToLower(filepath.Base(command)) {
	case "sh", "bash", "zsh", "dash", "ksh":
		return true
	default:
		return false
	}
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
// remains available by configuring command: sh and args: [-c, ...], as in k9s.
func (p Plugin) ExecCommand(vars Vars) *exec.Cmd {
	args := make([]string, len(p.Args))
	shellCode := -1
	if isPOSIXShell(p.Command) {
		for i, arg := range p.Args {
			// POSIX shells accept -c and grouped short flags such as -lc.
			// Long options (--norc, --rcfile) are not short flag groups even
			// when their spelling happens to contain the letter c.
			shortFlags := len(arg) >= 2 && arg[0] == '-' && arg[1] != '-'
			if shortFlags && strings.Contains(arg[1:], "c") && i+1 < len(p.Args) {
				shellCode = i + 1
				break
			}
		}
	}
	for i, arg := range p.Args {
		if i == shellCode {
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
