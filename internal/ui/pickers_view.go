package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/p10node/k10s/internal/theme"
)

// saveButton renders the shared Save affordance used by the pickers.
func saveButton(th theme.Theme, id string, focused bool) (rendered, plain string) {
	plain = "  Save  "
	st := lipgloss.NewStyle().Background(th.Border).Foreground(th.Fg)
	if focused {
		st = lipgloss.NewStyle().Background(th.Accent).Foreground(th.Bg).Bold(true)
	}
	return zone.Mark(id, st.Render(plain)), plain
}

// ---- theme picker --------------------------------------------------------

func (m *Model) overlayThemePicker(root Block) Block {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}

	w := 60
	if w > m.w-6 {
		w = m.w - 6
	}
	inner := w - 2

	// Custom themes make the list unbounded. Keep the selected row in a
	// viewport and reserve fixed room for the separator and Save button, so
	// the popup never grows beyond the terminal.
	visible := maxi(1, m.h-10)
	if visible > len(m.themes) {
		visible = len(m.themes)
	}
	start := clamp(m.themeRow-visible/2, 0, len(m.themes)-visible)
	end := start + visible
	title := "Theme · live preview"
	if visible < len(m.themes) {
		title += fmt.Sprintf(" · %d–%d/%d", start+1, end, len(m.themes))
	}

	body := []string{""}
	for i := start; i < end; i++ {
		t := m.themes[i]
		selected := i == m.themeRow && !m.themeSave
		bg := th.Bg
		nameCol := th.Fg
		if selected {
			bg, nameCol = th.SelBg, th.SelFg
		}
		st := func(c lipgloss.Color) lipgloss.Style {
			return lipgloss.NewStyle().Background(bg).Foreground(c)
		}
		marker := "   "
		if selected {
			marker = " ▸ "
		}
		// A miniature swatch of the theme's own colours, so the list is
		// scannable even before previewing each entry.
		swatch := lipgloss.NewStyle().Background(t.Bg).Foreground(t.Accent).Render("██") +
			lipgloss.NewStyle().Background(t.Bg).Foreground(t.Accent2).Render("██") +
			lipgloss.NewStyle().Background(t.Bg).Foreground(t.Ok).Render("██") +
			lipgloss.NewStyle().Background(t.Bg).Foreground(t.Warn).Render("██") +
			lipgloss.NewStyle().Background(t.Bg).Foreground(t.Err).Render("██")

		name := t.Name
		row := st(th.Accent).Render(marker) + st(nameCol).Render(fmt.Sprintf("%-22s", trunc(name, 22))) +
			swatch
		if i == m.themeOrig {
			row += st(th.Subtle).Render("  current")
		}
		body = append(body, zone.Mark(fmt.Sprintf("thm:%d", i), padBG(row, inner, bg)))
	}

	body = append(body, "", s(th.Border).Render(strings.Repeat("╌", inner)))

	btn, btnPlain := saveButton(th, "thm:save", m.themeSave)
	hint := " ↑↓ preview · enter apply · esc cancel"
	gap := inner - lipgloss.Width(hint) - len(btnPlain) - 1
	if gap < 1 {
		gap = 1
	}
	body = append(body, s(th.Subtle).Render(hint)+s(th.Bg).Render(spaces(gap))+btn+s(th.Bg).Render(" "))

	h := len(body) + 2
	box := Panel(th, PanelOpts{Title: title, Focused: true, W: w, H: h}, body)
	return root.Overlay(box, (m.w-w)/2, maxi(0, (m.h-h)/2))
}
