package ui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/domain"
)

// S: quit k10s and open the pod shell in the terminal k10s was started from,
// with that terminal's scrollback, selection, fonts and colours.
//
// This is not tea.ExecProcess, which hands the terminal over and comes back
// into k10s afterwards. The point of S is to leave, so the UI only resolves
// the command, records it and quits; main.go runs it once the alt screen is
// gone, the same way /update's relaunch works.

// externalShellMsg carries the command Source.Shell resolved, so that call
// (an API round trip) stays off the UI goroutine.
type externalShellMsg struct {
	session, view int
	target        string
	cmd           tea.ExecCommand
	err           error
}

// startExternalShell resolves the shell for S. short is the kind's short name
// ("po"), for the line main.go prints before handing over.
func (m *Model) startExternalShell(src domain.Source, short, kind, ns, name string) tea.Cmd {
	m.backToTable()
	session, view, release := m.connGen, m.viewGen, m.leaseSource()
	m.startBusy("shell " + short + "/" + name)

	shown := ns
	if shown == "" {
		shown = "default"
	}
	target := short + "/" + name + " (namespace " + shown + ")"

	return func() tea.Msg {
		defer release()
		cmd, err := src.Shell(kind, ns, name)
		return externalShellMsg{session: session, view: view, target: target, cmd: cmd, err: err}
	}
}

// handleExternalShell quits k10s once the shell is ready, or stays and says
// why there isn't one.
//
// Nothing on the tea.Quit path closes the source, and that matters: the
// command builds its exec request from the store only when main.go runs it.
func (m *Model) handleExternalShell(msg externalShellMsg) tea.Cmd {
	if msg.session != m.connGen || msg.view != m.viewGen {
		return nil // the user moved on; don't quit under them
	}
	m.busy = false
	if msg.err != nil && !errors.Is(msg.err, domain.ErrNoShell) {
		m.toast = "✗ " + msg.err.Error()
		return nil
	}
	if msg.cmd == nil {
		m.toast = "no interactive shell here (offline demo, or not a pod)"
		return nil
	}
	m.exitShell = msg.cmd
	m.exitShellTarget = msg.target
	return tea.Quit
}

// ExitShell is the shell S asked for, and the resource it belongs to. main.go
// runs it after the program exits; nil when k10s quit any other way.
func (m *Model) ExitShell() (tea.ExecCommand, string) {
	return m.exitShell, m.exitShellTarget
}
