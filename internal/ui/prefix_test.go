package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"k10s/internal/domain"
	"k10s/internal/mock"
)

// ---- two command prefixes ------------------------------------------------

func TestEachPrefixListsItsOwnCommands(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.openPrompt("/")
	for _, c := range m.suggestions() {
		if !strings.HasPrefix(c.Name, "/") {
			t.Errorf("%q offered under the / prefix", c.Name)
		}
	}
	if len(m.suggestions()) == 0 {
		t.Fatal("/ offered no commands")
	}

	m.openPrompt(":")
	for _, c := range m.suggestions() {
		if !strings.HasPrefix(c.Name, ":") {
			t.Errorf("%q offered under the : prefix", c.Name)
		}
	}
	if len(m.suggestions()) == 0 {
		t.Fatal(": offered no commands")
	}
}

func TestColonPrefillsAndOpensPopup(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain

	m.handleKey(key(":"))
	if m.focus != focusPrompt {
		t.Fatalf("\":\" should open the prompt, focus = %v", m.focus)
	}
	if m.input.Value() != ":" {
		t.Errorf("prompt = %q, want it pre-filled with \":\"", m.input.Value())
	}
	if len(m.suggestions()) == 0 {
		t.Error("\":\" should show its own command popup")
	}
}

func TestAppCommandsMovedToColon(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.runSlash(":search nodes")
	if got := m.curKind().Key; got != "nodes" {
		t.Errorf(":search did not work: kind = %q", got)
	}

	// The old spellings are gone, not silently aliased.
	m2 := newTestModel(t, mock.New(""))
	dismissOnboarding(m2)
	m2.runSlash("/search nodes")
	if got := m2.curKind().Key; got == "nodes" {
		t.Error("/search should no longer exist — it moved to :search")
	}
	if !strings.Contains(m2.toast, "unknown command") {
		t.Errorf("toast = %q, want an unknown-command message", m2.toast)
	}
}

func TestCommandSetsAreDisjoint(t *testing.T) {
	for _, c := range clusterCommands {
		if !strings.HasPrefix(c.Name, "/") {
			t.Errorf("cluster command %q should start with /", c.Name)
		}
	}
	for _, c := range appCommands {
		if !strings.HasPrefix(c.Name, ":") {
			t.Errorf("app command %q should start with :", c.Name)
		}
	}
}

// ---- pickers instead of typed names --------------------------------------

func TestThemeAndContextCommandsOpenPickers(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.runSlash("/theme")
	if !m.themeOpen {
		t.Error("/theme should open the picker")
	}
	m.themeOpen = false

	m.runSlash("/context")
	if m.mode != modeContexts {
		t.Error("/context should show the context list in the main panel")
	}
}

func TestContextChooserOpensInMainPanel(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	start := m.src.ClusterInfo().Context

	m.showContextChooser()
	if m.mode != modeContexts {
		t.Fatalf("mode = %v, want the context list in the main panel", m.mode)
	}
	if m.modalOpen() {
		t.Error("the context chooser should not be a popup any more")
	}
	choices := m.ctxChoices()
	if len(choices) < 2 {
		t.Skip("demo backend has too few contexts to switch between")
	}
	if choices[m.ctxIdx] != start {
		t.Errorf("opened on %q, want the current context %q", choices[m.ctxIdx], start)
	}

	// Move to any entry that isn't the current one — with the list sorted,
	// the current context can be anywhere in it.
	m.ctxIdx = 0
	if choices[0] == start {
		m.ctxIdx = 1
	}
	cmd := m.openSelected()
	if m.mode != modeTable {
		t.Error("choosing a context should return to the table")
	}
	if cmd == nil {
		t.Fatal("selecting a different context should start a switch")
	}
	if msg := cmd(); IsAsyncMsg(msg) {
		m.Update(msg)
	}
	if m.src.ClusterInfo().Context == start {
		t.Errorf("context did not change from %q", start)
	}
}

