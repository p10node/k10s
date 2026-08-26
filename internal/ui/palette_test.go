package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"k10s/internal/domain"
	"k10s/internal/mock"
)

// ---- enter opens logs, falling back to describe -------------------------

func TestEnterOpensLogsWhenAvailable(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	if !m.curKind().Can(domain.ALogs) {
		t.Fatalf("precondition: %q should support logs", m.curKind().Key)
	}
	drainCmd(m, m.handleKey(key("enter")))

	if m.mode != modeLogs {
		t.Fatalf("enter should open the log viewer, mode = %v", m.mode)
	}
	if !strings.Contains(m.textTitle, "logs") {
		t.Errorf("title = %q, want the logs view", m.textTitle)
	}
}

func TestEnterFallsBackToDescribeWithoutLogs(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	// ConfigMaps have no logs action. (Namespaces would also qualify, but
	// enter is special-cased there to switch namespace instead.)
	for i, k := range m.kinds() {
		if k.Key == "configmaps" {
			m.selectResource(i)
		}
	}
	if m.curKind().Can(domain.ALogs) {
		t.Fatal("precondition: configmaps should not support logs")
	}

	drainCmd(m, m.handleKey(key("enter")))
	if m.mode != modeText {
		t.Fatal("enter should still open something")
	}
	if !strings.Contains(m.textTitle, "describe") {
		t.Errorf("title = %q, want it to fall back to describe", m.textTitle)
	}
}

// drainCmd runs a returned command and feeds the resulting message back into
// the model, so an async action (describe/logs fetch) resolves within a test
// the way it would once bubbletea delivered the message.
func drainCmd(m *Model, cmd tea.Cmd) {
	for i := 0; cmd != nil && i < 8; i++ {
		msg := cmd()
		if !IsAsyncMsg(msg) {
			return
		}
		_, cmd = m.Update(msg)
	}
}

// ---- global search palette ----------------------------------------------

func TestPaletteFindsKindsAndObjects(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.openPalette()
	if !m.palOpen {
		t.Fatal("ctrl+p should open the palette")
	}

	m.input.SetValue("web-frontend")
	hits := m.paletteHits()
	if len(hits) == 0 {
		t.Fatal("expected object hits for web-frontend")
	}
	found := false
	for _, h := range hits {
		if h.row >= 0 && strings.Contains(h.label, "web-frontend") {
			found = true
		}
	}
	if !found {
		t.Errorf("no object hit for web-frontend in %d hits", len(hits))
	}

	// Kind names match too.
	m.input.SetValue("Deploy")
	hits = m.paletteHits()
	kindHit := false
	for _, h := range hits {
		if h.row < 0 && h.kind.Key == "deployments" {
			kindHit = true
		}
	}
	if !kindHit {
		t.Error("searching a kind name should offer the kind itself")
	}
}

func TestPaletteEnterJumpsToObject(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.openPalette()
	m.input.SetValue("cache-redis-1")
	hits := m.paletteHits()
	if len(hits) == 0 {
		t.Fatal("expected a hit")
	}

	// Pick the first object hit.
	idx := -1
	for i, h := range hits {
		if h.row >= 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("no object hit")
	}
	m.palIdx = idx
	m.handlePaletteKey(key("enter"))

	if m.palOpen {
		t.Error("enter should close the palette")
	}
	if m.curName() != "cache-redis-1" {
		t.Errorf("selected row = %q, want cache-redis-1", m.curName())
	}
	if m.focus != focusMain {
		t.Errorf("focus = %v, want the main pane after jumping", m.focus)
	}
}

func TestPaletteEmptyQueryHasNoHits(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openPalette()
	if got := m.paletteHits(); len(got) != 0 {
		t.Errorf("empty query returned %d hits, want none", len(got))
	}
}

func TestPaletteEscCloses(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openPalette()
	m.handlePaletteKey(key("esc"))
	if m.palOpen {
		t.Error("esc should close the palette")
	}
}

// TestPaletteSkipsUnloadedKinds pins the perf contract: the palette must not
// pull rows from kinds whose data isn't loaded, since that would start a
// cluster-wide watch per kind.
func TestPaletteSkipsUnloadedKinds(t *testing.T) {
	m := newTestModel(t, unloadedSource{mock.New("")})
	dismissOnboarding(m)
	m.openPalette()
	m.input.SetValue("web-frontend")

	for _, h := range m.paletteHits() {
		if h.row >= 0 {
			t.Fatalf("palette scanned rows of an unloaded kind (%q)", h.kind.Key)
		}
	}
}

// unloadedSource reports every kind as not yet synced.
type unloadedSource struct{ domain.Source }

func (unloadedSource) Synced(string) bool { return false }

// ---- namespace chooser (the Namespaces table) ---------------------------

func TestNamespaceChooserLeadsWithAll(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.showNamespaceChooser()
	if m.curKind().Key != "namespaces" {
		t.Fatalf("kind = %q, want namespaces", m.curKind().Key)
	}
	_, rows := m.tableData()
	if len(rows) == 0 {
		t.Fatal("no namespace rows")
	}
	if rows[0][0] != domain.AllNamespaces {
		t.Errorf("first row = %q, want %q", rows[0][0], domain.AllNamespaces)
	}
}

func TestNamespaceChooserAllRowSwitchesToAll(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.showNamespaceChooser()
	m.rowIdx = 0 // the synthetic "all" row
	m.openSelected()

	if m.namespace != domain.AllNamespaces {
		t.Errorf("namespace = %q, want %q", m.namespace, domain.AllNamespaces)
	}
	if m.curKind().Key != "pods" {
		t.Errorf("kind = %q, want pods", m.curKind().Key)
	}
	cols, _ := m.tableData()
	if cols[0] != "NAMESPACE" {
		t.Errorf("under all namespaces expected a NAMESPACE column, got %v", cols)
	}
}

func TestNSCommandOpensTableNotPopup(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.runSlash("/ns")
	if m.curKind().Key != "namespaces" {
		t.Errorf("/ns should open the Namespaces table, kind = %q", m.curKind().Key)
	}
	if m.modalOpen() {
		t.Error("/ns should not open a popup any more")
	}
}

func TestNamespaceSwitchResetsRowSelection(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.rowIdx, m.rowScroll = 5, 3

	m.applyNamespace("kube-system")
	if m.rowIdx != 0 || m.rowScroll != 0 {
		t.Errorf("switching namespace left the cursor at row %d/scroll %d", m.rowIdx, m.rowScroll)
	}
}

// ---- actions pane shows only applicable actions -------------------------

func TestActionsPaneListsOnlyApplicableActions(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	for i, k := range m.kinds() {
		if k.Key == "configmaps" {
			m.selectResource(i)
		}
	}
	kind := m.curKind()
	view := m.viewActions(26, 26).String()

	for _, a := range Actions {
		listed := strings.Contains(view, a.Label)
		if want := kind.Can(a.ID); listed != want {
			t.Errorf("action %q listed=%v, want %v for %s", a.Label, listed, want, kind.Key)
		}
	}
}
