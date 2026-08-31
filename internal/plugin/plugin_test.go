package plugin

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadSupportsK9sPluginsFileAndDirectoryForms(t *testing.T) {
	root := t.TempDir()
	t.Setenv("K10S_PLUGINS", filepath.Join(root, "plugins.yaml"))
	t.Setenv("K10S_PLUGIN_DIR", filepath.Join(root, "plugins"))

	file := `plugins:
  logs:
    shortCut: Ctrl-L
    description: Pod logs
    scopes: [po]
    command: kubectl
    args: [logs, -f, $NAME]
`
	if err := os.WriteFile(filepath.Join(root, "plugins.yaml"), []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plugins", "misc"), 0o755); err != nil {
		t.Fatal(err)
	}
	dirFile := `restart:
  shortCut: Shift-R
  description: Restart
  scopes: [deploy]
  command: kubectl
  args: [rollout, restart, deployment/$NAME]
`
	if err := os.WriteFile(filepath.Join(root, "plugins", "misc", "workloads.yaml"), []byte(dirFile), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if names := []string{got[0].Name, got[1].Name}; !reflect.DeepEqual(names, []string{"logs", "restart"}) {
		t.Fatalf("plugin names = %v, want deterministic name order", names)
	}
	if got[0].ShortCut != "Ctrl-L" || got[0].Args[2] != "$NAME" {
		t.Errorf("k9s fields were not preserved: %+v", got[0])
	}
}

func TestLoadRejectsIncompletePlugin(t *testing.T) {
	root := t.TempDir()
	t.Setenv("K10S_PLUGINS", filepath.Join(root, "plugins.yaml"))
	t.Setenv("K10S_PLUGIN_DIR", filepath.Join(root, "plugins"))
	if err := os.WriteFile(filepath.Join(root, "plugins.yaml"), []byte("plugins:\n  broken:\n    shortCut: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a plugin without command or scope")
	}
}

func TestLoadAcceptsEmptyWrappedPluginDocuments(t *testing.T) {
	for _, document := range []string{"plugins: {}\n", "plugins:\n"} {
		t.Run(strings.TrimSpace(document), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "plugins.yaml")
			t.Setenv("K10S_PLUGINS", path)
			t.Setenv("K10S_PLUGIN_DIR", filepath.Join(root, "none"))
			if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := Load()
			if err != nil {
				t.Fatalf("Load empty wrapped document: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("plugins = %+v, want none", got)
			}
		})
	}
}

func TestLoadKeepsValidDirectoryPluginsWhenSiblingIsInvalid(t *testing.T) {
	root := t.TempDir()
	t.Setenv("K10S_PLUGINS", filepath.Join(root, "missing.yaml"))
	dir := filepath.Join(root, "plugins")
	t.Setenv("K10S_PLUGIN_DIR", dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a-broken.yaml"), []byte("shortCut: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := "shortCut: Ctrl-L\nscopes: [po]\ncommand: kubectl\n"
	if err := os.WriteFile(filepath.Join(dir, "z-valid.yaml"), []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err == nil {
		t.Fatal("Load should report the malformed sibling")
	}
	if len(got) != 1 || got[0].Name != "z-valid" {
		t.Fatalf("plugins = %+v, want the valid sibling despite the error", got)
	}
}

func TestLoadDoesNotPartiallyMergeAFileContainingInvalidPlugin(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plugins.yaml")
	t.Setenv("K10S_PLUGINS", path)
	t.Setenv("K10S_PLUGIN_DIR", filepath.Join(root, "none"))
	data := `plugins:
  valid:
    shortCut: x
    scopes: [all]
    command: printf
  invalid:
    shortCut: y
    scopes: [all]
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 25; i++ {
		got, err := Load()
		if err == nil {
			t.Fatal("Load should report the invalid entry")
		}
		if len(got) != 0 {
			t.Fatalf("iteration %d partially merged invalid file: %+v", i, got)
		}
	}
}

func TestShortcutNormalizesK9sNotation(t *testing.T) {
	cases := map[string]string{
		"Ctrl-L":  "ctrl+l",
		"Shift-R": "R",
		"Alt-X":   "alt+x",
		"x":       "x",
	}
	for in, want := range cases {
		if got := NormalizeShortcut(in); got != want {
			t.Errorf("NormalizeShortcut(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandReplacesK9sVariablesAndColumnValues(t *testing.T) {
	vars := Vars{
		Name: "api-0", Namespace: "prod", Context: "kind-dev",
		ResourceName: "pods", Filter: "api", Kubeconfig: "/tmp/kube config", User: "alice",
		Groups: "devs,ops", Pod: "api-0", Columns: map[string]string{"STATUS": "Running"},
	}
	got := Expand("$NAME $NAMESPACE $CONTEXT $RESOURCE_NAME $FILTER $KUBECONFIG $USER $GROUPS $POD $COL-STATUS $HOME", vars)
	want := "api-0 prod kind-dev pods api /tmp/kube config alice devs,ops api-0 Running $HOME"
	if got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}
}

func TestMatchesScopeAcceptsAllAndResourceAliases(t *testing.T) {
	if !(Plugin{Scopes: []string{"all"}}).MatchesScope("pods", "po") {
		t.Error("all scope should match")
	}
	if !(Plugin{Scopes: []string{"po"}}).MatchesScope("pods", "po", "pod") {
		t.Error("short resource scope should match")
	}
	if (Plugin{Scopes: []string{"svc"}}).MatchesScope("pods", "po") {
		t.Error("unrelated scope matched")
	}
}

func TestShellCommandExpandsPluginValuesWithoutReparsingThem(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	pwned := filepath.Join(dir, "pwned")
	t.Setenv("OUT", out)
	payload := `"; touch ` + pwned + `; # $(touch ignored)`
	p := Plugin{Command: "sh", Args: []string{"-c", `printf '%s' "$COL-STATUS" > "$OUT"`}}

	cmd := p.ExecCommand(Vars{Columns: map[string]string{"STATUS": payload}})
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Errorf("shell received %q, want literal payload %q", got, payload)
	}
	if _, err := os.Stat(pwned); !os.IsNotExist(err) {
		t.Fatalf("cluster value was reparsed as shell syntax; pwned stat err=%v", err)
	}
}

func TestBashLongOptionsDoNotHideShellCodeFromSafeExpansion(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	pwned := filepath.Join(dir, "pwned")
	t.Setenv("OUT", out)
	payload := `"; touch ` + pwned + `; # $(touch ignored)`
	p := Plugin{
		Command: "bash",
		Args:    []string{"--norc", "-c", `printf '%s' "$NAME" > "$OUT"`},
	}

	if err := p.ExecCommand(Vars{Name: payload}).Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Errorf("shell received %q, want literal payload %q", got, payload)
	}
	if _, err := os.Stat(pwned); !os.IsNotExist(err) {
		t.Fatalf("cluster value was reparsed as shell syntax; pwned stat err=%v", err)
	}
}

func TestExecCommandOverridesExistingPluginEnvironmentValues(t *testing.T) {
	t.Setenv("USER", "operating-system-user")
	p := Plugin{Command: "sh", Args: []string{"-c", `printf '%s' "$USER"`}}
	out, err := p.ExecCommand(Vars{User: "kubeconfig-user"}).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "kubeconfig-user" {
		t.Errorf("USER = %q, want kubeconfig identity", out)
	}
}
