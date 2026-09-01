package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/plugin"
)

// availablePlugins returns plugins whose k9s scope names the current kind.
func (m *Model) availablePlugins() []plugin.Named {
	// Context rows are not Kubernetes resources. Do not expose plugins for
	// the resource table hidden underneath the chooser.
	if m.mode == modeContexts {
		return nil
	}

	kind := m.curKind()
	aliases := aliasesFor(kind)
	aliases = append(aliases, kind.Key, kind.Short, strings.ToLower(kind.Name))
	out := make([]plugin.Named, 0, len(m.plugins))
	for _, item := range m.plugins {
		if item.MatchesScope(aliases...) {
			out = append(out, item)
		}
	}
	return out
}

func (m *Model) pluginForKey(key string, override bool) (plugin.Named, bool) {
	for _, item := range m.availablePlugins() {
		if item.Override == override && plugin.NormalizeShortcut(item.ShortCut) == key {
			return item, true
		}
	}
	return plugin.Named{}, false
}

// pluginVars snapshots the selected row before a command starts. Column names
// follow k9s's $COL-<COLUMN> convention exactly as displayed in the table.
func (m *Model) pluginVars() plugin.Vars {
	cols, _ := m.tableData()
	row := m.curRow()
	columnValues := make(map[string]string, len(cols))
	for i, column := range cols {
		if i < len(row) {
			columnValues[column] = row[i]
		}
	}
	cluster := m.src.ClusterInfo()
	return plugin.Vars{
		Name:         m.curName(),
		Namespace:    m.curNamespace(),
		Context:      cluster.Context,
		Cluster:      cluster.Cluster,
		ResourceName: m.curKind().Key,
		Filter:       m.rowSearch,
		Kubeconfig:   cluster.Kubeconfig,
		User:         cluster.User,
		Groups:       cluster.Groups,
		Columns:      columnValues,
	}
}

func pluginLabel(item plugin.Named) string {
	if item.Description != "" {
		return item.Description
	}
	return item.Name
}

func (m *Model) firePlugin(item plugin.Named) tea.Cmd {
	// Keep this guard even though availablePlugins hides plugins in context
	// mode: it makes direct callers safe too.
	if m.mode == modeContexts {
		return nil
	}

	label := pluginLabel(item)
	vars := m.pluginVars()
	if item.Confirm {
		name, namespace := m.curName(), m.curNamespace()
		m.confirm = &confirmState{
			title:  "Plugin · " + label,
			danger: item.Dangerous,
			message: []string{
				label,
				m.curKind().Short + "/" + name,
				"namespace: " + namespace,
				"",
				item.DisplayCommand(vars),
			},
			onOK: func(mm *Model) tea.Cmd { return mm.runPlugin(item, vars) },
		}
		return nil
	}
	return m.runPlugin(item, vars)
}

func (m *Model) runPlugin(item plugin.Named, vars plugin.Vars) tea.Cmd {
	label := pluginLabel(item)
	cmd := item.ExecCommand(vars)
	if item.Background {
		m.toast = "… plugin " + label
		return func() tea.Msg {
			if err := cmd.Start(); err != nil {
				return actionResultMsg{err: fmt.Errorf("plugin %s: %w", item.Name, err)}
			}
			if err := cmd.Process.Release(); err != nil {
				return actionResultMsg{err: fmt.Errorf("plugin %s: %w", item.Name, err)}
			}
			return actionResultMsg{toast: "✓ plugin " + label + " started in background"}
		}
	}

	m.toast = "⟩ plugin " + label
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return foregroundPluginResult(item, label, err)
	})
}

// foregroundPluginResult marks the terminal handoff so Model.Update restores
// Bubble Tea's mouse tracking after the child process exits.
func foregroundPluginResult(item plugin.Named, label string, err error) actionResultMsg {
	if err != nil {
		return actionResultMsg{
			err:         fmt.Errorf("plugin %s: %w", item.Name, err),
			resumeMouse: true,
		}
	}
	return actionResultMsg{
		toast:       "✓ plugin " + label + " finished",
		resumeMouse: true,
	}
}
