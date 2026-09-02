package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

// ---- key remap: "/" is the prompt, "f" is find ---------------------------

func TestSlashOpensPromptFromTable(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain
	m.mode = modeTable

	m.handleKey(key("/"))
	if m.focus != focusPrompt {
		t.Fatalf("\"/\" should open the command prompt, focus = %v", m.focus)
	}
	if m.input.Value() != "/" {
		t.Errorf("prompt = %q, want it pre-filled with \"/\"", m.input.Value())
	}
}

func TestFOpensRowSearch(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain
	m.mode = modeTable

	m.handleKey(key("f"))
	if m.focus != focusMainSearch {
		t.Fatalf("\"f\" should focus the row-search box, focus = %v", m.focus)
	}
}

func TestPortForwardMovedToP(t *testing.T) {
	var fKey, pKey string
	for _, a := range Actions {
		if a.ID == domain.APortFwd {
			pKey = a.Key
		}
		if a.Key == "f" {
			fKey = a.ID
		}
	}
	if pKey != "p" {
		t.Errorf("port-forward key = %q, want p", pKey)
	}
	if fKey != "" {
		t.Errorf("action %q still bound to f, which is now the find key", fKey)
	}
}

// ---- shortcuts keep working inside a search box --------------------------

func TestSearchBoxAcceptsGlobalShortcuts(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.focus = focusMainSearch
	m.handleKey(key("ctrl+p"))
	if !m.palOpen {
		t.Error("ctrl+p in the search box should open the global search palette")
	}

	m.palOpen = false
	m.focus = focusMainSearch
	m.handleKey(key("ctrl+s"))
	if !m.mouseOff {
		t.Error("ctrl+s should work while typing a row filter")
	}
}

// Tab walks Resources → Main → Command box and wraps.
func TestTabCyclesResourcesMainPrompt(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain

	want := []focusPane{focusPrompt, focusList, focusMain, focusPrompt}
	for i, w := range want {
		m.handleKey(key("tab"))
		if m.focus != w {
			t.Fatalf("tab #%d landed on %v, want %v", i+1, m.focus, w)
		}
	}
}

func TestShiftTabWalksTheCycleBackwards(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain

	want := []focusPane{focusList, focusPrompt, focusMain, focusList}
	for i, w := range want {
		m.handleKey(key("shift+tab"))
		if m.focus != w {
			t.Fatalf("shift+tab #%d landed on %v, want %v", i+1, m.focus, w)
		}
	}
}

// Getting back to where you started must take exactly one lap.
func TestTabCycleReturnsToStart(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain

	for range tabOrder {
		m.handleKey(key("tab"))
	}
	if m.focus != focusMain {
		t.Errorf("a full lap ended on %v, want focusMain", m.focus)
	}
}

// The resource pane is focusable again, and typing there filters the list.
func TestResourcePaneIsFocusableAndFilters(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusList

	m.handleKey(key("n"))
	m.handleKey(key("o"))
	if m.search != "no" {
		t.Fatalf("list filter = %q, want no", m.search)
	}
	if m.curKind().Key != "nodes" {
		t.Errorf("selection did not follow the filter: kind = %q", m.curKind().Key)
	}

	m.handleKey(key("esc"))
	if m.search != "" {
		t.Error("esc should clear the list filter")
	}
}

// The Actions pane stays out of the cycle: every action has a hotkey and a
// clickable row, so a tab stop there would lead nowhere.
func TestTabNeverFocusesActionsPane(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain

	for i := 0; i < 9; i++ {
		m.handleKey(key("tab"))
		if m.focus == focusActions {
			t.Fatalf("tab #%d focused the actions pane", i+1)
		}
	}
}

func TestContextChooserBlocksEveryResourceActionHotkey(t *testing.T) {
	for _, action := range Actions {
		t.Run(action.ID, func(t *testing.T) {
			m := newTestModel(t, mock.New(""))
			dismissOnboarding(m)
			m.showContextChooser()

			cmd := m.handleKey(key(action.Key))
			if cmd != nil {
				t.Fatalf("%q returned a command while choosing a context", action.Key)
			}
			if m.confirm != nil {
				t.Fatalf("%q opened a resource confirmation while choosing a context: %+v", action.Key, m.confirm)
			}
			if m.mode != modeContexts {
				t.Fatalf("%q left the context chooser, mode = %v", action.Key, m.mode)
			}
		})
	}
}

