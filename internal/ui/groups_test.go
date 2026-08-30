package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/config"
	"github.com/p10node/k10s/internal/mock"
)

func groupOf(m *Model, key string) string {
	for _, k := range m.kinds() {
		if k.Key == key {
			return k.Group
		}
	}
	return ""
}

// The kinds people open k10s for stay in view; the reference material starts
// folded, because thirty kinds do not fit a laptop-sized sidebar.
func TestSeldomUsedGroupsStartFolded(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	for _, g := range defaultCollapsedGroups {
		if !m.collapsed[g] {
			t.Errorf("%s should start folded", g)
		}
	}
	for _, g := range []string{"Workloads", "Network", "Cluster"} {
		if m.collapsed[g] {
			t.Errorf("%s should start open — it is what the tool is for", g)
		}
	}

	// Folded kinds are not in the sidebar's walk, and their names are not on
	// screen either.
	view := stripANSI(m.View())
	for _, k := range m.kinds() {
		shown := strings.Contains(view, k.Name)
		if m.collapsed[k.Group] && shown && k.Key != "pvcs" {
			t.Errorf("%s is in a folded group but still rendered", k.Name)
		}
	}
	for _, i := range m.visible() {
		if m.collapsed[m.kinds()[i].Group] {
			t.Errorf("%s is folded but still walkable", m.kinds()[i].Key)
		}
	}
}

func TestFoldedGroupHeaderShowsHowManyItHides(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	view := stripANSI(m.View())
	if !strings.Contains(view, "▸ RBAC") {
		t.Error("a folded group should show the closed chevron")
	}
	if !strings.Contains(view, "▾ WORKLOADS") {
		t.Error("an open group should show the open chevron")
	}
	n := len(m.groupKinds("RBAC"))
	if n == 0 {
		t.Fatal("RBAC has no kinds")
	}
	line := ""
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "RBAC") {
			line = l
		}
	}
	if !strings.Contains(line, "5") || n != 5 {
		t.Errorf("folded RBAC header %q should carry its kind count (%d)", line, n)
	}
}

func TestToggleGroupPersists(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.toggleGroup("RBAC")    // open it
	m.toggleGroup("Network") // fold one that starts open
	if m.collapsed["RBAC"] || !m.collapsed["Network"] {
		t.Fatalf("toggle did not flip both ways: %v", m.collapsed)
	}

	c, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !c.CollapsedSet {
		t.Error("the saved config does not record the choice")
	}
	got := strings.Join(c.Collapsed, ",")
	if strings.Contains(got, "RBAC") || !strings.Contains(got, "Network") {
		t.Errorf("saved collapsed = %q, want Network folded and RBAC open", got)
	}
}

// Opening every group must survive a restart — the defaults may not fold
// them again behind the user's back.
func TestOpeningEveryGroupSurvivesRestart(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	for _, g := range m.groupOrder() {
		if m.collapsed[g] {
			m.toggleGroup(g)
		}
	}

	restarted := New(mock.New(""))
	for _, g := range restarted.groupOrder() {
		if restarted.collapsed[g] {
			t.Errorf("%s was folded again after a restart", g)
		}
	}
}

// A search has to win over folding: a match hidden inside a folded group
// would make the filter look broken.
func TestSearchShowsMatchesInFoldedGroups(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusList

	for _, r := range "secret" {
		m.handleKey(key(string(r)))
	}
	if !strings.Contains(stripANSI(m.View()), "Secrets") {
		t.Error("searching did not surface Secrets from the folded Config group")
	}
	found := false
	for _, i := range m.visible() {
		if m.kinds()[i].Key == "secrets" {
			found = true
		}
	}
	if !found {
		t.Error("a search match must be walkable even inside a folded group")
	}
}

