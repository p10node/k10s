package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"k10s/internal/theme"
)

// ---- theme picker: live preview while browsing ---------------------------

func (m *Model) openThemePicker() {
	m.themeOpen = true
	m.themeOrig = m.themeIdx
	m.themeRow = m.themeIdx
	m.themeSave = false
}

func (m *Model) handleThemeKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "shift+tab":
		if m.themeSave {
			m.themeSave = false
			m.themeRow = len(theme.Themes) - 1
		} else {
			m.themeRow = clamp(m.themeRow-1, 0, len(theme.Themes)-1)
		}
		m.previewTheme()
	case "down":
		if !m.themeSave {
			if m.themeRow >= len(theme.Themes)-1 {
				m.themeSave = true
			} else {
				m.themeRow = clamp(m.themeRow+1, 0, len(theme.Themes)-1)
				m.previewTheme()
			}
		}
	case "tab":
		// tab jumps straight to Save from anywhere in the list.
		m.themeSave = !m.themeSave
	case "enter":
		// Enter commits from anywhere: browsing already previewed the
		// theme, so confirming is the obvious next step whether the cursor
		// sits on a row or on the Save button.
		return m.saveTheme()
	case "esc", "q":
		// Cancel restores whatever was active before the picker opened.
		m.themeIdx = m.themeOrig
		m.themeOpen = false
		m.toast = "theme unchanged"
	}
	return nil
}

// previewTheme applies the highlighted theme immediately so the whole UI
// shows what it would look like before committing.
func (m *Model) previewTheme() {
	m.themeIdx = m.themeRow
}

func (m *Model) saveTheme() tea.Cmd {
	m.themeIdx = m.themeRow
	m.themeOpen = false
	m.themeSave = false
	m.saveConfig()
	m.toast = "theme → " + m.th().Name + " (saved)"
	return nil
}