func TestContextChooserFilters(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.showContextChooser()

	all := len(m.ctxChoices())
	m.ctxFilter = "ek"
	filtered := m.ctxChoices()
	if len(filtered) >= all {
		t.Errorf("filtering by \"ek\" did not narrow: %d -> %d", all, len(filtered))
	}
	for _, c := range filtered {
		if !strings.Contains(strings.ToLower(c), "ek") {
			t.Errorf("%q does not match the filter", c)
		}
	}
}

// ---- namespace list in the main panel ------------------------------------

func TestEnterOnNamespaceSwitchesAndShowsPods(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.jumpToResource("namespaces")
	if m.curKind().Key != "namespaces" {
		t.Fatal("failed to open the namespaces table")
	}
	// Move onto a namespace that isn't the current one.
	for m.curName() == m.namespace && m.rowIdx < 10 {
		m.move(1)
	}
	want := m.curName()

	m.openSelected()

	if m.namespace != want {
		t.Errorf("namespace = %q, want %q", m.namespace, want)
	}
	if got := m.curKind().Key; got != "pods" {
		t.Errorf("after choosing a namespace, kind = %q, want pods", got)
	}
}

func TestNamespaceButtonOpensNamespacesTable(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	// The header button is rendered by View; drive the same effect the
	// click handler produces.
	m.jumpToResource("namespaces")
	if m.curKind().Key != "namespaces" {
		t.Errorf("kind = %q, want namespaces", m.curKind().Key)
	}
	view := m.View()
	if !strings.Contains(view, "ns ") {
		t.Error("header should carry a namespace button")
	}
}

// ---- double click --------------------------------------------------------

