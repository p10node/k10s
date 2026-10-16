package ui

import (
	"slices"
	"strings"
	"unicode/utf8"
)

const maxLogLineBytes = 64 * 1024

func truncateLogLine(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	end := limit
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return strings.Clone(s[:end])
}

// boundLogs limits retained payload and discards backing references to old lines.
// History cannot grow past the same cap used by live streams.
func (m *Model) boundLogs() {
	maxLines, maxBytes := m.logLimits.Limits()
	start, used := len(m.textLines), 0
	for start > 0 && len(m.textLines)-start < maxLines {
		i := start - 1
		line := truncateLogLine(m.textLines[i], min(maxLogLineBytes, maxBytes))
		if used+len(line) > maxBytes {
			break
		}
		m.textLines[i] = line
		used += len(line)
		start = i
	}
	if start > 0 {
		m.textLines = slices.Clone(m.textLines[start:])
		m.logMore = false
		// Dropped lines shrink the wrapped-row count, so clamp the offset.
		// Only then: re-wrapping every retained line per incoming line is
		// too costly while the view is paused on a busy pod.
		if m.logScroll > 0 {
			m.logScroll = min(m.logScroll, m.logTotalRows())
		}
	}
	if len(m.textLines) >= maxLines || used >= maxBytes {
		m.logMore = false
	}
}
