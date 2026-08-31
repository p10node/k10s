package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDirAddsCustomTheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rose-pine.yaml")
	data := []byte(`name: rose-pine
bg: "#191724"
fg: "#e0def4"
subtle: "#6e6a86"
border: "#403d52"
border_on: "#c4a7e7"
accent: "#c4a7e7"
accent2: "#ebbcba"
ok: "#9ccfd8"
warn: "#f6c177"
err: "#eb6f92"
sel_bg: "#26233a"
sel_fg: "#e0def4"
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("LoadDir returned %d themes, want 1", len(got))
	}
	if got[0].Name != "rose-pine" {
		t.Errorf("theme name = %q, want rose-pine", got[0].Name)
	}
	if string(got[0].Accent) != "#c4a7e7" {
		t.Errorf("accent = %q, want #c4a7e7", got[0].Accent)
	}
}

func TestLoadDirKeepsValidThemesAndReportsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	valid := `name: valid
bg: "#111111"
fg: "#eeeeee"
subtle: "#777777"
border: "#333333"
border_on: "#8888ff"
accent: "#8888ff"
accent2: "#ff88ff"
ok: "#88ff88"
warn: "#ffff88"
err: "#ff8888"
sel_bg: "#222244"
sel_fg: "#ffffff"
`
	if err := os.WriteFile(filepath.Join(dir, "a-valid.yaml"), []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b-broken.yaml"), []byte("name: broken\nbg: '#000000'\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadDir(dir)
	if len(got) != 1 || got[0].Name != "valid" {
		t.Fatalf("LoadDir returned %+v, want the valid theme", got)
	}
	if err == nil || !strings.Contains(err.Error(), "b-broken.yaml") {
		t.Fatalf("error = %v, want it to identify b-broken.yaml", err)
	}
}

func TestLoadIncludesThemesNextToConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("K10S_CONFIG", filepath.Join(root, "config.yaml"))
	themesDir := filepath.Join(root, "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `name: custom-blue
bg: "#101828"
fg: "#f2f4f7"
subtle: "#98a2b3"
border: "#344054"
border_on: "#53b1fd"
accent: "#53b1fd"
accent2: "#b692f6"
ok: "#32d583"
warn: "#fec84b"
err: "#f97066"
sel_bg: "#1d2939"
sel_fg: "#ffffff"
`
	if err := os.WriteFile(filepath.Join(themesDir, "custom-blue.yaml"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != len(Themes)+1 {
		t.Fatalf("Load returned %d themes, want %d", len(got), len(Themes)+1)
	}
	if got[len(got)-1].Name != "custom-blue" {
		t.Errorf("last theme = %q, want custom-blue", got[len(got)-1].Name)
	}
}

func TestLoadRejectsCustomThemeThatShadowsBuiltIn(t *testing.T) {
	root := t.TempDir()
	t.Setenv("K10S_CONFIG", filepath.Join(root, "config.yaml"))
	themesDir := filepath.Join(root, "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `name: tokyo-night
bg: "#111111"
fg: "#eeeeee"
subtle: "#777777"
border: "#333333"
border_on: "#8888ff"
accent: "#8888ff"
accent2: "#ff88ff"
ok: "#88ff88"
warn: "#ffff88"
err: "#ff8888"
sel_bg: "#222244"
sel_fg: "#ffffff"
`
	if err := os.WriteFile(filepath.Join(themesDir, "shadow.yaml"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if len(got) != len(Themes) {
		t.Fatalf("Load returned %d themes, want only %d built-ins", len(got), len(Themes))
	}
	if err == nil || !strings.Contains(err.Error(), "tokyo-night") {
		t.Fatalf("error = %v, want duplicate name tokyo-night", err)
	}
}

func TestLoadDirRejectsInvalidColor(t *testing.T) {
	dir := t.TempDir()
	data := `name: bad-color
bg: "blue-ish"
fg: "#eeeeee"
subtle: "#777777"
border: "#333333"
border_on: "#8888ff"
accent: "#8888ff"
accent2: "#ff88ff"
ok: "#88ff88"
warn: "#ffff88"
err: "#ff8888"
sel_bg: "#222244"
sel_fg: "#ffffff"
`
	if err := os.WriteFile(filepath.Join(dir, "bad-color.yaml"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadDir(dir)
	if len(got) != 0 {
		t.Fatalf("LoadDir accepted invalid theme: %+v", got)
	}
	if err == nil || !strings.Contains(err.Error(), "bg") {
		t.Fatalf("error = %v, want invalid bg", err)
	}
}