func clickAt(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func TestDoubleClickOpensRow(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.View() // register zones

	l := m.layout()
	x, y := l.leftW+5, l.midY+3

	// First click selects only.
	m.handleMouse(clickAt(x, y))
	if m.mode != modeTable {
		t.Fatal("a single click should not open the row")
	}

	// Second click within the window opens it.
	cmd := m.handleMouse(clickAt(x, y))
	if cmd == nil {
		t.Fatal("double click should open the row")
	}
	drainCmd(m, cmd)
	if m.mode == modeTable {
		t.Error("double click should have opened the row")
	}
}

func TestSlowSecondClickDoesNotOpen(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.View()

	l := m.layout()
	x, y := l.leftW+5, l.midY+3

	m.handleMouse(clickAt(x, y))
	// Simulate the user pausing longer than the double-click window.
	m.lastClickAt = time.Now().Add(-2 * doubleClickWindow)
	if cmd := m.handleMouse(clickAt(x, y)); cmd != nil {
		t.Error("two slow clicks should not count as a double click")
	}
}

// ---- action hover and click flash ---------------------------------------

func TestActionFlashOnClickAndClears(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	cmd := m.flashAction("describe")
	if m.flashAct != "describe" {
		t.Fatal("clicking an action should light it")
	}
	if !strings.Contains(m.viewActions(26, 26).String(), "Describe") {
		t.Error("the flashed action should still render")
	}

	// The timer message clears it.
	msg := cmd()
	m.Update(msg)
	if m.flashAct != "" {
		t.Error("the flash should clear when its timer fires")
	}
}

func TestStaleFlashTimerDoesNotClearNewerFlash(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	old := m.flashAction("describe")
	m.flashAction("yaml") // a second click before the first timer lands

	m.Update(old())
	if m.flashAct != "yaml" {
		t.Errorf("flashAct = %q, want the newer flash to survive the stale timer", m.flashAct)
	}
}

func TestHoverTracksPointer(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.View() // register zones

	l := m.layout()
	// Motion over the first action row.
	m.handleMouse(tea.MouseMsg{
		X: l.leftW + l.mainW + 3, Y: l.midY + 3,
		Action: tea.MouseActionMotion, Button: tea.MouseButtonNone,
	})
	if m.hoverAct == "" {
		t.Skip("action zones not resolvable at this geometry")
	}

	// Moving away clears it.
	m.handleMouse(tea.MouseMsg{
		X: 1, Y: l.midY + 3,
		Action: tea.MouseActionMotion, Button: tea.MouseButtonNone,
	})
	if m.hoverAct != "" {
		t.Errorf("hoverAct = %q, want it cleared when the pointer leaves", m.hoverAct)
	}
}

// ---- find hint -----------------------------------------------------------

func TestFindHintShownNextToZoom(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	view := m.viewMain(90, 20).String()
	if !strings.Contains(view, "to search") {
		t.Error("the main panel should advertise the find key next to zoom")
	}

	// While the search box is open the hint would be redundant.
	m.focus = focusMainSearch
	view = m.viewMain(90, 20).String()
	if strings.Contains(view, "to search") {
		t.Error("the hint should go away once the search box is open")
	}
}

// ---- enter in the popup runs the command ---------------------------------

func TestEnterInPopupRunsCommandImmediately(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	// "/th" highlights /theme; one enter should open the picker, with no
	// second trip through the prompt.
	m.openPrompt("/")
	m.input.SetValue("/th")
	if len(m.suggestions()) == 0 {
		t.Fatal("expected /theme to be suggested")
	}
	m.handleKey(key("enter"))

	if !m.themeOpen {
		t.Error("enter in the popup should run the highlighted command outright")
	}
	if m.input.Value() != "" {
		t.Errorf("prompt = %q, want it cleared after running", m.input.Value())
	}
}

func TestEnterFillsCommandsNeedingAnArgument(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	// :search takes a term, so enter should fill it in rather than run it
	// with nothing.
	m.openPrompt(":")
	m.input.SetValue(":sea")
	m.handleKey(key("enter"))

	if !strings.HasPrefix(m.input.Value(), ":search") {
		t.Errorf("prompt = %q, want it completed to :search ", m.input.Value())
	}
	if m.focus != focusPrompt {
		t.Error("focus should stay in the prompt so the argument can be typed")
	}
}

func TestEnterRunsOnceTheArgumentIsThere(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.openPrompt(":")
	m.input.SetValue(":search nodes")
	m.handleKey(key("enter"))

	if got := m.curKind().Key; got != "nodes" {
		t.Errorf("kind = %q, want nodes — the command should have run", got)
	}
}

// ---- removed commands ----------------------------------------------------

func TestRemovedCommandsAreGone(t *testing.T) {
	for _, name := range []string{"/ai", "/crd", "/dr", ":config", ":settings", ":theme", ":help"} {
		for _, c := range SlashCommands {
			if c.Name == name {
				t.Errorf("%q should have been removed or moved", name)
			}
		}
	}
	for _, name := range []string{"/settings", "/theme", "/help", "/ns", "/context"} {
		found := false
		for _, c := range SlashCommands {
			if c.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("%q should exist under the / prefix", name)
		}
	}
}

func TestRemovedCommandsToastUnknown(t *testing.T) {
	for _, cmd := range []string{"/ai hello", "/crd", "/dr"} {
		m := newTestModel(t, mock.New(""))
		dismissOnboarding(m)
		m.runSlash(cmd)
		if !strings.Contains(m.toast, "unknown command") {
			t.Errorf("%q: toast = %q, want an unknown-command message", cmd, m.toast)
		}
	}
}

// The context list is rebuilt on every render as well as on every keypress,
// so an unstable order would move the highlighted row under the cursor.
func TestContextChoicesAreStableAcrossCalls(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.showContextChooser()

	first := m.ctxChoices()
	if len(first) < 2 {
		t.Skip("need at least two contexts")
	}
	for i := 0; i < 50; i++ {
		got := m.ctxChoices()
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("call %d reordered the list: %v vs %v", i, got, first)
			}
		}
	}
}

func TestContextChoicesAreSorted(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	got := m.ctxChoices()
	for i := 1; i < len(got); i++ {
		if domain.NaturalLess(got[i], got[i-1]) {
			t.Errorf("contexts out of order at %d: %q before %q", i, got[i-1], got[i])
		}
	}
}

