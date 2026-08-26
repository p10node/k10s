package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"k10s/internal/ai"
	"k10s/internal/config"
)

// One settings modal covering everything persisted to ~/.k10s/config.yaml:
// the CLI name used in hints, and the AI provider details. They used to be
// two separate dialogs (/settings and /config), which meant remembering
// which one held what.
//
// The same modal is the first-run onboarding, with a different title — a new
// user sees exactly the screen they will later return to.

// Row layout. The built-in command names are not rows: all of them always
// work, so there is nothing to choose and a row of checkboxes only invited
// the question "what happens if I untick these?". The dialog states the
// fact instead, and offers one row for adding a name of your own.
func setRows() int { return 5 } // custom name + provider + url + model + key

func rowCustom() int   { return 0 }
func rowProvider() int { return 1 }
func rowURL() int      { return 2 }
func rowModel() int    { return 3 }
func rowKey() int      { return 4 }

const setSaveRow = -1 // sentinel: focus is on Save

func (m *Model) openSettings() {
	m.setOpen = true
	m.setEditing = false
	m.setRow = rowCustom()
}

func (m *Model) handleSettingsKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()

	// An inline text field has focus (custom CLI name, URL, model, key).
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
		if m.setRow == rowProvider() {
			m.setProvider(1 - m.cfg.provider)
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

	case m.setRow == rowProvider():
		m.setProvider(1 - m.cfg.provider)
		return nil

	default:
		// A text field: start editing it.
		m.setEditing = true
		switch m.setRow {
		case rowCustom():
			m.input.SetValue(customSeed(m.cli))
		case rowURL():
			m.input.SetValue(m.cfg.url)
		case rowModel():
			m.input.SetValue(m.cfg.model)
		case rowKey():
			m.input.SetValue("") // never pre-fill a secret
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
	switch m.setRow {
	case rowCustom():
		// Empty clears the custom name and falls back to the default.
		if v == "" {
			m.cli = config.DefaultCLI
		} else {
			m.cli = v
		}
		m.syncCLINames()
	case rowURL():
		m.cfg.url = v
	case rowModel():
		m.cfg.model = v
	case rowKey():
		if v != "" {
			m.cfg.key = v
		}
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
	first := !m.onboarded
	m.onboarded = true
	m.setOpen = false
	m.input.SetValue("")
	m.input.Blur()
	m.saveConfig()

	if first {
		m.toast = "cli → " + m.cli + "   ·   change it any time with /settings"
	} else {
		m.toast = "settings saved → " + config.Path()
	}
	return nil
}

func (m *Model) setProvider(p int) {
	m.cfg.provider = p
	m.cfg.url = ai.Providers[p].URL
	m.cfg.model = ai.Providers[p].Model
	m.saveConfig()
	m.toast = "provider → " + ai.Providers[p].Label
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

// stripCLIPrefix removes a leading CLI name so every enabled alias is
// accepted at the prompt: "kubectl get pods", "k get pods" and
// "k8s get pods" are the same command.
func stripCLIPrefix(cmd string, names []string) string {
	head, rest, ok := strings.Cut(strings.TrimSpace(cmd), " ")
	if !ok {
		return cmd
	}
	for _, n := range names {
		if n != "" && head == n {
			return rest
		}
	}
	return cmd
}

// cliEnabled reports whether a name is accepted at the prompt.
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
