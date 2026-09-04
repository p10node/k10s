package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/theme"
)

func sourceSynced(src domain.Source, kind, namespace string) (supported, synced bool) {
	if scoped, ok := src.(interface{ SyncedFor(string, string) bool }); ok {
		return true, scoped.SyncedFor(kind, namespace)
	}
	if legacy, ok := src.(interface{ Synced(string) bool }); ok {
		return true, legacy.Synced(kind)
	}
	return false, true
}

func sourceLoadError(src domain.Source, kind, namespace string) error {
	if failed, ok := src.(interface{ LoadErrorFor(string, string) error }); ok {
		return failed.LoadErrorFor(kind, namespace)
	}
	return nil
}

// kindLoading reports whether the pane is waiting on its first batch of data
// for the current kind. Backends that can't report sync state (the offline
// demo) always answer false — their data is there immediately.
func (m *Model) kindLoading() bool {
	supported, synced := sourceSynced(m.src, m.curKind().Key, m.namespace)
	return supported && !synced
}

func (m *Model) kindLoadError() error {
	return sourceLoadError(m.src, m.curKind().Key, m.namespace)
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

// loadErrorLines is the terminal state for a first LIST/WATCH that failed.
// Keeping the API error visible is especially important for RBAC: an
// indefinite spinner suggests patience will help, while Forbidden requires a
// different namespace or permission grant.
func (m *Model) loadErrorLines(inner int, err error) []string {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	label := "unable to load " + strings.ToLower(m.curKind().Name)
	lines := []string{"", s(th.Err).Bold(true).Render("   ✗ " + label), ""}
	for _, line := range wrapLine(err.Error(), maxi(8, inner-6)) {
		lines = append(lines, s(th.Fg).Render("   "+line))
	}
	lines = append(lines, "")
	for _, line := range wrapLine("Check RBAC and the selected namespace; K10s will retry automatically.", maxi(8, inner-6)) {
		lines = append(lines, s(th.Subtle).Render("   "+line))
	}
	return lines
}

// connectingLines renders the startup spinner, shown until the backend is
// built. The hint names the way out, since an unreachable context is the
// usual reason this takes long enough to read.
func (m *Model) connectingLines(inner int) []string {
	target := m.connName
	if target == "" {
		target = "the cluster"
	}
	return m.spinnerBlock(inner, "connecting to "+target+"…",
		"kubeconfig · API server handshake — :ctx picks another one")
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
