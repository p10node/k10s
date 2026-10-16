package ui

import (
	"strings"
	"testing"

	"github.com/p10node/k10s/internal/mock"
	"github.com/p10node/k10s/internal/plugin"
)

func newReadOnlyModel(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.readOnly = true
	return m
}

func TestReadOnlyActionsPaneListsOnlyReads(t *testing.T) {
	m := newReadOnlyModel(t)
	pane := strings.Join(m.viewActions(40, 30).Lines, "\n")
	for _, want := range []string{"Describe", "YAML", "Logs"} {
		if !strings.Contains(pane, want) {
			t.Errorf("Actions pane lacks %q:\n%s", want, pane)
		}
	}
	for _, gone := range []string{"Shell", "Port Forward", "Edit", "Delete"} {
		if strings.Contains(pane, gone) {
			t.Errorf("Actions pane still offers %q in read-only mode:\n%s", gone, pane)
		}
	}
}

func TestReadOnlyRefusesWriteKeysBeforeAnyDialog(t *testing.T) {
	for _, k := range []string{"D", "e", "s", "p"} {
		m := newReadOnlyModel(t)
		m.handleKey(key(k))
		if m.confirm != nil {
			t.Errorf("key %q opened a confirmation in read-only mode", k)
		}
		if !strings.Contains(m.toast, "read-only") {
			t.Errorf("key %q: toast = %q, want a read-only notice", k, m.toast)
		}
	}
}

func TestReadOnlyStillReads(t *testing.T) {
	m := newReadOnlyModel(t)
	m.handleKey(key("d"))
	if strings.Contains(m.toast, "read-only") {
		t.Errorf("describe was refused: toast = %q", m.toast)
	}
}

func TestReadOnlyRefusesScaleCommand(t *testing.T) {
	m := newReadOnlyModel(t)
	if cmd := m.runSlash(":scale 3"); cmd != nil {
		t.Error(":scale returned a command in read-only mode")
	}
	if !strings.Contains(m.toast, "read-only") {
		t.Errorf("toast = %q, want a read-only notice", m.toast)
	}
}

func TestReadOnlyRefusesTypedShellCommands(t *testing.T) {
	m := newReadOnlyModel(t)
	if cmd := m.runShellCmd("kubectl delete pod web"); cmd != nil {
		t.Error("a typed shell command ran in read-only mode")
	}
	if !strings.Contains(m.toast, "read-only") {
		t.Errorf("toast = %q, want a read-only notice", m.toast)
	}
}

func TestReadOnlyDropsDangerousPlugins(t *testing.T) {
	m := newReadOnlyModel(t)
	m.plugins = []plugin.Named{
		{Name: "tail", Plugin: plugin.Plugin{ShortCut: "Ctrl-L", Scopes: []string{"all"}, Command: "kubectl"}},
		{Name: "nuke", Plugin: plugin.Plugin{ShortCut: "Shift-N", Scopes: []string{"all"}, Command: "kubectl", Dangerous: true}},
	}
	got := m.availablePlugins()
	if len(got) != 0 {
		t.Fatalf("plugins offered = %+v, want none", got)
	}
	for _, p := range m.plugins {
		if cmd := m.firePlugin(p); cmd != nil || m.confirm != nil {
			t.Fatal("external plugin admitted in read-only mode")
		}
		if cmd := m.runPlugin(p, plugin.Vars{}); cmd != nil {
			t.Fatal("direct external plugin admitted in read-only mode")
		}
	}
	if cmd := m.firePlugin(m.plugins[1]); cmd != nil || m.confirm != nil {
		t.Error("a dangerous plugin could still be fired directly in read-only mode")
	}
}

func TestReadOnlyShowsInHeader(t *testing.T) {
	m := newReadOnlyModel(t)
	if !strings.Contains(m.View(), "READ-ONLY") {
		t.Error("header does not say READ-ONLY")
	}
	m.readOnly = false
	if strings.Contains(m.View(), "READ-ONLY") {
		t.Error("header says READ-ONLY without --readonly")
	}
}