// Moving the cursor must land on the neighbouring entry, not somewhere
// random — the symptom an unsorted list produces.
func TestContextCursorMovesOneStepAtATime(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.showContextChooser()

	choices := m.ctxChoices()
	if len(choices) < 3 {
		t.Skip("need at least three contexts")
	}
	m.ctxIdx = 0
	for i := 1; i < len(choices); i++ {
		m.move(1)
		if m.ctxIdx != i {
			t.Fatalf("after %d moves ctxIdx = %d, want %d", i, m.ctxIdx, i)
		}
		if m.ctxChoices()[m.ctxIdx] != choices[i] {
			t.Fatalf("cursor landed on %q, want %q", m.ctxChoices()[m.ctxIdx], choices[i])
		}
	}
}

// ---- theme button opens the picker ---------------------------------------

func TestThemeButtonOpensPicker(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.View() // register zones

	before := m.themeIdx
	if !clickZone(m, "theme") {
		t.Skip("theme zone not resolvable at this geometry")
	}
	if !m.themeOpen {
		t.Error("clicking the theme button should open the picker, not cycle")
	}
	if m.themeIdx != before {
		t.Error("clicking should not change the theme on its own")
	}
}

// clickZone clicks the middle of a named zone, reporting whether it existed.
func clickZone(m *Model, id string) bool {
	z := zone.Get(id)
	if z == nil {
		return false
	}
	x, y := z.StartX, z.StartY
	if x == 0 && y == 0 {
		return false
	}
	m.handleMouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	return true
}

// ---- :scale ---------------------------------------------------------------

func TestScaleMovedToColonPrefix(t *testing.T) {
	for _, c := range SlashCommands {
		if c.Name == "/scale" {
			t.Error("/scale should have moved to :scale")
		}
	}
	found := false
	for _, c := range appCommands {
		if c.Name == ":scale" {
			found = true
		}
	}
	if !found {
		t.Error(":scale should be in the : set")
	}
}

func TestScaleActionPrefillsColonForm(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	for i, k := range m.kinds() {
		if k.Key == "deployments" {
			m.selectResource(i)
		}
	}
	for _, a := range Actions {
		if a.ID == domain.AScale {
			m.fireAction(a)
		}
	}
	if !strings.HasPrefix(m.input.Value(), ":scale ") {
		t.Errorf("prompt = %q, want it pre-filled with :scale", m.input.Value())
	}
}

// ---- prompt zoom ----------------------------------------------------------

func TestPlainCommandGrowsThePrompt(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	small := m.layout().promptH

	m.openPrompt("")
	for _, r := range "get pods" {
		m.handleKey(key(string(r)))
	}
	if !m.promptZoom {
		t.Fatal("typing a plain command should grow the prompt")
	}
	if m.layout().promptH <= small {
		t.Errorf("prompt height %d did not grow from %d", m.layout().promptH, small)
	}
	if m.layout().promptH < m.h/2-1 {
		t.Errorf("prompt height %d, want about half of %d", m.layout().promptH, m.h)
	}
}

func TestSlashCommandDoesNotGrowThePrompt(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.openPrompt("/")
	m.handleKey(key("n"))
	if m.promptZoom {
		t.Error("a / command should keep the small box — the popup is the point")
	}
}

func TestEscShrinksThenLeaves(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.openPrompt("")
	m.handleKey(key("g"))
	if !m.promptZoom {
		t.Fatal("precondition: the box should have grown")
	}

	m.handleKey(key("esc"))
	if m.promptZoom {
		t.Error("the first esc should shrink the box")
	}
	if m.focus != focusPrompt {
		t.Error("the first esc should not also leave the prompt")
	}

	m.handleKey(key("esc"))
	if m.focus != focusMain {
		t.Error("the second esc should leave the prompt")
	}
}

func TestZoomedPromptWrapsTheCommand(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.openPrompt("")
	m.promptZoom = true
	m.input.SetValue(strings.Repeat("kubectl describe pod some-very-long-name ", 6))

	body := m.zoomedPromptBody(" ❯ ", 60, 12)
	if len(body) < 3 {
		t.Fatalf("expected a tall body, got %d rows", len(body))
	}
	// The continuation rows must carry the rest of the command.
	joined := stripANSI(strings.Join(body, "\n"))
	if !strings.Contains(joined, "some-very-long-name") {
		t.Error("the zoomed prompt should show the wrapped command text")
	}
}
