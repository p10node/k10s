package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/config"
)

// One settings modal covering what is persisted to ~/.k10s/config.yaml and
// still adjustable: the CLI name used in hints, and whether k10s looks for a
// newer release at startup. It also held the AI provider details until that
// feature was switched off (aidisabled.go).
//
// It opens only when asked (/settings). It used to open itself on first run
// as an onboarding screen, which put a form in front of the cluster you
// launched k10s to look at — every field here has a working default, so
// there was nothing that had to be answered before starting.

// Row layout. The built-in command names are not rows: all of them always
// work, so there is nothing to choose and a row of checkboxes only invited
// the question "what happens if I untick these?". The dialog states the
// fact instead, and offers one row for adding a name of your own.
//
// The AI provider fields (provider, base url, model, api key) used to sit
// between these two. They are gone while the feature is off (see
// aidisabled.go) — a dialog that offers to configure something unreachable
// is worse than one that doesn't mention it. The values stay in the config
// file untouched, so turning the feature back on restores them.
func setRows() int { return 2 } // custom name + update check

func rowCustom() int { return 0 }
func rowUpdate() int { return 1 }

const setSaveRow = -1 // sentinel: focus is on Save

func (m *Model) openSettings() {
	m.setOpen = true
	m.setEditing = false
	m.setRow = rowCustom()
}

func (m *Model) handleSettingsKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()

	// An inline text field has focus (the custom CLI name).
	if m.setEditing {
		switch key {
		case "enter", "tab":
			m.commitSettingField()
			m.setEditing = false
			m.input.Blur()
			if key == "tab" {
				m.setRow = setSaveRow
			}
			return nil
		case "esc":
			m.setEditing = false
			m.input.SetValue("")
			m.input.Blur()
			return nil
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return cmd
		}
	}

	switch key {
	case "up", "shift+tab":
		if m.setRow == setSaveRow {
			m.setRow = setRows() - 1
		} else {
			m.setRow = clamp(m.setRow-1, 0, setRows()-1)
		}
	case "down", "tab":
		// tab walks the list then lands on Save, so the button is always
		// reachable from the keyboard.
		if m.setRow >= setRows()-1 {
			m.setRow = setSaveRow
		} else {
			m.setRow = clamp(m.setRow+1, 0, setRows()-1)
		}
	case "left", "right":
		if m.setRow == rowUpdate() {
			m.setUpdateChecks(m.updDisabled)
		}
	case "enter":
		return m.activateSettingRow()
	case "esc":
		// Escaping keeps whatever is already set rather than trapping the
		// user in the dialog.
		return m.closeSettings()
	}
	return nil
}

// activateSettingRow is what enter does on the highlighted row.
func (m *Model) activateSettingRow() tea.Cmd {
	switch {
	case m.setRow == setSaveRow:
		return m.closeSettings()

	case m.setRow == rowUpdate():
		m.setUpdateChecks(m.updDisabled)
		return nil

	default:
		// A text field: start editing it.
		m.setEditing = true
		if m.setRow == rowCustom() {
			m.input.SetValue(customSeed(m.cli))
		}
		m.input.CursorEnd()
		// Focus alone doesn't animate the caret — Blink is what keeps it
		// pulsing, which is the difference between "is this editable?" and
		// an obvious text field.
		return tea.Batch(m.input.Focus(), textinput.Blink)
	}
}

// commitSettingField stores whatever was typed into the field being edited.
func (m *Model) commitSettingField() {
	v := strings.TrimSpace(m.input.Value())
	if m.setRow == rowCustom() {
		// Empty clears the custom name and falls back to the default.
		if v == "" {
			m.cli = config.DefaultCLI
		} else {
			m.cli = v
		}
		m.syncCLINames()
	}
	m.input.SetValue("")
}

func (m *Model) closeSettings() tea.Cmd {
	if m.setEditing {
		m.commitSettingField()
		m.setEditing = false
	}
	if m.cli == "" {
		m.cli = config.DefaultCLI
	}
	m.syncCLINames()
	m.setOpen = false
	m.input.SetValue("")
	m.input.Blur()
	m.saveConfig()
	m.toast = "settings saved → " + config.Path()
	return nil
}

// customSeed is what the custom-name field starts with. Pre-filling one of
// the presets would mean typing appends to it ("kubectl" + "kk"), so the
// field starts empty unless a custom name is already set and being edited.
func customSeed(cur string) string {
	if isPreset(cur) {
		return ""
	}
	return cur
}

// selectedCLI is the name the preview line shows.
func (m *Model) selectedCLI() string {
	if m.setEditing && m.setRow == rowCustom() {
		if v := strings.TrimSpace(m.input.Value()); v != "" {
			return v
		}
	}
	return m.cli
}

func isPreset(v string) bool {
	for _, p := range config.CLIPresets {
		if p == v {
			return true
		}
	}
	return false
}

// The prompt no longer strips a leading CLI name: a typed line is run as
// typed, so "kubectl get pods" has to reach kubectl intact. What the name
// still does is label the hints and the command echoes.

// cliEnabled reports whether a name is one k10s recognises.
func (m *Model) cliEnabled(name string) bool {
	return slices.Contains(m.clis, name)
}

// syncCLINames rebuilds the accepted set: the built-ins are always in, plus
// whatever custom name has been set. Nothing to toggle means nothing to get
// into a broken state.
func (m *Model) syncCLINames() {
	m.clis = append([]string(nil), config.CLIPresets...)
	if m.cli != "" && !isPreset(m.cli) {
		m.clis = append(m.clis, m.cli)
	}
}
