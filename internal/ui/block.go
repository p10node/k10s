package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"k10s/internal/theme"
)

// Block is a fixed-size rectangle of terminal cells. Every line is padded to
// exactly W visible cells, so blocks can be joined without re-measuring
// (important: measuring breaks once bubblezone markers are embedded).
type Block struct {
	W, H  int
	Lines []string
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

func pad(s string, w int) string {
	d := w - lipgloss.Width(s)
	switch {
	case d > 0:
		return s + spaces(d)
	case d < 0:
		return ansi.Truncate(s, w, "")
	}
	return s
}

// padBG pads to w using an independently-rendered background run, so we never
// nest lipgloss styles (a nested reset would drop the outer background).
func padBG(s string, w int, bg lipgloss.Color) string {
	d := w - lipgloss.Width(s)
	switch {
	case d > 0:
		return s + lipgloss.NewStyle().Background(bg).Render(spaces(d))
	case d < 0:
		return ansi.Truncate(s, w, "")
	}
	return s
}

func trunc(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 1 {
		return ansi.Truncate(s, w, "")
	}
	return ansi.Truncate(s, w, "…")
}

func NewBlock(w, h int, bg lipgloss.Color) Block {
	fill := lipgloss.NewStyle().Background(bg).Render(spaces(w))
	lines := make([]string, h)
	for i := range lines {
		lines[i] = fill
	}
	return Block{W: w, H: h, Lines: lines}
}

func BlockOf(w, h int, lines []string, bg lipgloss.Color) Block {
	out := make([]string, h)
	for i := 0; i < h; i++ {
		s := ""
		if i < len(lines) {
			s = lines[i]
		}
		out[i] = padBG(s, w, bg)
	}
	return Block{W: w, H: h, Lines: out}
}

func HJoin(bs ...Block) Block {
	h, w := 0, 0
	for _, b := range bs {
		if b.H > h {
			h = b.H
		}
		w += b.W
	}
	lines := make([]string, h)
	for i := 0; i < h; i++ {
		var sb strings.Builder
		for _, b := range bs {
			if i < len(b.Lines) {
				sb.WriteString(b.Lines[i])
			} else {
				sb.WriteString(spaces(b.W))
			}
		}
		lines[i] = sb.String()
	}
	return Block{W: w, H: h, Lines: lines}
}

func VJoin(bs ...Block) Block {
	w, h := 0, 0
	var lines []string
	for _, b := range bs {
		if b.W > w {
			w = b.W
		}
		h += b.H
		lines = append(lines, b.Lines...)
	}
	return Block{W: w, H: h, Lines: lines}
}

// Overlay stamps o onto b at (x, y).
func (b Block) Overlay(o Block, x, y int) Block {
	lines := append([]string(nil), b.Lines...)
	for i := 0; i < o.H; i++ {
		ly := y + i
		if ly < 0 || ly >= len(lines) {
			continue
		}
		src := lines[ly]
		left := pad(ansi.Truncate(src, x, ""), x)
		right := ansi.TruncateLeft(src, x+o.W, "")
		lines[ly] = left + o.Lines[i] + right
	}
	return Block{W: b.W, H: b.H, Lines: lines}
}

func (b Block) String() string { return strings.Join(b.Lines, "\n") }

const (
	bTL, bTR, bBL, bBR = "╭", "╮", "╰", "╯"
	bH, bV             = "─", "│"
)

// Panel draws a bordered box with the title embedded in the top border and an
// optional right-aligned tag (used for the zoom button).
type PanelOpts struct {
	Title    string
	Tag      string // already-styled; TagPlain gives its visible text
	TagPlain string
	Focused  bool
	W, H     int
	// BorderCol overrides the border/title colour (used by the danger modal).
	BorderCol lipgloss.Color
}

func Panel(th theme.Theme, o PanelOpts, body []string) Block {
	bs := lipgloss.NewStyle().Background(th.Bg).Foreground(th.Border)
	if o.Focused {
		bs = bs.Foreground(th.BorderOn)
	}
	if o.BorderCol != "" {
		bs = bs.Foreground(o.BorderCol)
	}
	ts := lipgloss.NewStyle().Background(th.Bg).Foreground(th.Subtle)
	if o.Focused {
		ts = ts.Foreground(th.Accent).Bold(true)
	}
	if o.BorderCol != "" {
		ts = ts.Foreground(o.BorderCol).Bold(true)
	}

	inner := o.W - 2
	if inner < 1 {
		inner = 1
	}

	// top border
	title := trunc(o.Title, inner-4)
	leftPlain := bH + " " + title + " "
	rightPlain := ""
	if o.TagPlain != "" {
		rightPlain = " " + o.TagPlain + " "
	}
	fill := inner - lipgloss.Width(leftPlain) - lipgloss.Width(rightPlain)
	if fill < 0 {
		fill = 0
	}
	top := bs.Render(bTL+bH+" ") + ts.Render(title) + bs.Render(" "+strings.Repeat(bH, fill))
	if rightPlain != "" {
		top += bs.Render(" ") + o.Tag + bs.Render(" ")
	}
	top += bs.Render(bTR)

	bodyH := o.H - 2
	lines := make([]string, 0, o.H)
	lines = append(lines, top)
	for i := 0; i < bodyH; i++ {
		s := ""
		if i < len(body) {
			s = body[i]
		}
		lines = append(lines, bs.Render(bV)+padBG(s, inner, th.Bg)+bs.Render(bV))
	}
	lines = append(lines, bs.Render(bBL+strings.Repeat(bH, inner)+bBR))
	return Block{W: o.W, H: o.H, Lines: lines}
}
