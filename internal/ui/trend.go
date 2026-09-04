package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/p10node/k10s/internal/theme"
)

// A trend remembers which way a metric last moved. Usage rising is drawn as
// a red ▲ and falling as a green ▼, so a glance at the header or a CPU/MEM
// column says "this just got worse" without reading the numbers twice.
type trend struct {
	prev int  // last value seen
	dir  int  // -1 fell, 0 unchanged, +1 rose
	at   int  // m.anim when dir last changed
	seen bool // prev is real, not the zero value
}

// trendTicks is how many repaint ticks an arrow stays on screen after a
// change — roughly half a minute at the idle two-second cadence. Metrics
// refresh far slower than the UI repaints, so an arrow that only lived until
// the next unchanged frame would blink off before anyone saw it.
const trendTicks = 15

// observe folds a new reading in and reports the direction of the latest
// change. The first reading sets the baseline and shows nothing.
func (t *trend) observe(v, now int) {
	if !t.seen {
		t.prev, t.seen = v, true
		return
	}
	switch {
	case v > t.prev:
		t.dir, t.at = 1, now
	case v < t.prev:
		t.dir, t.at = -1, now
	}
	t.prev = v
}

// arrow is the direction still worth showing at tick now, or 0.
func (t trend) arrow(now int) int {
	if t.dir == 0 || now-t.at > trendTicks {
		return 0
	}
	return t.dir
}

// trendGlyph draws the arrow (or a space to hold the column steady) over bg.
func trendGlyph(th theme.Theme, bg lipgloss.Color, dir int) string {
	st := lipgloss.NewStyle().Background(bg)
	switch dir {
	case 1:
		return st.Foreground(th.Err).Render("▲")
	case -1:
		return st.Foreground(th.Ok).Render("▼")
	}
	return st.Render(" ")
}

// metricColumn says whether a table column carries live usage that is worth
// trending — pods' CPU/MEM and nodes' CPU%/MEM%.
func metricColumn(name string) bool {
	switch name {
	case "CPU", "MEM", "CPU%", "MEM%":
		return true
	}
	return false
}

// metricValue reads the leading number out of a usage cell — "250m", "64Mi",
// "38%" — so cells can be compared. "-" and other non-numbers report false.
func metricValue(s string) (int, bool) {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:end])
	return n, err == nil
}

// resetTrends forgets every reading. Called when the backend changes, so a
// pod on the new cluster that happens to share a name with one on the old
// is not compared against a stranger's numbers.
func (m *Model) resetTrends() {
	m.cpuTrend, m.memTrend = trend{}, trend{}
	m.trends = nil
}

// rowTrend returns the tracked trend for one metric cell, keyed by the kind,
// namespace and name so the same pod is compared with itself across frames.
func (m *Model) rowTrend(kind, ns, name, col string) *trend {
	if m.trends == nil {
		m.trends = map[string]*trend{}
	}
	k := kind + "\x00" + ns + "\x00" + name + "\x00" + col
	t := m.trends[k]
	if t == nil {
		t = &trend{}
		m.trends[k] = t
	}
	return t
}