func TestContextChooserDoesNotRenderTheHiddenResourcesActions(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	hiddenName := m.curName()
	m.showContextChooser()

	view := m.viewActions(24, 20).String()
	if strings.Contains(view, hiddenName) {
		t.Fatalf("context chooser exposed the hidden resource %q in the Actions pane", hiddenName)
	}
	if !strings.Contains(view, "actions paused") {
		t.Fatalf("context Actions pane did not explain that resource actions are paused:\n%s", view)
	}
}

func TestSearchBoxTabReachesPrompt(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMainSearch

	m.handleKey(key("tab"))
	if m.focus != focusPrompt {
		t.Errorf("tab from the row-search box should reach the command box, focus = %v", m.focus)
	}
}

func TestSearchBoxStillTypesPlainLetters(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMainSearch

	for _, r := range "web" {
		m.handleKey(key(string(r)))
	}
	if m.rowSearch != "web" {
		t.Fatalf("rowSearch = %q, want web — plain letters must still be search text", m.rowSearch)
	}
	if m.focus != focusMainSearch {
		t.Errorf("typing should not move focus, focus = %v", m.focus)
	}
}

func TestListSearchAcceptsShortcutsAndLetters(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusList

	m.handleKey(key("p"))
	m.handleKey(key("o"))
	if m.search != "po" {
		t.Fatalf("list search = %q, want po", m.search)
	}

	m.handleKey(key("ctrl+s"))
	if !m.mouseOff {
		t.Error("ctrl+s should work while typing in the resource-list filter")
	}
}

// ---- wheel scrolls the pane under the pointer ----------------------------

func wheel(x, y int, up bool) tea.MouseMsg {
	btn := tea.MouseButtonWheelDown
	if up {
		btn = tea.MouseButtonWheelUp
	}
	return tea.MouseMsg{X: x, Y: y, Button: btn, Action: tea.MouseActionPress}
}

func TestWheelScrollsOnlyTheCentrePane(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	l := m.layout()
	midY := l.midY + 2

	// The resource pane ignores the wheel: scrolling it changes the whole
	// view, which was too easy to trigger while reaching for the table.
	resBefore, rowBefore := m.resIdx, m.rowIdx
	m.handleMouse(wheel(1, midY, false))
	if m.resIdx != resBefore {
		t.Error("the wheel should not change the resource selection")
	}
	if m.rowIdx != rowBefore {
		t.Error("scrolling the resource pane should not move the table either")
	}

	// The centre pane still scrolls.
	m.handleMouse(wheel(l.leftW+5, midY, false))
	if m.rowIdx == rowBefore {
		t.Error("wheel over the table did not scroll the table")
	}
}

func TestWheelOutsidePanesDoesNothing(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	before := m.rowIdx

	m.handleMouse(wheel(10, 0, false))     // header
	m.handleMouse(wheel(10, m.h-1, false)) // status bar
	if m.rowIdx != before {
		t.Errorf("wheel outside the panes changed the selection (%d → %d)", before, m.rowIdx)
	}
}

// Scrolling a side pane neither acts on it nor focuses it.
func TestWheelOverSidePaneIsInert(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain
	l := m.layout()

	before := m.resIdx
	m.handleMouse(wheel(1, l.midY+2, false))
	if m.resIdx != before {
		t.Error("the resource pane should not scroll")
	}
	if m.focus != focusMain {
		t.Errorf("scrolling a side pane must not focus it, focus = %v", m.focus)
	}
}

// Clicking blank space in the centre pane selects it; clicking the side
// panes never moves focus there.
func TestClickBlankSpaceFocusesMainOnly(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	l := m.layout()

	click := func(x, y int) tea.MouseMsg {
		return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	}

	m.focus = focusPrompt
	m.handleMouse(click(l.leftW+5, l.midY+l.midH-2))
	if m.focus != focusMain {
		t.Errorf("clicking blank space in the table should focus it, focus = %v", m.focus)
	}

	for _, x := range []int{1, l.leftW + l.mainW + 2} {
		m.focus = focusMain
		m.handleMouse(click(x, l.midY+l.midH-2))
		if m.focus != focusMain {
			t.Errorf("clicking a side pane at x=%d moved focus to %v", x, m.focus)
		}
	}
}

