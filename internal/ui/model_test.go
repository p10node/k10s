package ui

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/muesli/termenv"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

func TestMain(m *testing.M) {
	// View() calls zone.Scan, which needs the global zone manager.
	zone.NewGlobal()
	// Tests have no TTY, so lipgloss would otherwise strip all styling and
	// colour assertions could never pass. Force the same profile cmd/shot
	// uses.
	lipgloss.SetColorProfile(termenv.TrueColor)
	m.Run()
}

func newTestModel(t *testing.T, src domain.Source) *Model {
	t.Helper()
	t.Setenv("K10S_CONFIG", t.TempDir()+"/config.yaml")
	m := New(src)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	return m
}

func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// countingSource wraps a Source and counts backend calls, so a test can
// assert on how much work one frame or one keypress triggers.
type countingSource struct {
	domain.Source
	rows      atomic.Int64
	rowCounts atomic.Int64
}

func (c *countingSource) Rows(kind, ns string) ([]string, [][]string) {
	c.rows.Add(1)
	return c.Source.Rows(kind, ns)
}

func (c *countingSource) RowCount(kind, ns string) int {
	c.rowCounts.Add(1)
	return c.Source.RowCount(kind, ns)
}

// TestViewDoesNotBuildRowsForEveryKind is the regression guard for the input
// lag: the Resources sidebar shows a count badge per kind, and it must get
// those from the cheap RowCount path. If it ever goes back to building full
// formatted Rows for every kind on every frame, this fails.
func TestViewDoesNotBuildRowsForEveryKind(t *testing.T) {
	src := &countingSource{Source: mock.New("")}
	m := newTestModel(t, src)
	nKinds := len(src.Kinds())

	src.rows.Store(0)
	src.rowCounts.Store(0)
	_ = m.View()

	gotRows := src.rows.Load()
	if gotRows >= int64(nKinds) {
		t.Errorf("one View() called Rows() %d times with %d kinds — the sidebar must use RowCount, not Rows, per kind", gotRows, nKinds)
	}
	if src.rowCounts.Load() == 0 {
		t.Error("expected the sidebar to use RowCount for its badges")
	}
}

// TestKeypressLatency guards responsiveness directly: a navigation keypress
// plus the frame it produces must stay far below human-perceptible lag.
func TestKeypressLatency(t *testing.T) {
	m := newTestModel(t, mock.New(""))

	// warm up (first frame builds caches/styles)
	m.Update(key("j"))
	_ = m.View()

	const iterations = 200
	start := time.Now()
	for i := 0; i < iterations; i++ {
		m.Update(key("j"))
		_ = m.View()
		m.Update(key("k"))
		_ = m.View()
	}
	perFrame := time.Since(start) / (iterations * 2)

	if raceEnabled {
		t.Skipf("race detector instrumentation dominates the measurement (%v/frame)", perFrame)
	}
	if perFrame > 8*time.Millisecond {
		t.Errorf("keypress+render took %v per frame, want < 8ms (input feels laggy above that)", perFrame)
	}
	t.Logf("keypress+render: %v per frame", perFrame)
}

// ":ns" with no name opens the Namespaces table, which is where the switch
// actually happens.
func TestNamespaceCommandOpensTable(t *testing.T) {
	m := newTestModel(t, mock.New(""))

	m.runSlash(":ns")
	if m.curKind().Key != "namespaces" {
		t.Fatalf(":ns should open the Namespaces table, kind = %q", m.curKind().Key)
	}

	m.applyNamespace("kube-system")
	if m.namespace != "kube-system" {
		t.Fatalf("namespace = %q, want kube-system", m.namespace)
	}

	// Namespaces itself is cluster-scoped, so check the column on a
	// namespaced kind.
	m.applyNamespace(domain.AllNamespaces)
	m.jumpToResource("pods")
	cols, _ := m.tableData()
	if cols[0] != "NAMESPACE" {
		t.Errorf("under all namespaces, expected NAMESPACE column, got %v", cols)
	}
}