// Bubbletea delivers a lone space as KeySpace, which is what the app sees.
func TestSpaceTogglesTheGroupUnderTheCursor(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusList
	g := groupOf(m, "pods")

	m.handleKey(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if !m.collapsed[g] {
		t.Fatalf("space did not fold %s", g)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if m.collapsed[g] {
		t.Errorf("space did not open %s again", g)
	}

	// With a search running, space is a search character — "Custom
	// Resources" cannot be typed without one.
	m.search = "custom"
	m.handleKey(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if m.collapsed[g] {
		t.Error("space folded a group while searching")
	}
	if m.search != "custom " {
		t.Errorf("search = %q, want the space typed into it", m.search)
	}
}

// Arrow keys walk what is on screen, and never open a group you folded.
func TestArrowsSkipFoldedGroups(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.focus = focusList

	// From the last open kind before Config, ↓ must land past the folded
	// groups rather than inside one.
	for i, k := range m.kinds() {
		if k.Key == "networkpolicies" { // last of Network, Config comes next
			m.selectResource(i)
		}
	}
	m.moveList(1)
	if g := m.curKind().Group; m.collapsed[g] {
		t.Errorf("↓ landed on %s inside the folded %s", m.curKind().Key, g)
	}
	if got := m.curKind().Key; got != "nodes" {
		t.Errorf("↓ landed on %q, want the first kind of the next open group", got)
	}
}

// Jumping by command or palette opens the group it lands in, otherwise the
// table changes while the sidebar shows nothing selected.
func TestJumpingRevealsAFoldedGroup(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	if !m.collapsed["RBAC"] {
		t.Fatal("RBAC should start folded for this test to mean anything")
	}

	m.runSlash(":sa")
	if m.collapsed["RBAC"] {
		t.Error(":sa should have opened the RBAC group")
	}
	if !strings.Contains(stripANSI(m.View()), "ServiceAccounts") {
		t.Error("the kind jumped to is not visible in the sidebar")
	}
}

// Folding the group you are standing in is allowed — the header says so
// rather than the selection vanishing without a trace.
func TestFoldedGroupHoldingTheCursorIsMarked(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.toggleGroup("Workloads")

	view := stripANSI(m.View())
	if !strings.Contains(view, "▸ WORKLOADS") {
		t.Fatal("Workloads did not fold")
	}
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "WORKLOADS") && !strings.Contains(l, "▸") {
			t.Errorf("header %q lost its marker", l)
		}
	}
}

// Object and kind names contain spaces ("Custom Resources", "my pod"), and
// bubbletea reports a lone space as KeySpace rather than KeyRunes — so both
// search boxes have to admit it explicitly.
func TestSpaceIsTypeableIntoBothSearchBoxes(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.focus = focusList
	for _, r := range "custom" {
		m.handleKey(key(string(r)))
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	m.handleKey(key("r"))
	if m.search != "custom r" {
		t.Errorf("resource search = %q, want %q", m.search, "custom r")
	}
	// Both Custom Resources kinds live in a group of that name, so the
	// match set is the group — what matters is that the space narrowed it.
	if got := len(m.filtered()); got != 2 {
		t.Errorf("%q matched %d kinds, want the two Custom Resources ones", m.search, got)
	}

	m.focus = focusMainSearch
	m.handleKey(key("a"))
	m.handleKey(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if m.rowSearch != "a " {
		t.Errorf("row search = %q, want %q", m.rowSearch, "a ")
	}
}

// ---- scrolling the Resources pane ----------------------------------------

// The wheel is for looking around. A kind list that reselected as it slid
// past would swap out the whole main panel by accident — which is why the
// pane used to refuse the wheel altogether.
func TestScrollingTheResourceListKeepsTheSelection(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	for _, g := range m.groupOrder() { // open everything so it overflows
		if m.collapsed[g] {
			m.toggleGroup(g)
		}
	}
	m.h = 24 // fewer rows than kinds
	if len(m.listEntries()) <= m.listRows() {
		t.Fatal("the list fits — this test needs it to overflow")
	}

	before := m.curKind().Key
	mainBefore, _ := m.tableData()

	m.scrollListPane(6)
	if m.listScroll == 0 {
		t.Error("the pane did not scroll")
	}
	if got := m.curKind().Key; got != before {
		t.Errorf("scrolling changed the selection: %q → %q", before, got)
	}
	mainAfter, _ := m.tableData()
	if strings.Join(mainAfter, "|") != strings.Join(mainBefore, "|") {
		t.Error("scrolling the sidebar changed the main panel")
	}
}

func TestListScrollStopsAtBothEnds(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.h = 24

	m.scrollListPane(-5)
	if m.listScroll != 0 {
		t.Errorf("scrolled to %d above the top", m.listScroll)
	}

	m.scrollListPane(500)
	max := len(m.listEntries()) - m.listRows()
	if max < 0 {
		max = 0
	}
	if m.listScroll != max {
		t.Errorf("scrolled to %d, want the last full screen at %d", m.listScroll, max)
	}
}

// The wheel over the sidebar scrolls the sidebar — not the table behind it,
// and not the selection.
func TestWheelOverTheListScrollsIt(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	for _, g := range m.groupOrder() {
		if m.collapsed[g] {
			m.toggleGroup(g)
		}
	}
	m.h = 24
	before, rowBefore := m.curKind().Key, m.rowIdx

	l := m.layout()
	m.handleMouse(tea.MouseMsg{
		X: 1, Y: l.midY + 2,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	if m.listScroll == 0 {
		t.Error("the wheel did not scroll the Resources pane")
	}
	if got := m.curKind().Key; got != before {
		t.Errorf("the wheel changed the selection: %q → %q", before, got)
	}
	if m.rowIdx != rowBefore {
		t.Error("the wheel over the sidebar moved the table's row cursor")
	}
}

// Moving the selection out of the visible window drags the window along —
// the one direction that is allowed.
func TestSelectionScrollsItselfIntoView(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	for _, g := range m.groupOrder() {
		if m.collapsed[g] {
			m.toggleGroup(g)
		}
	}
	m.h = 24

	last := len(m.kinds()) - 1
	m.selectResource(last)
	if !m.selectionVisible() {
		t.Error("selecting the last kind did not scroll it into view")
	}

	m.selectResource(0)
	if !m.selectionVisible() {
		t.Error("selecting the first kind did not scroll back up to it")
	}
	// Scrolled all the way back, group header included — a kind at the top
	// edge with no header over it looks like it belongs to another group.
	if m.listScroll != 0 {
		t.Errorf("listScroll = %d after selecting the first kind, want the top", m.listScroll)
	}
}

// selectionVisible reports whether the selected kind's line is inside the
// pane's current window.
func (m *Model) selectionVisible() bool {
	entries := m.listEntries()
	top := m.listTop(len(entries))
	for i, e := range entries {
		if e.kind == m.resIdx {
			return i >= top && i < top+m.listRows()
		}
	}
	return false
}

// A pane that scrolls has to say so, or the wheel is a feature nobody finds.
func TestOverflowingListShowsScrollHints(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	for _, g := range m.groupOrder() {
		if m.collapsed[g] {
			m.toggleGroup(g)
		}
	}
	m.h = 24

	view := stripANSI(m.viewList(22, m.layout().midH).String())
	if !strings.Contains(view, "↓") {
		t.Error("no hint that the list continues below")
	}
	if strings.Contains(view, "↑") {
		t.Error("claims there is more above while scrolled to the top")
	}

	m.scrollListPane(500)
	view = stripANSI(m.viewList(22, m.layout().midH).String())
	if !strings.Contains(view, "↑") {
		t.Error("no hint that the list continues above once scrolled down")
	}
	if strings.Contains(view, "↓") {
		t.Error("claims there is more below at the very bottom")
	}

	// A list that fits needs no hints at all.
	m.h = 60
	m.listScroll = 0
	view = stripANSI(m.viewList(22, m.layout().midH).String())
	if strings.Contains(view, "↑") || strings.Contains(view, "↓") {
		t.Error("a list that fits should show no scroll hints")
	}
}
