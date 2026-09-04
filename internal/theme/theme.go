package theme

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/p10node/k10s/internal/config"
	"sigs.k8s.io/yaml"
)

type Theme struct {
	Name     string
	Bg       lipgloss.Color
	Fg       lipgloss.Color
	Subtle   lipgloss.Color
	Border   lipgloss.Color
	BorderOn lipgloss.Color
	Accent   lipgloss.Color
	Accent2  lipgloss.Color
	Ok       lipgloss.Color
	Warn     lipgloss.Color
	Err      lipgloss.Color
	SelBg    lipgloss.Color
	SelFg    lipgloss.Color
}

func c(s string) lipgloss.Color { return lipgloss.Color(s) }

var (
	validThemeName  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	validThemeColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// Dir returns the custom-theme directory. K10S_THEME_DIR overrides the
// default themes/ directory beside config.yaml.
func Dir() string {
	if dir := os.Getenv("K10S_THEME_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(filepath.Dir(config.Path()), "themes")
}

// Load returns the built-in themes followed by custom themes from Dir.
func Load() ([]Theme, error) {
	themes := append([]Theme(nil), Themes...)
	custom, err := LoadDir(Dir())
	if err != nil && os.IsNotExist(err) {
		return themes, nil
	}
	seen := make(map[string]bool, len(themes)+len(custom))
	for _, t := range themes {
		seen[t.Name] = true
	}
	var duplicateErrs []error
	for _, t := range custom {
		if seen[t.Name] {
			duplicateErrs = append(duplicateErrs, fmt.Errorf("duplicate theme name %q", t.Name))
			continue
		}
		seen[t.Name] = true
		themes = append(themes, t)
	}
	return themes, errors.Join(append([]error{err}, duplicateErrs...)...)
}

type themeFile struct {
	Name     string `json:"name"`
	Bg       string `json:"bg"`
	Fg       string `json:"fg"`
	Subtle   string `json:"subtle"`
	Border   string `json:"border"`
	BorderOn string `json:"border_on"`
	Accent   string `json:"accent"`
	Accent2  string `json:"accent2"`
	Ok       string `json:"ok"`
	Warn     string `json:"warn"`
	Err      string `json:"err"`
	SelBg    string `json:"sel_bg"`
	SelFg    string `json:"sel_fg"`
}

// LoadDir reads custom theme YAML files from dir in filename order.
func LoadDir(dir string) ([]Theme, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var (
		out  []Theme
		errs []error
	)
	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if entry.IsDir() || (ext != ".yaml" && ext != ".yml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		var f themeFile
		if err := yaml.UnmarshalStrict(data, &f); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		if err := f.validate(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		out = append(out, Theme{
			Name: f.Name, Bg: c(f.Bg), Fg: c(f.Fg), Subtle: c(f.Subtle),
			Border: c(f.Border), BorderOn: c(f.BorderOn), Accent: c(f.Accent), Accent2: c(f.Accent2),
			Ok: c(f.Ok), Warn: c(f.Warn), Err: c(f.Err), SelBg: c(f.SelBg), SelFg: c(f.SelFg),
		})
	}
	return out, errors.Join(errs...)
}

func (f themeFile) validate() error {
	fields := map[string]string{
		"name": f.Name, "bg": f.Bg, "fg": f.Fg, "subtle": f.Subtle,
		"border": f.Border, "border_on": f.BorderOn, "accent": f.Accent,
		"accent2": f.Accent2, "ok": f.Ok, "warn": f.Warn, "err": f.Err,
		"sel_bg": f.SelBg, "sel_fg": f.SelFg,
	}
	for name, value := range fields {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("missing required field %q", name)
		}
	}
	if !validThemeName.MatchString(f.Name) {
		return fmt.Errorf("name %q must use lowercase letters, numbers, hyphens, or underscores", f.Name)
	}
	delete(fields, "name")
	for name, value := range fields {
		if !validThemeColor.MatchString(value) {
			return fmt.Errorf("field %q must be a #RRGGBB color, got %q", name, value)
		}
	}
	return nil
}

var Themes = []Theme{
	{
		Name: "tokyo-night",
		Bg:   c("#1a1b26"), Fg: c("#c0caf5"), Subtle: c("#565f89"),
		Border: c("#3b4261"), BorderOn: c("#7aa2f7"),
		Accent: c("#7aa2f7"), Accent2: c("#bb9af7"),
		Ok: c("#9ece6a"), Warn: c("#e0af68"), Err: c("#f7768e"),
		SelBg: c("#283457"), SelFg: c("#c0caf5"),
	},
	{
		Name: "catppuccin-mocha",
		Bg:   c("#1e1e2e"), Fg: c("#cdd6f4"), Subtle: c("#6c7086"),
		Border: c("#45475a"), BorderOn: c("#89b4fa"),
		Accent: c("#89b4fa"), Accent2: c("#cba6f7"),
		Ok: c("#a6e3a1"), Warn: c("#f9e2af"), Err: c("#f38ba8"),
		SelBg: c("#313244"), SelFg: c("#cdd6f4"),
	},
	{
		Name: "dracula",
		Bg:   c("#282a36"), Fg: c("#f8f8f2"), Subtle: c("#6272a4"),
		Border: c("#44475a"), BorderOn: c("#bd93f9"),
		Accent: c("#bd93f9"), Accent2: c("#ff79c6"),
		Ok: c("#50fa7b"), Warn: c("#f1fa8c"), Err: c("#ff5555"),
		SelBg: c("#44475a"), SelFg: c("#f8f8f2"),
	},
	{
		Name: "nord",
		Bg:   c("#2e3440"), Fg: c("#d8dee9"), Subtle: c("#4c566a"),
		Border: c("#3b4252"), BorderOn: c("#88c0d0"),
		Accent: c("#88c0d0"), Accent2: c("#81a1c1"),
		Ok: c("#a3be8c"), Warn: c("#ebcb8b"), Err: c("#bf616a"),
		SelBg: c("#434c5e"), SelFg: c("#eceff4"),
	},
	{
		Name: "gruvbox-dark",
		Bg:   c("#282828"), Fg: c("#ebdbb2"), Subtle: c("#928374"),
		Border: c("#3c3836"), BorderOn: c("#fabd2f"),
		Accent: c("#fabd2f"), Accent2: c("#d3869b"),
		Ok: c("#b8bb26"), Warn: c("#fe8019"), Err: c("#fb4934"),
		SelBg: c("#3c3836"), SelFg: c("#fbf1c7"),
	},
	{
		Name: "solarized-dark",
		Bg:   c("#002b36"), Fg: c("#93a1a1"), Subtle: c("#586e75"),
		Border: c("#073642"), BorderOn: c("#268bd2"),
		Accent: c("#268bd2"), Accent2: c("#6c71c4"),
		Ok: c("#859900"), Warn: c("#b58900"), Err: c("#dc322f"),
		SelBg: c("#073642"), SelFg: c("#eee8d5"),
	},
	{
		Name: "solarized-light",
		Bg:   c("#fdf6e3"), Fg: c("#073642"), Subtle: c("#93a1a1"),
		Border: c("#d9d2bd"), BorderOn: c("#268bd2"),
		Accent: c("#268bd2"), Accent2: c("#6c71c4"),
		Ok: c("#859900"), Warn: c("#b58900"), Err: c("#dc322f"),
		SelBg: c("#eee8d5"), SelFg: c("#073642"),
	},
	{
		Name: "matrix",
		Bg:   c("#000000"), Fg: c("#00ff41"), Subtle: c("#00803a"),
		Border: c("#00451c"), BorderOn: c("#00ff41"),
		Accent: c("#00ff41"), Accent2: c("#7fff9f"),
		Ok: c("#00ff41"), Warn: c("#d7ff00"), Err: c("#ff2d2d"),
		SelBg: c("#00381a"), SelFg: c("#b7ffcb"),
	},
}

func (t Theme) FG(col lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(col)
}
