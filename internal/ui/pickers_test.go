package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/config"
	"github.com/p10node/k10s/internal/mock"
	"github.com/p10node/k10s/internal/theme"
)

// dismissOnboarding closes the first-run settings screen so a test can
// exercise whatever it actually cares about.
func dismissOnboarding(m *Model) {
	m.setOpen = false
	m.setEditing = false
	m.onboarded = true
}

func TestSettingsShownOnFirstRunOnly(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	if !m.setOpen {
		t.Fatal("first run should open the settings screen")
	}

	m.handleSettingsKey(key("esc"))
	if m.setOpen {
		t.Fatal("esc should close it")
	}

	m2 := New(mock.New(""))
	if m2.setOpen {
		t.Error("settings reappeared after being completed")
	}
	if m2.cli != m.cli {
		t.Errorf("cli not persisted: %q vs %q", m2.cli, m.cli)
	}
}

// The built-in names are stated, not chosen: all of them always work, so
// there is nothing to tick and no way to end up with none enabled.
func TestBuiltInCLINamesAlwaysWork(t *testing.T) {
	m := newTestModel(t, mock.New(""))

	for _, p := range config.CLIPresets {
		if !m.cliEnabled(p) {
			t.Errorf("%q should always be accepted", p)
		}
	}

	// Setting a custom name adds to the set rather than replacing it.
	m.setRow = rowCustom()
	m.handleSettingsKey(key("enter"))
	m.input.SetValue("kc")
	m.handleSettingsKey(key("enter"))

	for _, p := range config.CLIPresets {
		if !m.cliEnabled(p) {
			t.Errorf("%q stopped working after adding a custom name", p)
		}
	}
	if !m.cliEnabled("kc") {
		t.Error("the custom name should be accepted too")
	}
	if m.cli != "kc" {
		t.Errorf("cli = %q, want the custom name shown in hints", m.cli)
	}
}

func TestClearingTheCustomNameFallsBackToDefault(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.cli = "mine"
	m.syncCLINames()

	m.setRow = rowCustom()
	m.handleSettingsKey(key("enter"))
	m.input.SetValue("")
	m.handleSettingsKey(key("enter"))

	if m.cli != config.DefaultCLI {
		t.Errorf("cli = %q, want the default back", m.cli)
	}
	if m.cliEnabled("mine") {
		t.Error("the cleared custom name should no longer be accepted")
	}
}

// Every enabled name is accepted at the prompt.
func TestAllEnabledCLINamesWorkAtThePrompt(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	for _, name := range config.CLIPresets {
		m.runCommand(name + " get nodes")
		if got := m.curKind().Key; got != "nodes" {
			t.Errorf("%q was not recognised: kind = %q", name, got)
		}
		m.selectResource(0) // reset
	}
}

func TestSettingsTabReachesSave(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	for i := 0; i < setRows()+1; i++ {
		m.handleSettingsKey(key("tab"))
		if m.setRow == setSaveRow {
			return
		}
	}
	t.Fatal("tab never reached the Save button")
}

func TestSettingsCustomCLIValue(t *testing.T) {
	m := newTestModel(t, mock.New(""))

	m.setRow = rowCustom()
	m.handleSettingsKey(key("enter"))
	if !m.setEditing {
		t.Fatal("enter on the custom row should start editing")
	}
	m.input.SetValue("kc")
	m.handleSettingsKey(key("enter"))
	if m.cli != "kc" {
		t.Fatalf("cli = %q, want kc", m.cli)
	}
}

// The merged dialog must carry the AI fields that used to live in /config.
func TestSettingsHoldsAIFields(t *testing.T) {
	m := newTestModel(t, mock.New(""))

	m.setRow = rowURL()
	m.handleSettingsKey(key("enter"))
	if !m.setEditing {
		t.Fatal("enter on base url should start editing")
	}
	m.input.SetValue("https://example.test/v1")
	m.handleSettingsKey(key("enter"))
	if m.cfg.url != "https://example.test/v1" {
		t.Errorf("base url = %q, want the edited value", m.cfg.url)
	}

	m.setRow = rowModel()
	m.handleSettingsKey(key("enter"))
	m.input.SetValue("some-model")
	m.handleSettingsKey(key("enter"))
	if m.cfg.model != "some-model" {
		t.Errorf("model = %q", m.cfg.model)
	}

	before := m.cfg.provider
	m.setRow = rowProvider()
	m.handleSettingsKey(key("enter"))
	if m.cfg.provider == before {
		t.Error("enter on the provider row should toggle it")
	}
}

func TestSettingsAPIKeyNeverPreFilled(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.cfg.key = "sk-secret-value"

	m.setRow = rowKey()
	m.handleSettingsKey(key("enter"))
	if m.input.Value() != "" {
		t.Errorf("api key field pre-filled with %q — a secret must not be echoed back", m.input.Value())
	}

	// Leaving it empty keeps the existing key rather than wiping it.
	m.handleSettingsKey(key("enter"))
	if m.cfg.key != "sk-secret-value" {
		t.Errorf("api key = %q, want the previous value kept", m.cfg.key)
	}
}

