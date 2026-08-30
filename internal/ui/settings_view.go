package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/p10node/k10s/internal/config"
	"github.com/p10node/k10s/internal/version"
)

// The single settings modal: CLI name and the update check in one place.
// Opened by /settings, and only by /settings.
func (m *Model) overlaySettings(root Block) Block {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}

	w := clamp(m.w-10, 48, 68)
	inner := w - 2

	rowBG := func(i int) lipgloss.Color {
		if i == m.setRow {
			return th.SelBg
		}
		return th.Bg
	}
	lead := func(i int) string {
		if i == m.setRow {
			return " ▸ "
		}
		return "   "
	}

	body := []string{
		"",
		s(th.Subtle).Render("  COMMAND NAME"),
		s(th.Border).Render("  " + strings.Join(config.CLIPresets, ", ") + " all work at the prompt."),
		s(th.Border).Render("  Add your own below if you alias it to something else."),
		"",
	}

	// Custom name.
	ci := rowCustom()
	{
		bg := rowBG(ci)
		st := func(c lipgloss.Color) lipgloss.Style {
			return lipgloss.NewStyle().Background(bg).Foreground(c)
		}
		row := st(th.Accent).Render(lead(ci)) + st(th.Subtle).Render(fmt.Sprintf("%-9s", "custom"))
		if m.setEditing && m.setRow == ci {
			m.input.Width = inner - 16
			row += m.input.View()
		} else {
			label := "—"
			if !isPreset(m.cli) && m.cli != "" {
				label = m.cli
			}
			row += st(th.Fg).Bold(ci == m.setRow).Render(label)
		}
		body = append(body, zone.Mark(fmt.Sprintf("set:%d", ci), padBG(row, inner, bg)))
	}

	// The AI PROMPT block (provider radios, base url, model, api key) used
	// to sit here. It is gone while the feature is off — see aidisabled.go.
	body = append(body,
		s(th.Subtle).Render("  shown in hints  ")+s(th.Ok).Render("$ "+m.selectedCLI()+" get pods"),
		"",
		s(th.Subtle).Render("  UPDATES")+s(th.Border).Render("  — /update installs on demand"),
	)

	// Update check toggle: two states, no typing, so it is a pair of radios
	// rather than a field.
	ui := rowUpdate()
	{
		bg := rowBG(ui)
		st := func(c lipgloss.Color) lipgloss.Style {
			return lipgloss.NewStyle().Background(bg).Foreground(c)
		}
		radio := func(on bool, txt, id string) string {
			mark, col := "○ ", th.Subtle
			if on {
				mark, col = "● ", th.Accent2
			}
			return zone.Mark(id, st(col).Render(mark+txt))
		}
		row := st(th.Accent).Render(lead(ui)) + st(th.Subtle).Render(fmt.Sprintf("%-9s", "check")) +
			radio(!m.updDisabled, "daily", "set:updon") +
			st(bg).Render("   ") +
			radio(m.updDisabled, "off", "set:updoff")
		body = append(body, zone.Mark(fmt.Sprintf("set:%d", ui), padBG(row, inner, bg)))
	}

	body = append(body,
		s(th.Border).Render("  "+trunc("from "+m.updateClient().Repository()+" · running "+version.Current(), inner-3)),
		"",
		s(th.Border).Render(strings.Repeat("╌", inner)),
	)

	btn, btnPlain := saveButton(th, "set:save", m.setRow == setSaveRow)
	hint := " ↑↓ move · enter edit · click a row · tab Save"
	if m.setEditing {
		hint = " editing — enter commit · esc cancel"
	}
	gap := inner - lipgloss.Width(hint) - len(btnPlain) - 1
	if gap < 1 {
		gap = 1
	}
	body = append(body, s(th.Subtle).Render(hint)+s(th.Bg).Render(spaces(gap))+btn+s(th.Bg).Render(" "))

	title := "Settings"

	h := len(body) + 2
	box := Panel(th, PanelOpts{Title: title, Focused: true, W: w, H: h}, body)
	return root.Overlay(box, (m.w-w)/2, maxi(0, (m.h-h)/2))
}
