package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/domain"
)

// The global search palette ("search everything"): one box that finds both
// resource kinds and individual objects.
//
// It only scans rows of kinds whose data is already loaded. Searching every
// kind would mean starting a cluster-wide watch for each one — exactly the
// cost the lazy-loading work removed — so kinds that haven't been opened are
// matched by name only, and the footer says so.

// paletteMax bounds how many hits are collected, so typing a single common
// letter on a large cluster can't build a huge list every keystroke.
const paletteMax = 200

type paletteHit struct {
	kindIdx int
	kind    domain.Kind
	label   string // object name, or the kind's name for a kind hit
	sub     string // context line: kind · namespace
	row     int    // index into the kind's rows; -1 for a kind itself
}

func (m *Model) openPalette() tea.Cmd {
	m.palOpen = true
	m.palIdx = 0
	m.input.SetValue("")
	m.input.CursorEnd()
	return m.input.Focus()
}

func (m *Model) closePalette() {
	m.palOpen = false
	m.palIdx = 0
	m.input.SetValue("")
	m.input.Blur()
}

// kindLoaded reports whether a kind's rows can be scanned without kicking
// off new cluster traffic.
func (m *Model) kindLoaded(key string) bool {
	sy, ok := m.src.(interface{ Synced(string) bool })
	if !ok {
		return true // backends without lazy loading always have their data
	}
	return sy.Synced(key)
}

// paletteHits returns what matches the current query.
func (m *Model) paletteHits() []paletteHit {
	q := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if q == "" {
		return nil
	}

	var out []paletteHit
	for i, k := range m.kinds() {
		// Kind itself.
		if strings.Contains(strings.ToLower(k.Name), q) ||
			strings.Contains(strings.ToLower(k.Short), q) ||
			strings.Contains(strings.ToLower(k.Group), q) {
			out = append(out, paletteHit{kindIdx: i, kind: k, label: k.Name, sub: k.Group, row: -1})
			if len(out) >= paletteMax {
				return out
			}
		}

		if !m.kindLoaded(k.Key) {
			continue
		}
		cols, rows := m.src.Rows(k.Key, m.namespace)
		nameIdx := identityColumn(cols, k.Key)
		for ri, row := range rows {
			if !strings.Contains(strings.ToLower(strings.Join(row, " ")), q) {
				continue
			}
			label := ""
			if nameIdx < len(row) {
				label = row[nameIdx]
			}
			out = append(out, paletteHit{
				kindIdx: i, kind: k, label: label,
				sub: k.Name + " · " + rowNamespace(cols, row, m.namespace), row: ri,
			})
			if len(out) >= paletteMax {
				return out
			}
		}
	}
	return out
}

// identityColumn finds the column that names the object.
func identityColumn(cols []string, kindKey string) int {
	want := "NAME"
	if kindKey == "events" {
		want = "OBJECT"
	}
	for i, c := range cols {
		if c == want {
			return i
		}
	}
	return 0
}

// rowNamespace reports which namespace a row belongs to, reading the
// NAMESPACE column when :ns all put one there.
func rowNamespace(cols []string, row []string, fallback string) string {
	for i, c := range cols {
		if c == "NAMESPACE" && i < len(row) {
			return row[i]
		}
	}
	if fallback == domain.AllNamespaces {
		return "all"
	}
	return fallback
}

func (m *Model) handlePaletteKey(msg tea.KeyMsg) tea.Cmd {
	hits := m.paletteHits()

	switch msg.String() {
	case "esc":
		m.closePalette()
		return nil
	case "up":
		if len(hits) > 0 {
			m.palIdx = (m.palIdx - 1 + len(hits)) % len(hits)
		}
		return nil
	case "down", "tab":
		if len(hits) > 0 {
			m.palIdx = (m.palIdx + 1) % len(hits)
		}
		return nil
	case "enter":
		if len(hits) == 0 {
			return nil
		}
		m.gotoHit(hits[clamp(m.palIdx, 0, len(hits)-1)])
		return nil
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.palIdx = 0 // the result list just changed underneath the cursor
	}
	return cmd
}

// gotoHit jumps to whatever was picked and closes the palette.
func (m *Model) gotoHit(h paletteHit) {
	m.revealGroup(h.kindIdx)
	m.selectResource(h.kindIdx)
	if h.row >= 0 {
		m.rowIdx = h.row
		m.rowMem[h.kind.Key] = h.row
		m.syncScroll()
		m.toast = "→ " + h.kind.Short + "/" + h.label
	} else {
		m.toast = "→ " + h.kind.Name
	}
	m.focus = focusMain
	m.closePalette()
}
