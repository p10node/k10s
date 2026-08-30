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
// dismissOnboarding is now only a guard: nothing opens a dialog on first
// run any more, so this just states that no modal is in the way.
func dismissOnboarding(m *Model) {
	m.setOpen = false
	m.setEditing = false
	m.onboarded = true
	m.firstRun = false
}

// First run opens into the cluster, not into a form. Every setting has a
// working default, so a dialog in front of the thing you launched is only a
// thing to dismiss.
func TestFirstRunOpensTheClusterNotADialog(t *testing.T) {
	m := newTestModel(t, mock.New(""))

	if m.setOpen {
		t.Error("first run should not open the settings screen")
	}
	if m.modalOpen() {
		t.Error("first run should not open any modal")
	}
	if m.cli != config.DefaultCLI {
		t.Errorf("cli = %q, want the default %q", m.cli, config.DefaultCLI)
	}
	if !m.firstRun {
		t.Error("the session should know it is the first, to say so once")
	}

	// It is recorded straight away, so the hint is not repeated next time.
	c, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !c.Onboarded {
		t.Error("the first run was not recorded")
	}
	if m2 := New(mock.New("")); m2.firstRun || m2.setOpen {
		t.Error("the second run should be silent about settings")
	}
}

// /settings still opens the same dialog, and it is the only way in now.
func TestSettingsOpensOnDemand(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.runSlash("/settings")
	if !m.setOpen {
		t.Fatal("/settings should open the settings screen")
	}
	m.handleSettingsKey(key("esc"))
	if m.setOpen {
		t.Error("esc should close it")
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
// A typed line runs as typed: the CLI name is part of the command now, not a
// prefix to be stripped before guessing what was meant. (The returned tea.Cmd
// is deliberately never invoked — this asserts what would run, without
// running kubectl against whatever cluster the test machine can reach.)
func TestTypedCLILinesRunVerbatim(t *testing.T) {
	for _, name := range config.CLIPresets {
		m := newTestModel(t, mock.New(""))
		dismissOnboarding(m)

		line := name + " get nodes"
		if cmd := m.runCommand(line); cmd == nil {
			t.Fatalf("%q produced no command to run", line)
		}
		if !strings.Contains(m.busyLabel, "$ "+line) {
			t.Errorf("busy label = %q, want the whole line %q", m.busyLabel, line)
		}
		if got := m.curKind().Key; got == "nodes" {
			t.Errorf("%q also jumped the sidebar — that guesswork belongs to :no now", line)
		}
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

// While the AI prompt is off, the dialog must not offer to configure it —
// an API key field for a feature that cannot be reached is just a place to
// leave a secret for nothing.
func TestSettingsHidesAIFieldsWhileDisabled(t *testing.T) {
	if !aiDisabled {
		t.Skip("AI is enabled again — this dialog should carry its fields")
	}
	m := newTestModel(t, mock.New(""))
	m.openSettings()

	if setRows() != 2 {
		t.Errorf("setRows() = %d, want 2 (custom name + update check)", setRows())
	}
	view := stripANSI(m.View())
	for _, banned := range []string{"AI PROMPT", "api key", "base url"} {
		if strings.Contains(view, banned) {
			t.Errorf("settings still shows %q", banned)
		}
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

// The prompt says what it is running, so the spinner is never anonymous.
func TestPromptAnnouncesWhatItRuns(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.runCommand("date")
	if !m.busy {
		t.Error("running a command should show the busy spinner")
	}
	if !strings.Contains(m.busyLabel, "$ date") || !strings.Contains(m.toast, "$ date") {
		t.Errorf("busy = %q, toast = %q, want both to name the command", m.busyLabel, m.toast)
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
