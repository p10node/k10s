package ui

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

// While a context signs in through a credential plugin, the connecting
// toast names it: that is what the wait is for, and the tool to log in with
// if it never ends.
func TestConnectingToastNamesTheCredentialPlugin(t *testing.T) {
	t.Setenv("K10S_CONFIG", t.TempDir()+"/config.yaml")
	plugins := map[string]string{"teleport-prod": "tsh", "kind-kind": ""}
	m := NewStartup(Startup{
		Kinds:            mock.New("").Kinds(),
		Contexts:         []string{"kind-kind", "teleport-prod"},
		Context:          "teleport-prod",
		Connect:          func(string) (domain.Source, string) { return mock.New(""), "" },
		CredentialPlugin: func(ctx string) string { return plugins[ctx] },
	})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	if !strings.Contains(m.toast, "credentials from tsh") {
		t.Errorf("startup toast = %q, want it to name tsh", m.toast)
	}

	m.connectCmd("kind-kind")
	if strings.Contains(m.toast, "credentials from") {
		t.Errorf("toast for a token context = %q, names a plugin it does not use", m.toast)
	}
	m.connectCmd("teleport-prod")
	if !strings.Contains(m.toast, "credentials from tsh") {
		t.Errorf("toast = %q, want it to name tsh", m.toast)
	}
}

// Processes that take the terminal over write errors to the terminal, not
// to the pipe os.Stderr has become; a stderr the caller chose is kept.
func TestProcessesThatTakeTheTerminalGetItsStderr(t *testing.T) {
	var terminal bytes.Buffer
	m := New(mock.New(""))
	m.termStderr = &terminal

	c := m.toTerminal(exec.Command("true"))
	if c.Stderr != &terminal {
		t.Errorf("Stderr = %v, want the terminal", c.Stderr)
	}

	var own bytes.Buffer
	c = exec.Command("true")
	c.Stderr = &own
	if m.toTerminal(c).Stderr != &own {
		t.Error("a stderr the caller set was replaced")
	}

	m.termStderr = nil
	if c := m.toTerminal(exec.Command("true")); c.Stderr != nil {
		t.Errorf("with no terminal known, Stderr = %v, want nil (Bubble Tea's default)", c.Stderr)
	}
}
