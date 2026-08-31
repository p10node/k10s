package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/p10node/k10s/internal/domain"
)

// Contexts are chosen from the main panel, the same way namespaces are —
// a full-width list beats a cramped popup for names as long as kubeconfig
// contexts tend to be.

func (m *Model) showContextChooser() {
	m.mode = modeContexts
	m.focus = focusMain
	m.ctxFilter = ""
	m.ctxIdx = 0
	cur := m.src.ClusterInfo().Context
	for i, c := range m.ctxChoices() {
		if c == cur {
			m.ctxIdx = i
		}
	}
	m.toast = "pick a context · enter reconnects"
}

// ctxChoices returns the contexts to show, always in the same order.
//
// Three sources, merged: whatever the current backend serves, kubeconfig's
// own list as read at startup, and the demo context. The middle one is what
// makes the demo escapable — the demo backend serves only its own contexts,
// so without it "pick a real context to leave" would have nothing to pick.
// The last one is what makes it reachable from a real cluster, and it is
// offered unconditionally: it needs no kubeconfig and can never fail.
//
// This is called on every render as well as from the key handler, so an
// unstable order would mean the highlighted row moves under the cursor
// between frames. Sorting here keeps that true whatever a backend returns.
func (m *Model) ctxChoices() []string {
	seen := map[string]bool{}
	all := make([]string, 0, len(m.kubeCtxs)+4)
	add := func(names ...string) {
		for _, n := range names {
			if n != "" && !seen[n] {
				seen[n] = true
				all = append(all, n)
			}
		}
	}
	add(m.src.Contexts()...)
	add(m.kubeCtxs...)
	add(domain.DemoContext)
	domain.SortNames(all)

	q := strings.ToLower(m.ctxFilter)
	if q == "" {
		return all
	}
	out := make([]string, 0, len(all))
	for _, c := range all {
		if strings.Contains(strings.ToLower(c), q) {
			out = append(out, c)
		}
	}
	return out
}

// chooseContext switches to the highlighted context, or says so when it is
// already the active one.
func (m *Model) chooseContext() tea.Cmd {
	choices := m.ctxChoices()
	if len(choices) == 0 {
		return nil
	}
	name := choices[clamp(m.ctxIdx, 0, len(choices)-1)]
	m.mode = modeTable
	// While connecting — or after that attempt failed — "the current
	// context" is only the one we are *trying*, so picking it again is a
	// retry, not a no-op. Answering "already on X" from the No cluster
	// panel would be a dead end, and the wrong claim besides.
	if name == m.src.ClusterInfo().Context && !m.connecting && !m.offline {
		m.toast = "already on " + name
		return nil
	}
	return m.switchContextCmd(name)
}

// contextBody renders the context list in the main panel, numbered and
// marked the same way tables are so it reads as part of the same UI.
func (m *Model) contextBody(inner, rows int) []string {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}

	choices := m.ctxChoices()
	current := m.src.ClusterInfo().Context
	numW := clamp(len(strconv.Itoa(maxi(1, len(choices)))), 2, 4)

	out := []string{
		s(th.Subtle).Bold(true).Render(spaces(numW+3) + "CONTEXT"),
		s(th.Border).Render(strings.Repeat("╌", inner)),
	}

	// How much of the row the right-hand label may take. Long context names
	// are the norm, so the label is what gets dropped when it does not fit,
	// never the name.
	cur := clamp(m.ctxIdx, 0, maxi(0, len(choices)-1))
	for i, c := range choices {
		selected := i == cur
		bg, fg := th.Bg, th.Fg
		if selected {
			bg, fg = th.SelBg, th.SelFg
		}
		st := func(col lipgloss.Color) lipgloss.Style {
			return lipgloss.NewStyle().Background(bg).Foreground(col)
		}
		marker := " "
		if selected {
			marker = "▌"
		}
		numCol := th.Border
		if selected {
			numCol = th.Accent2
		}
		row := st(th.Accent).Render(marker) + st(bg).Render(" ") +
			st(numCol).Render(fmt.Sprintf("%*d", numW, i+1)) + st(bg).Render(" ") +
			st(fg).Bold(selected).Render(trunc(c, inner-numW-14))

		// The demo says so on its own row. A context list is exactly where
		// somebody decides what they are looking at, so the one entry that
		// is not a cluster cannot be told apart by its name alone.
		label, labelCol := "", th.Ok
		if domain.IsDemoContext(c) {
			label, labelCol = "k10s demo · sample data", th.Warn
		}
		if c == current {
			label, labelCol = "current", th.Ok
			if domain.IsDemoContext(c) {
				label, labelCol = "current · k10s demo, sample data", th.Warn
			}
		}
		if label != "" {
			gap := inner - lipgloss.Width(row) - len(label) - 1
			if gap >= 1 {
				row += st(bg).Render(spaces(gap)) + st(labelCol).Render(label)
			}
		}
		out = append(out, m.mark(fmt.Sprintf("ctxp:%d", i), padBG(row, inner, bg)))
	}

	if len(choices) == 0 {
		out = append(out, s(th.Subtle).Render("   no contexts in kubeconfig"))
	}

	// The legend, once, under the list: what the demo entry is and how to
	// leave it. Without it "k10s-demo" is just another name in a list of
	// names, and the whole point is that it is not one. Two lines, because
	// the sentence that fits on one is the one that says too little.
	tag := "   " + domain.DemoContext + "*  "
	body := maxi(0, inner-len(tag))
	out = append(out,
		s(th.Border).Render(strings.Repeat("╌", inner)),
		s(th.Warn).Render(tag)+
			s(th.Subtle).Render(trunc("k10s's own demo cluster — sample data, not from kubeconfig.", body)),
		s(th.Bg).Render(spaces(len(tag)))+
			s(th.Subtle).Render(trunc("Nothing in it is real. Pick any other context to leave it.", body)),
	)
	return out
}
