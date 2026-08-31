package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/p10node/k10s/internal/mock"
	"github.com/p10node/k10s/internal/plugin"
)

func ctrlL() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlL} }

func TestPluginsAreScopedToResourceAliases(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.plugins = []plugin.Named{
		{Name: "logs", Plugin: plugin.Plugin{ShortCut: "Ctrl-L", Description: "Pod logs", Scopes: []string{"po"}, Command: "kubectl"}},
		{Name: "services", Plugin: plugin.Plugin{ShortCut: "Shift-X", Description: "Service only", Scopes: []string{"svc"}, Command: "kubectl"}},
	}

	got := m.availablePlugins()
	if len(got) != 1 || got[0].Name != "logs" {
		t.Fatalf("pod plugins = %+v, want only logs", got)
	}
}

func TestPluginShortcutOpensConfirmationWithSelectedResource(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.plugins = []plugin.Named{{Name: "logs", Plugin: plugin.Plugin{
		ShortCut: "Ctrl-L", Description: "Pod logs", Scopes: []string{"po"},
		Command: "kubectl", Confirm: true,
	}}}

	m.handleKey(ctrlL())
	if m.confirm == nil {
		t.Fatal("confirmed plugin should open a confirmation modal")
	}
	message := strings.Join(m.confirm.message, " ")
	if !strings.Contains(message, m.curName()) || !strings.Contains(message, "Pod logs") {
		t.Errorf("confirmation message = %q, want description and selected name", message)
	}
}

func TestPluginOverrideWinsOverBuiltInShortcut(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.plugins = []plugin.Named{{Name: "custom-describe", Plugin: plugin.Plugin{
		ShortCut: "d", Description: "Custom describe", Scopes: []string{"all"},
		Command: "printf", Args: []string{"custom"}, Confirm: true, Override: true,
	}}}

	m.handleKey(key("d"))
	if m.confirm == nil || m.confirm.title != "Plugin · Custom describe" {
		t.Fatalf("override plugin did not win: confirm=%+v", m.confirm)
	}
}

func TestPluginOverrideCanReplaceFindShortcut(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.plugins = []plugin.Named{{Name: "custom-find", Plugin: plugin.Plugin{
		ShortCut: "f", Description: "Custom find", Scopes: []string{"all"},
		Command: "printf", Confirm: true, Override: true,
	}}}

	m.handleKey(key("f"))
	if m.confirm == nil || m.focus == focusMainSearch {
		t.Fatalf("override plugin did not replace find: confirm=%+v focus=%v", m.confirm, m.focus)
	}
}

func TestPluginShortcutsAreConsistentWhileSearchHasFocus(t *testing.T) {
	for _, tc := range []struct {
		name  string
		focus focusPane
		msg   tea.KeyMsg
		item  plugin.Plugin
	}{
		{"override-list-global", focusList, tea.KeyMsg{Type: tea.KeyCtrlP}, plugin.Plugin{ShortCut: "Ctrl-P", Override: true}},
		{"ordinary-list-modifier", focusList, ctrlL(), plugin.Plugin{ShortCut: "Ctrl-L"}},
		{"ordinary-table-search-modifier", focusMainSearch, ctrlL(), plugin.Plugin{ShortCut: "Ctrl-L"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t, mock.New(""))
			dismissOnboarding(m)
			m.focus = tc.focus
			tc.item.Scopes = []string{"all"}
			tc.item.Command = "printf"
			tc.item.Confirm = true
			m.plugins = []plugin.Named{{Name: tc.name, Plugin: tc.item}}

			m.handleKey(tc.msg)
			if m.confirm == nil {
				t.Fatalf("plugin did not run with focus %v", tc.focus)
			}
		})
	}
}

func TestPluginWithoutOverrideYieldsToBuiltInShortcut(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.plugins = []plugin.Named{{Name: "custom-describe", Plugin: plugin.Plugin{
		ShortCut: "d", Description: "Custom describe", Scopes: []string{"all"},
		Command: "printf", Args: []string{"custom"}, Confirm: true,
	}}}

	cmd := m.handleKey(key("d"))
	if cmd == nil || m.confirm != nil || !m.busy {
		t.Fatalf("built-in describe should win without override: cmd=%v confirm=%+v busy=%v", cmd, m.confirm, m.busy)
	}
}

func TestPluginVariablesIncludeSelectedRowAndColumns(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	vars := m.pluginVars()
	if vars.Name != m.curName() || vars.Namespace != m.curNamespace() {
		t.Errorf("vars identity = %+v, selected %s/%s", vars, m.curNamespace(), m.curName())
	}
	if vars.Context != m.src.ClusterInfo().Context || vars.ResourceName != m.curKind().Key {
		t.Errorf("vars cluster/resource = %+v", vars)
	}
	if vars.Cluster == vars.Context || vars.User == "" || vars.Kubeconfig == "" {
		t.Errorf("vars should use kubeconfig cluster identity, not OS/context approximations: %+v", vars)
	}
	if vars.Columns["NAME"] != m.curName() {
		t.Errorf("NAME column = %q, want %q", vars.Columns["NAME"], m.curName())
	}
}

func TestPluginConfirmationExecutesTheDisplayedVariableSnapshot(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	out := filepath.Join(t.TempDir(), "selected")
	t.Setenv("OUT", out)
	item := plugin.Named{Name: "snapshot", Plugin: plugin.Plugin{
		Scopes: []string{"all"}, Command: "sh",
		Args:    []string{"-c", `printf '%s' "$NAME" > "$OUT"`},
		Confirm: true, Background: true,
	}}
	want := m.curName()

	m.firePlugin(item)
	if m.confirm == nil {
		t.Fatal("confirmation did not open")
	}
	m.rowIdx++
	if got := m.curName(); got == want {
		t.Fatal("test requires selection to change before confirmation")
	}
	cmd := m.confirm.onOK(m)
	if msg := cmd(); msg == nil {
		t.Fatal("background plugin returned no result message")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := os.ReadFile(out)
		if err == nil {
			if string(got) != want {
				t.Fatalf("executed name = %q, want confirmed snapshot %q", got, want)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("plugin did not write output: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestActionPaneListsApplicablePlugins(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.plugins = []plugin.Named{{Name: "logs", Plugin: plugin.Plugin{
		ShortCut: "Ctrl-L", Description: "Pod logs plugin", Scopes: []string{"po"}, Command: "kubectl",
	}}}

	view := stripANSI(m.viewActions(30, 18).String())
	if !strings.Contains(view, "ctrl+l") || !strings.Contains(view, "Pod logs plugin") {
		t.Errorf("actions pane does not show plugin shortcut and description:\n%s", view)
	}
}

func TestActionPaneHandlesLongPluginShortcut(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.plugins = []plugin.Named{{Name: "long", Plugin: plugin.Plugin{
		ShortCut: "Ctrl-This-Is-Much-Longer-Than-The-Pane", Description: "Long shortcut",
		Scopes: []string{"all"}, Command: "printf",
	}}}

	view := m.viewActions(20, 18).String()
	for _, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width != 20 {
			t.Fatalf("action line width = %d, want 20", width)
		}
	}
}
