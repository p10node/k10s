package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"k10s/internal/theme"
)

// kindLoading reports whether the pane is waiting on its first batch of data
// for the current kind. Backends that can't report sync state (the offline
// demo) always answer false — their data is there immediately.
func (m *Model) kindLoading() bool {
	sy, ok := m.src.(interface{ Synced(string) bool })
	if !ok {
		return false
	}
	return !sy.Synced(m.curKind().Key)
}

// Braille spinner: smooth, single-cell, and present in the same fonts the
// rest of the UI already relies on.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// busyLines renders the spinner for an action that is still running, so
// pressing a key always looks like it did something.
func (m *Model) busyLines(inner int) []string {
	return m.spinnerBlock(inner, m.busyLabel+"…", "working — the result opens here when it lands")
}

// loadingLines renders a centred spinner, label and indeterminate progress
// bar for the main panel's empty state.
func (m *Model) loadingLines(inner int) []string {
	return m.spinnerBlock(inner,
		"loading "+strings.ToLower(m.curKind().Name)+"…",
		"watch is being established · first list can take a moment")
}

func (m *Model) spinnerBlock(inner int, label, hint string) []string {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}

	frame := spinnerFrames[m.anim%len(spinnerFrames)]

	spin := s(th.Accent).Bold(true).Render(frame) + s(th.Fg).Render(" "+label)
	pad := maxi(0, (inner-lipgloss.Width(frame)-1-len(label))/2)

	// Indeterminate bar: a lit block sweeping back and forth, since the
	// informer gives no completion percentage to report honestly.
	const barW = 24
	bar := indeterminateBar(th, m.anim, barW)
	barPad := maxi(0, (inner-barW)/2)

	hintPad := maxi(0, (inner-len(hint))/2)

	return []string{
		s(th.Bg).Render(spaces(pad)) + spin,
		"",
		s(th.Bg).Render(spaces(barPad)) + bar,
		"",
		s(th.Bg).Render(spaces(hintPad)) + s(th.Subtle).Render(hint),
	}
}

// indeterminateBar draws a track with a short lit run bouncing left to
// right — an honest "working" signal, since the informer reports no
// percentage we could show.
func indeterminateBar(th theme.Theme, step, w int) string {
	const runW = 6
	start := bounce(step, w-runW)

	var b strings.Builder
	on := lipgloss.NewStyle().Background(th.Bg).Foreground(th.Accent)
	off := lipgloss.NewStyle().Background(th.Bg).Foreground(th.Border)
	for i := 0; i < w; i++ {
		if i >= start && i < start+runW {
			b.WriteString(on.Render("━"))
		} else {
			b.WriteString(off.Render("─"))
		}
	}
	return b.String()
}

// bounce maps a monotonically increasing step onto a ping-pong position in
// [0, span], so the lit run reverses at each end instead of jumping back.
func bounce(step, span int) int {
	if span <= 0 {
		return 0
	}
	period := span * 2
	p := step % period
	if p <= span {
		return p
	}
	return period - p
}
