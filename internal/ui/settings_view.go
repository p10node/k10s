package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"k10s/internal/ai"
	"k10s/internal/config"
)

// The single settings modal: CLI name and AI provider in one place, doubling
// as the first-run screen.
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

	body = append(body,
		s(th.Subtle).Render("  shown in hints  ")+s(th.Ok).Render("$ "+m.selectedCLI()+" get pods"),
		"",
		s(th.Subtle).Render("  AI PROMPT")+s(th.Border).Render("  — used by AI mode (ctrl+a)"),
	)

	// Provider radios.
	pi := rowProvider()
	{
		bg := rowBG(pi)
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
		row := st(th.Accent).Render(lead(pi)) +
			radio(m.cfg.provider == 0, ai.Providers[0].Label, "set:openai") +
			st(bg).Render("   ") +
			radio(m.cfg.provider == 1, ai.Providers[1].Label, "set:anthropic")
		body = append(body, zone.Mark(fmt.Sprintf("set:%d", pi), padBG(row, inner, bg)))
	}

	// Text fields.
	field := func(i int, name, val string, secret bool) string {
		bg := rowBG(i)
		st := func(c lipgloss.Color) lipgloss.Style {
			return lipgloss.NewStyle().Background(bg).Foreground(c)
		}
		row := st(th.Accent).Render(lead(i)) + st(th.Subtle).Render(fmt.Sprintf("%-9s", name))
		if m.setEditing && m.setRow == i {
			m.input.Width = inner - 16
			row += m.input.View()
		} else {
			shown := val
			if secret {
				shown = maskKey(val)
			}
			if shown == "" {
				shown = "—"
			}
			row += st(th.Fg).Render(trunc(shown, inner-16))
		}
		return zone.Mark(fmt.Sprintf("set:%d", i), padBG(row, inner, bg))
	}
	body = append(body,
		field(rowURL(), "base url", m.cfg.url, false),
		field(rowModel(), "model", m.cfg.model, false),
		field(rowKey(), "api key", m.cfg.key, true),
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
	if !m.onboarded {
		title = "Welcome to k10s"
	}

	h := len(body) + 2
	box := Panel(th, PanelOpts{Title: title, Focused: true, W: w, H: h}, body)
	return root.Overlay(box, (m.w-w)/2, maxi(0, (m.h-h)/2))
}

// maskKey shows enough of an API key to recognise it without exposing it.
func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) > 18 {
		return k[:10] + "••••" + k[len(k)-4:]
	}
	return strings.Repeat("•", len(k))
}