// Clicking a resource row still switches kind — it just doesn't focus the
// pane it lives in.
func TestClickResourceRowSelectsWithoutFocusing(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.View() // register zones
	m.focus = focusMain

	l := m.layout()
	before := m.curKind().Key
	// Walk down the resource pane looking for a row that changes the kind.
	for y := l.midY + 1; y < l.midY+10; y++ {
		m.handleMouse(tea.MouseMsg{X: 3, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.curKind().Key != before {
			break
		}
	}
	if m.curKind().Key == before {
		t.Skip("no resource row hit at this geometry")
	}
	if m.focus != focusMain {
		t.Errorf("clicking a resource row moved focus to %v", m.focus)
	}
}

func TestClickOutsidePanesLeavesFocus(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain

	// The status bar is not a pane.
	m.handleMouse(tea.MouseMsg{X: 5, Y: m.h - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.focus != focusMain {
		t.Errorf("clicking outside the panes should not change focus, focus = %v", m.focus)
	}
}

// ---- node-only actions are hidden elsewhere -----------------------------

func TestCordonDrainHiddenForNonNodes(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	// Pods are selected by default.
	if got := m.curKind().Key; got != "pods" {
		t.Fatalf("expected pods to be selected, got %q", got)
	}
	view := m.viewActions(24, 24).String()
	for _, label := range []string{"Cordon", "Drain"} {
		if strings.Contains(view, label) {
			t.Errorf("%q should not appear in the actions pane for pods", label)
		}
	}

	// Nodes should show them.
	for i, k := range m.kinds() {
		if k.Key == "nodes" {
			m.selectResource(i)
		}
	}
	view = m.viewActions(24, 24).String()
	for _, label := range []string{"Cordon", "Drain"} {
		if !strings.Contains(view, label) {
			t.Errorf("%q should appear in the actions pane for nodes", label)
		}
	}
}

// ---- row numbers --------------------------------------------------------

func TestTableShowsRowNumbers(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	// The first rows should carry their 1-based index in the gutter.
	nums := gutterNumbers(m, 100, 12)
	for i, want := range []string{"1", "2", "3"} {
		if i >= len(nums) {
			t.Fatalf("only %d rows rendered", len(nums))
		}
		if nums[i] != want {
			t.Errorf("gutter row %d = %q, want %q", i, nums[i], want)
		}
	}
}

// ---- loading state ------------------------------------------------------

// loadingSource reports a kind as never synced, mimicking a cluster whose
// informer is still doing its initial list.
type loadingSource struct {
	domain.Source
}

func (loadingSource) Synced(string) bool { return false }

func (l loadingSource) Rows(kind, ns string) ([]string, [][]string) {
	cols, _ := l.Source.Rows(kind, ns)
	return cols, nil // nothing cached yet
}

func (loadingSource) RowCount(string, string) int { return 7 } // last known

func TestLoadingShowsSpinnerNotEmptyMessage(t *testing.T) {
	m := newTestModel(t, loadingSource{mock.New("")})
	dismissOnboarding(m)

	body := strings.Join(m.tableBody(100, 12), "\n")
	if strings.Contains(body, "no resources found") {
		t.Error("a still-loading table must not claim there are no resources")
	}
	if !strings.Contains(body, "loading") {
		t.Errorf("expected a loading indicator, got:\n%s", body)
	}
}

func TestLoadingKeepsSidebarCount(t *testing.T) {
	m := newTestModel(t, loadingSource{mock.New("")})
	dismissOnboarding(m)

	list := m.viewList(24, 24).String()
	if !strings.Contains(list, "7") {
		t.Errorf("sidebar should keep showing the last known count while loading, got:\n%s", list)
	}
}

type failedLoadingSource struct{ loadingSource }

func (failedLoadingSource) LoadErrorFor(string, string) error {
	return errors.New("pods is forbidden: cannot list resource pods at the cluster scope")
}

func TestLoadErrorReplacesIndefiniteSpinner(t *testing.T) {
	m := newTestModel(t, failedLoadingSource{loadingSource{mock.New("")}})
	dismissOnboarding(m)

	body := strings.Join(m.tableBody(100, 14), "\n")
	if !strings.Contains(body, "unable to load pods") || !strings.Contains(body, "forbidden") {
		t.Fatalf("load failure is not explained in the table:\n%s", body)
	}
	if strings.Contains(body, "loading pods") {
		t.Fatalf("failed initial list still renders an indefinite spinner:\n%s", body)
	}
	if strings.Contains(body, "no resources found") {
		t.Fatalf("failed initial list is being reported as an empty result:\n%s", body)
	}
}

type loadedEmptyScopedSource struct{ domain.Source }

func (loadedEmptyScopedSource) Synced(string) bool            { return false }
func (loadedEmptyScopedSource) SyncedFor(string, string) bool { return true }
func (s loadedEmptyScopedSource) Rows(kind, ns string) ([]string, [][]string) {
	cols, _ := s.Source.Rows(kind, ns)
	return cols, nil
}

func TestScopedLoadedEmptyViewDoesNotRenderSpinner(t *testing.T) {
	m := newTestModel(t, loadedEmptyScopedSource{mock.New("")})
	dismissOnboarding(m)
	m.jumpToResource("customresources")

	body := strings.Join(m.tableBody(100, 12), "\n")
	if strings.Contains(body, "loading custom resources") {
		t.Fatalf("loaded empty custom-resource view still renders a spinner:\n%s", body)
	}
	if !strings.Contains(body, "no resources found") {
		t.Fatalf("loaded empty custom-resource view has no stable empty state:\n%s", body)
	}
}

func TestSpinnerAdvancesWithTick(t *testing.T) {
	m := newTestModel(t, loadingSource{mock.New("")})
	dismissOnboarding(m)

	first := strings.Join(m.loadingLines(60), "\n")
	m.Update(tickMsg{})
	second := strings.Join(m.loadingLines(60), "\n")
	if first == second {
		t.Error("the spinner should advance on each repaint tick")
	}
}

func TestBouncePingPongs(t *testing.T) {
	// A bouncing indeterminate bar must reverse rather than jump back.
	span := 4
	var seq []int
	for i := 0; i <= span*2; i++ {
		seq = append(seq, bounce(i, span))
	}
	want := []int{0, 1, 2, 3, 4, 3, 2, 1, 0}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("bounce sequence = %v, want %v", seq, want)
		}
	}
	if bounce(5, 0) != 0 {
		t.Error("bounce with zero span should stay at 0")
	}
}

// ---- j/k are no longer movement -----------------------------------------

func TestJKNoLongerMoveTheSelection(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain

	before := m.rowIdx
	m.handleKey(key("j"))
	if m.rowIdx != before {
		t.Error("j should no longer move the selection")
	}
}

// "k" is a command name, so it opens the prompt already typed.
func TestKOpensThePromptPreTyped(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusMain

	m.handleKey(key("k"))
	if m.focus != focusPrompt {
		t.Fatalf("k should focus the command box, focus = %v", m.focus)
	}
	if m.input.Value() != "k" {
		t.Errorf("prompt = %q, want it seeded with \"k\"", m.input.Value())
	}
	// Free text, so the box grows like any other plain command.
	if !m.promptZoom {
		t.Error("a seeded plain command should grow the box")
	}
}

func TestKStillTypesInTheResourceFilter(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusList

	m.handleKey(key("k"))
	if m.focus != focusList {
		t.Error("k in the resource filter should stay there")
	}
	if m.search != "k" {
		t.Errorf("list filter = %q, want k", m.search)
	}
}

// ---- the wheel stays inside an open popup --------------------------------

func TestWheelInPopupDoesNotScrollBehindIt(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openThemePicker()

	rowBefore, resBefore := m.rowIdx, m.resIdx
	themeBefore := m.themeRow

	// Scroll well inside the screen, where the table would be.
	l := m.layout()
	m.handleMouse(wheel(l.leftW+5, l.midY+4, false))

	if m.rowIdx != rowBefore || m.resIdx != resBefore {
		t.Error("scrolling a popup must not move anything behind it")
	}
	if m.themeRow == themeBefore {
		t.Error("scrolling a popup should move the popup's own selection")
	}
	// Moving in the theme picker previews as it goes.
	if m.themeIdx != m.themeRow {
		t.Error("the wheel should preview the theme it lands on")
	}
}

func TestWheelInPaletteMovesItsSelection(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openPalette()
	m.input.SetValue("web")
	if len(m.paletteHits()) < 2 {
		t.Skip("need at least two hits")
	}

	l := m.layout()
	m.handleMouse(wheel(l.leftW+5, l.midY+4, false))
	if m.palIdx == 0 {
		t.Error("the wheel should move the palette selection")
	}
}
