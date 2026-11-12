package ui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/domain"
)

// /login: signing in again with the context's credential plugin without
// leaving k10s.
//
// The plugin cannot prompt while the TUI is up (internal/k8s/credplugin.go),
// so when its credentials lapse the choices used to be a second shell or a
// restart. /login hands the terminal over instead — the same way the
// editor and foreground plugins get it — runs the plugin, and takes the
// terminal back when it exits. The plugin does its own relogin, MFA tap or
// device code on the real terminal and caches the result where its next
// non-interactive run will find it. Then k10s reconnects, if it needed to.

// loginExitMsg is the plugin having run with the terminal and exited.
type loginExitMsg struct {
	session int
	context string
	plugin  string
	err     error
}

// loginFor is how to sign in with a context, nil when the context has no
// credential plugin or nothing was wired to ask (tests, the demo).
func (m *Model) loginFor(context string) *domain.Login {
	if m.login == nil {
		return nil
	}
	return m.login(context)
}

// loginContext is the context /login applies to: the one on screen, or the
// one k10s is trying to reach when there is nothing on screen yet.
func (m *Model) loginContext() string {
	if m.offline || m.connecting {
		return m.connName
	}
	return m.src.ClusterInfo().Context
}

// loginCmd is /login.
func (m *Model) loginCmd() tea.Cmd {
	ctx := m.loginContext()
	lg := m.loginFor(ctx)
	if lg == nil {
		name := ctx
		if name == "" {
			name = "this context"
		}
		m.toast = "✗ " + name + " does not sign in through a credential plugin — nothing to log in with"
		return nil
	}
	m.toast = "⟩ signing in with " + lg.Plugin
	session := m.connGen
	return tea.ExecProcess(m.toTerminal(lg.Cmd), func(err error) tea.Msg {
		return loginExitMsg{session: session, context: ctx, plugin: lg.Plugin, err: err}
	})
}

// handleLoginExit is the plugin's verdict. A failure is left to the user:
// the plugin said why on the terminal, and k10s has nothing to add. A
// success reconnects when the lapse was what kept k10s off the cluster, or
// off part of it; otherwise it just says so.
func (m *Model) handleLoginExit(msg loginExitMsg) tea.Cmd {
	// The terminal came back raw and on the alternate screen, but without
	// mouse tracking; see editExitMsg.
	resumeMouse := m.resumeMouseAfterExec()
	if msg.err != nil {
		m.toast = "✗ " + msg.plugin + ": " + msg.err.Error() + " — log in with it in a shell, then r"
		return resumeMouse
	}
	if msg.session != m.connGen {
		// Switched away while the plugin ran; its credentials are cached
		// for whenever that context comes back.
		m.toast = "✓ signed in with " + msg.plugin
		return resumeMouse
	}
	if m.offline || credentialPlugin(m.kindLoadError()) != "" {
		// connectCmd sets its own toast; this one says why it is running.
		reconnect := m.connectCmd(msg.context)
		m.toast = m.withThemeWarning("✓ signed in with " + msg.plugin + " · reconnecting")
		return tea.Batch(resumeMouse, reconnect)
	}
	m.toast = "✓ signed in with " + msg.plugin
	return resumeMouse
}

// credentialPlugin names the plugin behind err when err is a credential
// plugin's failure, "" otherwise. The backend reports that with a type the
// UI does not import, so this goes by the method instead, the way
// sourceLoadError goes by LoadErrorFor.
func credentialPlugin(err error) string {
	var failed interface{ CredentialPlugin() string }
	if errors.As(err, &failed) {
		return failed.CredentialPlugin()
	}
	return ""
}
