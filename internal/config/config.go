// Package config persists user settings to ~/.k10s/config.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AI struct {
	Provider string `yaml:"provider"` // "openai" | "anthropic"
	BaseURL  string `yaml:"base_url"`
	Model    string `yaml:"model"`
	APIKey   string `yaml:"api_key"`
}

type Config struct {
	Theme     string `yaml:"theme"`
	Context   string `yaml:"context"`
	Namespace string `yaml:"namespace"`
	// CLI is the command name shown in copyable hints and command echoes —
	// "kubectl", "k8s", "k", or anything the user prefers.
	CLI string `yaml:"cli"`
	// CLIs is every name k10s should recognise when you type a command at
	// the prompt. All three presets are enabled by default, because people
	// alias them interchangeably.
	CLIs []string `yaml:"clis"`
	// Onboarded records that the first-run setup has been completed, so it
	// isn't shown again even if every other value is left at its default.
	Onboarded bool `yaml:"onboarded"`
	AI        AI   `yaml:"ai"`
}

// DefaultCLI is used until the user picks one during onboarding.
const DefaultCLI = "kubectl"

// CLIPresets are the choices offered by onboarding and the settings modal.
var CLIPresets = []string{"kubectl", "k8s", "k"}

// Path returns the config file location: $K10S_CONFIG if set,
// otherwise ~/.k10s/config.yaml.
func Path() string {
	if p := os.Getenv("K10S_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".k10s", "config.yaml")
}

// Load reads the config file; a missing file returns zero values, nil error.
func Load() (Config, error) {
	var c Config
	p := Path()
	if p == "" {
		return c, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	parse(string(b), &c)
	return c, nil
}

// Save writes the config file (0600 — it can hold an API key), creating
// ~/.k10s if needed.
func Save(c Config) error {
	p := Path()
	if p == "" {
		return fmt.Errorf("cannot resolve config path")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(render(c)), 0o600)
}

// The format is a flat, quoted subset of YAML — written and read by us only,
// so a hand-rolled parser keeps the dependency tree clean.

func render(c Config) string {
	var b strings.Builder
	b.WriteString("# k10s configuration — edited in-app via /config, T, /ns, /context\n")
	fmt.Fprintf(&b, "theme: %q\n", c.Theme)
	fmt.Fprintf(&b, "context: %q\n", c.Context)
	fmt.Fprintf(&b, "namespace: %q\n", c.Namespace)
	fmt.Fprintf(&b, "cli: %q\n", c.CLI)
	fmt.Fprintf(&b, "clis: %q\n", strings.Join(c.CLIs, ","))
	fmt.Fprintf(&b, "onboarded: %v\n", c.Onboarded)
	b.WriteString("ai:\n")
	fmt.Fprintf(&b, "  provider: %q\n", c.AI.Provider)
	fmt.Fprintf(&b, "  base_url: %q\n", c.AI.BaseURL)
	fmt.Fprintf(&b, "  model: %q\n", c.AI.Model)
	fmt.Fprintf(&b, "  api_key: %q\n", c.AI.APIKey)
	return b.String()
}

func parse(s string, c *Config) {
	inAI := false
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") || strings.TrimSpace(line) == "" {
			continue
		}
		indented := strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")
		key, val, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if v, err := unquote(val); err == nil {
			val = v
		}
		if !indented {
			inAI = key == "ai"
		}
		switch {
		case !indented && key == "theme":
			c.Theme = val
		case !indented && key == "context":
			c.Context = val
		case !indented && key == "namespace":
			c.Namespace = val
		case !indented && key == "cli":
			c.CLI = val
		case !indented && key == "clis":
			c.CLIs = nil
			for _, p := range strings.Split(val, ",") {
				if p = strings.TrimSpace(p); p != "" {
					c.CLIs = append(c.CLIs, p)
				}
			}
		case !indented && key == "onboarded":
			c.Onboarded = val == "true"
		case indented && inAI:
			switch key {
			case "provider":
				c.AI.Provider = val
			case "base_url":
				c.AI.BaseURL = val
			case "model":
				c.AI.Model = val
			case "api_key":
				c.AI.APIKey = val
			}
		}
	}
}

func unquote(s string) (string, error) {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(s[1 : len(s)-1]), nil
	}
	return s, fmt.Errorf("not quoted")
}
