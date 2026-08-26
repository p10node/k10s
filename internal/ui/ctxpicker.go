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
// This is called on every render as well as from the key handler, so an
// unstable order would mean the highlighted row moves under the cursor
// between frames. Sorting here keeps that true whatever a backend returns.
func (m *Model) ctxChoices() []string {
	all := append([]string(nil), m.src.Contexts()...)
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
	// While connecting, "the current context" is only the one we are
	// *trying* — picking it again is a retry, not a no-op.
	if name == m.src.ClusterInfo().Context && !m.connecting {
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
		if c == current {
			gap := inner - lipgloss.Width(row) - len("current") - 1
			if gap < 1 {
				gap = 1
			}
			row += st(bg).Render(spaces(gap)) + st(th.Ok).Render("current")
		}
		out = append(out, m.mark(fmt.Sprintf("ctxp:%d", i), padBG(row, inner, bg)))
	}

	if len(choices) == 0 {
		out = append(out, s(th.Subtle).Render("   no contexts in kubeconfig"))
	}
	return out
}