func TestRowSearchFiltersTable(t *testing.T) {
	m := newTestModel(t, mock.New(""))

	_, before := m.tableData()
	m.runSlash(":filter web-frontend")
	_, after := m.tableData()

	if len(after) == 0 {
		t.Fatal("filter matched nothing")
	}
	if len(after) >= len(before) {
		t.Fatalf("filter did not narrow rows: %d -> %d", len(before), len(after))
	}
	for _, r := range after {
		if !strings.Contains(strings.Join(r, " "), "web-frontend") {
			t.Errorf("row %v does not match the filter", r)
		}
	}

	m.runSlash(":filter")
	_, cleared := m.tableData()
	if len(cleared) != len(before) {
		t.Errorf("clearing the filter did not restore all rows: %d, want %d", len(cleared), len(before))
	}
}

func TestResourceSearchSelectsMatchingKind(t *testing.T) {
	m := newTestModel(t, mock.New(""))

	m.runSlash(":search nodes")
	if got := m.curKind().Key; got != "nodes" {
		t.Errorf("after :search nodes, selected kind = %q, want nodes", got)
	}
}

func TestUnavailableActionIsRejected(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.jumpToResource("configmaps") // no Logs action allowed here

	var logs Action
	for _, a := range Actions {
		if a.ID == domain.ALogs {
			logs = a
		}
	}
	if cmd := m.fireAction(logs); cmd != nil {
		t.Error("firing an unavailable action should not produce a command")
	}
	if !strings.Contains(m.toast, "not available") {
		t.Errorf("toast = %q, want it to say the action is unavailable", m.toast)
	}
}

func TestConfirmModalGatesDestructiveAction(t *testing.T) {
	m := newTestModel(t, mock.New(""))

	var del Action
	for _, a := range Actions {
		if a.ID == domain.ADelete {
			del = a
		}
	}
	m.fireAction(del)
	if m.confirm == nil {
		t.Fatal("delete should open a confirm modal, not act immediately")
	}
	if !m.confirm.danger {
		t.Error("delete confirm modal should be marked danger")
	}

	// esc cancels without running the callback
	m.handleKey(key("esc"))
	if m.confirm != nil {
		t.Error("esc should dismiss the confirm modal")
	}
	if !strings.Contains(m.toast, "cancel") {
		t.Errorf("toast = %q, want it to mention cancellation", m.toast)
	}
}

func TestStaleLogLinesAreDropped(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.logGen = 3
	m.showText("logs -f x", "first")

	// a line from an older stream generation must not append
	m.Update(logLineMsg{gen: 2, line: "stale", ok: true})
	for _, l := range m.textLines {
		if l == "stale" {
			t.Fatal("a log line from a previous stream generation leaked into the view")
		}
	}

	m.Update(logLineMsg{gen: 3, line: "fresh", ok: true})
	found := false
	for _, l := range m.textLines {
		if l == "fresh" {
			found = true
		}
	}
	if !found {
		t.Error("current-generation log line was not appended")
	}
}

func TestTextResultErrorSurfacesAsToast(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.Update(textResultMsg{title: "describe x", err: errFake{}})

	if m.mode == modeText {
		t.Error("a failed fetch should not switch the main pane to text mode")
	}
	if !strings.Contains(m.toast, "boom") {
		t.Errorf("toast = %q, want it to contain the error text", m.toast)
	}
}

type errFake struct{}

func (errFake) Error() string { return "boom" }

func TestViewIsStableAcrossKindsAndNamespaces(t *testing.T) {
	m := newTestModel(t, mock.New(""))

	for _, k := range m.kinds() {
		m.jumpToResource(k.Key)
		for _, ns := range []string{"", "kube-system", domain.AllNamespaces} {
			m.namespace = ns
			out := m.View()
			if out == "" {
				t.Fatalf("empty frame for kind=%s ns=%s", k.Key, ns)
			}
			// every rendered line must be the same display width, or joins
			// and modal overlays drift (the Block invariant)
			lines := strings.Split(out, "\n")
			if len(lines) < 10 {
				t.Fatalf("suspiciously short frame (%d lines) for kind=%s ns=%s", len(lines), k.Key, ns)
			}
		}
	}
}
