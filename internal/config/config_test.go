package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	t.Setenv("K10S_CONFIG", filepath.Join(t.TempDir(), "does-not-exist.yaml"))

	c, err := Load()
	if err != nil {
		t.Fatalf("Load of missing file: %v", err)
	}
	if c.Theme != "" || c.Context != "" || c.AI.APIKey != "" {
		t.Errorf("missing file should yield zero values, got %+v", c)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("K10S_CONFIG", path)

	want := Config{
		Theme:     "tokyo-night",
		Context:   "kind-dev",
		Namespace: "kube-system",
		CLI:       "k",
		CLIs:      []string{"kubectl", "k8s", "k"},
		Onboarded: true,
		AI: AI{
			Provider: "anthropic",
			BaseURL:  "https://api.anthropic.com/v1",
			Model:    "claude-sonnet-5",
			APIKey:   "sk-ant-secret",
		},
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestSaveUsesOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("K10S_CONFIG", path)

	if err := Save(Config{AI: AI{APIKey: "sk-secret"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// The file can hold an API key, so it must not be group/world readable.
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("config file mode = %o, want 600", perm)
	}
}

func TestRoundTripPreservesValuesNeedingQuoting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("K10S_CONFIG", path)

	want := Config{
		// Colons and quotes are exactly what a naive line-splitting parser
		// gets wrong, and context names / URLs routinely contain colons.
		Context: "https://k8s.example.com:6443",
		Theme:   `weird "quoted" name`,
		AI:      AI{BaseURL: "http://localhost:11434/v1"},
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Context != want.Context {
		t.Errorf("Context = %q, want %q", got.Context, want.Context)
	}
	if got.Theme != want.Theme {
		t.Errorf("Theme = %q, want %q", got.Theme, want.Theme)
	}
	if got.AI.BaseURL != want.AI.BaseURL {
		t.Errorf("AI.BaseURL = %q, want %q", got.AI.BaseURL, want.AI.BaseURL)
	}
}
