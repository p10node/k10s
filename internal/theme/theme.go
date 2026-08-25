package theme

import "github.com/charmbracelet/lipgloss"

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
