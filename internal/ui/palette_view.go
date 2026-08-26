package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// ---- global search palette ----------------------------------------------

func (m *Model) overlayPalette(root Block) Block {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}

	w := clamp(m.w-16, 40, 88)
	inner := w - 2
	hits := m.paletteHits()

	m.input.Width = inner - 4
	body := []string{
		s(th.Accent).Bold(true).Render(" ⌕ ") + m.input.View(),
		s(th.Border).Render(strings.Repeat("╌", inner)),
	}

	const maxShown = 12
	cur := clamp(m.palIdx, 0, maxi(0, len(hits)-1))
	// Keep the highlighted entry on screen when the list is longer than the
	// box: scroll the window rather than the selection.
	top := 0
	if cur >= maxShown {
		top = cur - maxShown + 1
	}
	end := clamp(top+maxShown, 0, len(hits))

	for i := top; i < end; i++ {
		h := hits[i]
		selected := i == cur
		bg := th.Bg
		if selected {
			bg = th.SelBg
		}
		st := func(c lipgloss.Color) lipgloss.Style {
			return lipgloss.NewStyle().Background(bg).Foreground(c)
		}
		lead := "   "
		if selected {
			lead = " ▸ "
		}
		icon := "◆" // a kind
		if h.row >= 0 {
			icon = "·" // an individual object
		}
		label := trunc(h.label, inner/2)
		row := st(th.Accent).Render(lead) + st(th.Subtle).Render(icon+" ") +
			st(th.Fg).Bold(selected).Render(label)
		gap := inner - lipgloss.Width(row) - lipgloss.Width(h.sub) - 2
		if gap < 1 {
			gap = 1
		}
		row += st(bg).Render(spaces(gap)) + st(th.Subtle).Render(trunc(h.sub, inner/3)) + st(bg).Render(" ")
		body = append(body, zone.Mark(fmt.Sprintf("pal:%d", i), padBG(row, inner, bg)))
	}

	if len(hits) == 0 {
		msg := "type to search kinds and objects"
		if strings.TrimSpace(m.input.Value()) != "" {
			msg = "no matches"
		}
		body = append(body, s(th.Subtle).Render("   "+msg))
	}

	body = append(body, s(th.Border).Render(strings.Repeat("╌", inner)))

	// Be explicit that unopened kinds are matched by name only — otherwise
	// "search everything" would quietly mean less than it says.
	unloaded := 0
	for _, k := range m.kinds() {
		if !m.kindLoaded(k.Key) {
			unloaded++
		}
	}
	foot := " ↑↓ move · enter open · esc close"
	if unloaded > 0 {
		foot += fmt.Sprintf("   ·   %d kind(s) not loaded: name-only", unloaded)
	}
	body = append(body, s(th.Subtle).Render(trunc(foot, inner)))

	h := len(body) + 2
	box := Panel(th, PanelOpts{Title: "Search everything", Focused: true, W: w, H: h}, body)
	return root.Overlay(box, (m.w-w)/2, maxi(0, m.layout().headerH+1))
}
