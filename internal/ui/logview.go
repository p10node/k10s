package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"k10s/internal/theme"
)

// The log viewer.
//
// Three behaviours make it feel like `tail -f` rather than a text dump:
//
//   - newest is at the bottom and the view opens pinned there;
//   - line numbers count *up from the bottom*, so the newest line is always
//     1 and a number keeps meaning the same thing as new lines arrive;
//   - scrolling up unpins ("follow off"), scrolling back to the bottom pins
//     again — the behaviour every log tool has.
//
// Long lines wrap rather than being cut off with an ellipsis: a truncated
// log line is often exactly the part you needed.

// logChunk is the page size: the viewer opens with the newest 500 lines and
// each scroll back past the top asks for 500 older ones, indefinitely.
const logChunk = 500

// wrapLine breaks s into segments of at most w cells, splitting on spaces
// where possible so words survive. Returns at least one segment.
func wrapLine(s string, w int) []string {
	if w < 8 {
		w = 8
	}
	if lipgloss.Width(s) <= w {
		return []string{s}
	}

	var out []string
	for lipgloss.Width(s) > w {
		cut := breakPoint(s, w)
		out = append(out, strings.TrimRight(s[:cut], " "))
		s = strings.TrimLeft(s[cut:], " ")
		if s == "" {
			break
		}
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

// breakPoint finds where to split a line: the last space inside the width
// if there is a reasonable one, otherwise a hard cut at the width.
func breakPoint(s string, w int) int {
	// Byte index of the w'th cell. Log lines are overwhelmingly ASCII, and
	// a hard cut on a multi-byte boundary is corrected by the rune scan.
	limit := w
	if limit > len(s) {
		limit = len(s)
	}
	for limit > 0 && limit < len(s) && !isRuneStart(s[limit]) {
		limit--
	}
	if sp := strings.LastIndexByte(s[:limit], ' '); sp > w/2 {
		return sp
	}
	return limit
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// logLevelStyle colours just the level token, not the whole line — the
// message is what you read; the level is what you scan for.
func logLevelStyle(th theme.Theme, tok string) (lipgloss.Color, bool) {
	switch strings.ToUpper(strings.Trim(tok, "[]():")) {
	case "ERROR", "ERR", "FATAL", "PANIC", "CRITICAL":
		return th.Err, true
	case "WARN", "WARNING":
		return th.Warn, true
	case "INFO", "NOTICE":
		return th.Ok, true
	case "DEBUG", "TRACE":
		return th.Subtle, true
	}
	return "", false
}

// renderLogLine colours the level token and dims the leading timestamp,
// leaving the message itself in the normal foreground.
func renderLogLine(th theme.Theme, s string) string {
	bg := th.Bg
	st := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(bg).Foreground(c)
	}

	fields := strings.Fields(s)
	if len(fields) == 0 {
		return st(th.Fg).Render(s)
	}

	var b strings.Builder
	rest := s
	for _, f := range fields[:min(len(fields), 3)] {
		idx := strings.Index(rest, f)
		if idx < 0 {
			break
		}
		b.WriteString(st(th.Fg).Render(rest[:idx]))
		rest = rest[idx+len(f):]

		switch {
		case looksLikeTimestamp(f):
			b.WriteString(st(th.Subtle).Render(f))
		default:
			if col, ok := logLevelStyle(th, f); ok {
				b.WriteString(lipgloss.NewStyle().Background(bg).Foreground(col).Bold(true).Render(f))
			} else {
				b.WriteString(st(th.Fg).Render(f))
			}
		}
	}
	b.WriteString(st(th.Fg).Render(rest))
	return b.String()
}

// looksLikeTimestamp is a cheap shape test — enough to dim the leading
// RFC3339-ish stamp kubectl prepends, without parsing dates on every line.
func looksLikeTimestamp(f string) bool {
	if len(f) < 8 {
		return false
	}
	digits := 0
	for i := 0; i < len(f); i++ {
		if f[i] >= '0' && f[i] <= '9' {
			digits++
		}
	}
	return digits >= 6 && (strings.Count(f, ":") >= 2 || strings.Count(f, "-") >= 2)
}

// logBody renders the visible window of the log, bottom-anchored, with
// wrapped lines and bottom-relative numbering.
func (m *Model) logBody(inner, rows int) []string {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}

	// One row is spent on the status line at the bottom.
	viewRows := maxi(1, rows-1)

	// Numbers count up from the newest line, so the gutter width follows
	// the oldest number on screen rather than the total.
	numW := clamp(len(fmt.Sprint(len(m.textLines))), 2, 6)
	textW := maxi(8, inner-numW-3)

	// Build display rows newest-first, then reverse: wrapping means one log
	// line can occupy several rows, so the window has to be filled from the
	// bottom to keep the newest line pinned to the last row.
	type drow struct {
		num  int    // bottom-relative log-line number, 0 for continuations
		text string // one wrapped segment
	}
	var stack []drow

	skip := m.logScroll // how many display rows are hidden below the view
	for i := len(m.textLines) - 1; i >= 0 && len(stack) < viewRows+skip; i-- {
		segs := wrapLine(m.textLines[i], textW)
		num := len(m.textLines) - i
		// Segments belong to one line; emit them bottom-up so the first
		// segment (carrying the number) ends up on top.
		for j := len(segs) - 1; j >= 0; j-- {
			n := 0
			if j == 0 {
				n = num
			}
			stack = append(stack, drow{num: n, text: segs[j]})
		}
	}
	if skip > len(stack) {
		skip = len(stack)
	}
	stack = stack[skip:]
	if len(stack) > viewRows {
		stack = stack[:viewRows]
	}

	out := make([]string, 0, rows)
	for i := len(stack) - 1; i >= 0; i-- {
		d := stack[i]
		gutter := strings.Repeat(" ", numW)
		if d.num > 0 {
			gutter = fmt.Sprintf("%*d", numW, d.num)
		}
		out = append(out, s(th.Border).Render(" "+gutter+" ")+renderLogLine(th, d.text))
	}
	for len(out) < viewRows {
		out = append([]string{""}, out...)
	}

	out = append(out, m.logStatusLine(inner))
	return out
}

// logScrollBy moves the log view, managing follow state. delta > 0 moves
// toward older entries (up the screen). Reaching the top of what is loaded
// asks for more, which is what makes scrolling up feel endless.
func (m *Model) logScrollBy(delta int) {
	maxScroll := maxi(0, len(m.textLines)-1)
	m.logScroll = clamp(m.logScroll+delta, 0, maxScroll)
	// Pinned to the bottom means following; anywhere else means paused.
	m.logFollow = m.logScroll == 0
}

// logNeedsOlder reports whether the view has reached the oldest loaded line
// and should pull the next page.
//
// The check fires slightly before the very top (prefetch) so the next 500
// are usually already there by the time you get there — scrolling back
// stays continuous instead of stalling at each page boundary.
func (m *Model) logNeedsOlder() bool {
	const prefetch = 40
	return m.logMore && !m.logLoading &&
		m.logScroll+m.visibleRows()+prefetch >= len(m.textLines)
}

// logStatusLine reports follow state, how much is loaded, and whether older
// entries are still available.
func (m *Model) logStatusLine(inner int) string {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}

	var left string
	if m.logFollow {
		left = s(th.Ok).Bold(true).Render(" ● following") + s(th.Subtle).Render("  newest at bottom")
	} else {
		left = s(th.Warn).Bold(true).Render(" ⏸ paused") +
			s(th.Subtle).Render(fmt.Sprintf("  %d line(s) below · end resumes", m.logScroll))
	}

	right := fmt.Sprintf("%d loaded", len(m.textLines))
	switch {
	case m.logLoading:
		right = "loading older…"
	case m.logMore:
		right += " · ↑ for older"
	default:
		right += " · start of log"
	}

	gap := inner - lipgloss.Width(left) - len(right) - 1
	if gap < 1 {
		gap = 1
	}
	return left + s(th.Bg).Render(spaces(gap)) + s(th.Subtle).Render(right) + s(th.Bg).Render(" ")
}
