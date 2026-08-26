package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/p10node/k10s/internal/mock"
	"github.com/p10node/k10s/internal/theme"
)

// A Panel wider than the W it reports drifts every HJoin and Overlay that
// comes after it, so the top border — the only line whose length depends on
// the title and the tag — has to hold the width on its own. Long text-view
// titles (`logs -f <pod>`, `top <pod>`) used to push it out by as much as the
// tag was wide.
func TestPanelTopBorderNeverExceedsItsWidth(t *testing.T) {
	th := theme.Themes[0]
	longTitle := "logs -f billing-worker-6f8d9c5b7-qq91x"

	cases := []struct {
		name     string
		title    string
		tagPlain string
	}{
		{"short title, no tag", "Pods · default", ""},
		{"short title with tag", "Pods · default", "[ f to search ] [ zoom ]"},
		{"long title, no tag", longTitle, ""},
		{"long title with tag", longTitle, "[ close ] [ zoom ]"},
		{"title longer than the panel", strings.Repeat("x", 300), "[ close ] [ zoom ]"},
		{"tag alone wider than the panel", "t", strings.Repeat("[ zoom ] ", 12)},
	}

	for _, c := range cases {
		for _, w := range []int{20, 40, 72, 96, 100, 140} {
			tag := ""
			if c.tagPlain != "" {
				tag = lipgloss.NewStyle().Background(th.Bg).Render(c.tagPlain)
			}
			b := Panel(th, PanelOpts{
				Title: c.title, Tag: tag, TagPlain: c.tagPlain, W: w, H: 4,
			}, []string{"body"})

			if b.W != w || b.H != 4 {
				t.Fatalf("%s at w=%d: Block is %dx%d, want %dx4", c.name, w, b.W, b.H, w)
			}
			for i, ln := range b.Lines {
				if got := lipgloss.Width(zone.Scan(ln)); got != w {
					t.Errorf("%s at w=%d: line %d is %d cells, want %d",
						c.name, w, i, got, w)
				}
			}
		}
	}
}

// The same invariant through the real render path: a log view whose title is
// long, in a terminal narrow enough that it cannot fit, must still produce a
// frame where every row is exactly the terminal width.
func TestLongTextTitleKeepsEveryRowAtTerminalWidth(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	// 100x28 is where this used to break: the logs title plus [ close ] and
	// [ zoom ] overflowed the centre pane by the width of the tags.
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	drainCmd(m, m.handleKey(key("l")))

	if m.mode != modeLogs {
		t.Fatalf("`l` should have opened the log viewer, mode = %v", m.mode)
	}
	if len(m.textTitle) < 30 {
		t.Fatalf("precondition: title %q is not long enough to overflow", m.textTitle)
	}
	for i, ln := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(zone.Scan(ln)); got != m.w {
			t.Errorf("row %d is %d cells wide, want %d: %q", i, got, m.w, zone.Scan(ln))
		}
	}
}