func TestConfigCommandIsGone(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.runSlash(":config")
	if m.setOpen {
		t.Error(":config should no longer exist — it merged into /settings")
	}
	if !strings.Contains(m.toast, "unknown command") {
		t.Errorf("toast = %q, want an unknown-command message", m.toast)
	}
}

func TestCLINameUsedInCommandEcho(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.cli = "k"
	dismissOnboarding(m)

	m.runCommand("get pods")
	if !strings.Contains(m.toast, "$ k get pods") {
		t.Errorf("toast = %q, want it to use the configured CLI name", m.toast)
	}
}

func TestThemePickerPreviewsLiveAndCancels(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	start := m.themeIdx

	m.openThemePicker()
	m.handleThemeKey(key("down"))
	if m.themeIdx == start {
		t.Fatal("moving in the picker should apply the theme immediately (live preview)")
	}
	previewed := m.themeIdx

	m.handleThemeKey(key("esc"))
	if m.themeOpen {
		t.Fatal("esc should close the picker")
	}
	if m.themeIdx != start {
		t.Errorf("esc left theme at %d (previewed %d), want the original %d", m.themeIdx, previewed, start)
	}
}

func TestThemePickerSavesAndPersists(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.openThemePicker()
	m.handleThemeKey(key("down"))
	want := m.th().Name
	m.handleThemeKey(key("enter"))

	if m.themeOpen {
		t.Fatal("enter should close the picker")
	}
	if m.th().Name != want {
		t.Errorf("theme = %q, want %q", m.th().Name, want)
	}

	m2 := New(mock.New(""))
	if m2.th().Name != want {
		t.Errorf("reloaded theme = %q, want %q", m2.th().Name, want)
	}
}

func TestThemePickerTabTogglesSaveFocus(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.openThemePicker()
	if m.themeSave {
		t.Fatal("picker should open with the list focused")
	}
	m.handleThemeKey(key("tab"))
	if !m.themeSave {
		t.Error("tab should move focus to the Save button")
	}
}

func TestThemePickerDownStopsAtSave(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openThemePicker()

	for i := 0; i < len(theme.Themes)+3; i++ {
		m.handleThemeKey(key("down"))
	}
	if !m.themeSave {
		t.Error("holding down past the last theme should land on Save")
	}
	if m.themeRow != len(theme.Themes)-1 {
		t.Errorf("themeRow = %d, want it clamped to the last theme", m.themeRow)
	}
}

func TestSlashSuggestionsNavigateWithArrows(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	// openPrompt is how the prompt is really entered, and it focuses the
	// textinput — without that, typed keys are ignored.
	m.openPrompt("/")
	sug := m.suggestions()
	if len(sug) < 3 {
		t.Fatalf("expected several suggestions, got %d", len(sug))
	}

	if m.sugIdx != 0 {
		t.Fatalf("sugIdx should start at 0, got %d", m.sugIdx)
	}
	m.handleKey(key("down"))
	m.handleKey(key("down"))
	if m.sugIdx != 2 {
		t.Fatalf("after two downs sugIdx = %d, want 2", m.sugIdx)
	}
	m.handleKey(key("up"))
	if m.sugIdx != 1 {
		t.Fatalf("after up sugIdx = %d, want 1", m.sugIdx)
	}

	// Tab completes whatever is highlighted.
	want := sug[1].Name
	m.handleKey(key("tab"))
	if !strings.HasPrefix(m.input.Value(), want) {
		t.Errorf("tab completed to %q, want it to start with %q", m.input.Value(), want)
	}
}

func TestSuggestionIndexResetsWhenQueryChanges(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	// openPrompt is how the prompt is really entered, and it focuses the
	// textinput — without that, typed keys are ignored.
	m.openPrompt("/")
	m.handleKey(key("down"))
	m.handleKey(key("down"))
	if m.sugIdx == 0 {
		t.Fatal("precondition: expected a non-zero highlight")
	}

	// Typing narrows the list; a stale index could point past its end.
	m.handleKey(key("t"))
	if m.sugIdx != 0 {
		t.Errorf("sugIdx = %d after typing, want it reset to 0", m.sugIdx)
	}
}

func TestMouseToggleFlipsState(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	if m.mouseOff {
		t.Fatal("mouse capture should start enabled")
	}
	if cmd := m.toggleMouse(); cmd == nil {
		t.Error("toggling should return a command that tells bubbletea to disable the mouse")
	}
	if !m.mouseOff {
		t.Error("toggle did not record copy-mode")
	}
	if !strings.Contains(m.toast, "select") {
		t.Errorf("toast = %q, want it to explain that selection is now possible", m.toast)
	}

	m.toggleMouse()
	if m.mouseOff {
		t.Error("second toggle should re-enable mouse capture")
	}
}

func TestCtrlKNoLongerBound(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain

	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.focus != focusMain {
		t.Errorf("ctrl+k should do nothing now, focus = %v", m.focus)
	}
	if m.palOpen {
		t.Error("ctrl+k should not open the palette")
	}
}
