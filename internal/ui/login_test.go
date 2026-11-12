package ui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

// loginModel is newStartupModel with a Login hook: alpha signs in through
// tsh, beta through nothing.
func loginModel(t *testing.T, connect func(string) (domain.Source, string)) *Model {
	t.Helper()
	t.Setenv("K10S_CONFIG", t.TempDir()+"/config.yaml")
	m := NewStartup(Startup{
		Kinds:    mock.New("").Kinds(),
		Contexts: []string{"alpha", "beta"},
		Context:  "alpha",
		Connect:  connect,
		Login: func(ctx string) *domain.Login {
			if ctx != "alpha" {
				return nil
			}
			return &domain.Login{Plugin: "tsh", Cmd: exec.Command("true"), Alt: "tsh proxy kube prod --exec"}
		},
	})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	return m
}

// With nothing to log in with, /login says so instead of running nothing.
func TestLoginWithoutAPluginSaysSo(t *testing.T) {
	m := newStartupModel(t, noCluster("no configuration has been provided"))
	m.Update(m.connectCmd("")())

	if cmd := m.runSlash("/login"); cmd != nil {
		t.Fatal("/login produced a command with no plugin to run")
	}
	if !strings.Contains(m.toast, "nothing to log in with") {
		t.Errorf("toast = %q, want it to say there is nothing to log in with", m.toast)
	}
}

// The No cluster panel offers /login, and Teleport's other way in, when the
// context signs in through a plugin; /login then hands the terminal over.
func TestNoClusterPanelOffersLoginForAPluginContext(t *testing.T) {
	m := loginModel(t, noCluster("credential plugin tsh: Having problems with relogin, please use 'tsh login/tsh kube login' manually"))
	m.Update(m.connectCmd("")())
	if !m.offline {
		t.Fatal("precondition: should be offline")
	}

	v := zone.Scan(m.View())
	for _, want := range []string{"/login", "sign in again with tsh", "tsh proxy kube prod --exec"} {
		if !strings.Contains(v, want) {
			t.Errorf("the No cluster frame does not offer %q:\n%s", want, v)
		}
	}

	if cmd := m.runSlash("/login"); cmd == nil {
		t.Fatal("/login did nothing for a context with a plugin")
	}
	if !strings.Contains(m.toast, "signing in with tsh") {
		t.Errorf("toast = %q, want it to say who is being signed in with", m.toast)
	}
}

// A context without a plugin gets no /login row: offering a button that
// only produces an error is the thing the Actions pane exists not to do.
func TestNoClusterPanelOffersNoLoginWithoutAPlugin(t *testing.T) {
	m := loginModel(t, noCluster("dial tcp: connection refused"))
	m.Update(m.connectCmd("beta")())

	if v := zone.Scan(m.View()); strings.Contains(v, "/login") {
		t.Errorf("beta has no plugin, yet the frame offers /login:\n%s", v)
	}
}

// Signing in while offline is followed by the reconnect the user would
// otherwise press r for.
func TestLoginSuccessReconnectsWhenOffline(t *testing.T) {
	m := loginModel(t, noCluster("credential plugin tsh: relogin needed"))
	m.Update(m.connectCmd("")())

	cmd := m.handleLoginExit(loginExitMsg{session: m.connGen, context: "alpha", plugin: "tsh"})
	if cmd == nil || !m.connecting {
		t.Fatalf("after a successful login offline: cmd=%v connecting=%v, want a reconnect", cmd != nil, m.connecting)
	}
	if !strings.Contains(m.toast, "reconnecting") {
		t.Errorf("toast = %q, want it to say k10s is reconnecting", m.toast)
	}
}

// A failed login is reported, not retried: the plugin said why on the
// terminal, and reconnecting would only fail the same way.
func TestLoginFailureIsReportedNotRetried(t *testing.T) {
	m := loginModel(t, noCluster("credential plugin tsh: relogin needed"))
	m.Update(m.connectCmd("")())

	m.handleLoginExit(loginExitMsg{session: m.connGen, context: "alpha", plugin: "tsh", err: errors.New("exit status 1")})
	if m.connecting {
		t.Error("a failed login started a reconnect")
	}
	if !strings.Contains(m.toast, "✗ tsh: exit status 1") {
		t.Errorf("toast = %q, want the plugin's failure", m.toast)
	}
}

// Signing in with a working cluster on screen changes nothing on screen:
// reconnecting would drop the rows for nothing.
func TestLoginSuccessOnlineJustSaysSo(t *testing.T) {
	m := loginModel(t, func(string) (domain.Source, string) { return mock.New(""), "" })
	m.Update(m.connectCmd("")())
	if m.offline {
		t.Fatal("precondition: should be connected")
	}

	m.handleLoginExit(loginExitMsg{session: m.connGen, context: "alpha", plugin: "tsh"})
	if m.connecting {
		t.Error("nothing was wrong, yet a reconnect was started")
	}
	if !strings.Contains(m.toast, "✓ signed in with tsh") {
		t.Errorf("toast = %q", m.toast)
	}
}

// credFailure is what the backend reports when the plugin failed, as the
// UI sees it: an error with a CredentialPlugin method.
type credFailure struct{ plugin string }

func (c credFailure) Error() string            { return "credential plugin " + c.plugin + ": relogin needed" }
func (c credFailure) CredentialPlugin() string { return c.plugin }

// lapsedSource is a backend whose watches are failing on credentials.
type lapsedSource struct {
	domain.Source
	err error
}

func (l lapsedSource) LoadErrorFor(string, string) error { return l.err }

// A credential failure mid-session is told apart from an RBAC one: the
// advice is /login, not "check RBAC", and the header says so on every frame.
func TestCredentialFailureIsToldApartFromRBAC(t *testing.T) {
	if got := credentialPlugin(credFailure{"tsh"}); got != "tsh" {
		t.Errorf("credentialPlugin = %q, want tsh", got)
	}
	if got := credentialPlugin(fmt.Errorf("list pods: %w", credFailure{"aws"})); got != "aws" {
		t.Errorf("credentialPlugin through a wrap = %q, want aws", got)
	}
	if got := credentialPlugin(errors.New("forbidden")); got != "" {
		t.Errorf("credentialPlugin(forbidden) = %q, want none", got)
	}

	t.Setenv("K10S_CONFIG", t.TempDir()+"/config.yaml")
	m := New(lapsedSource{Source: mock.New(""), err: credFailure{"tsh"}})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	dismissOnboarding(m)

	lines := strings.Join(m.loadErrorLines(80, credFailure{"tsh"}), "\n")
	for _, want := range []string{"tsh credentials failed", "/login"} {
		if !strings.Contains(lines, want) {
			t.Errorf("load error for a credential failure lacks %q:\n%s", want, lines)
		}
	}
	if rbac := strings.Join(m.loadErrorLines(80, errors.New("forbidden")), "\n"); strings.Contains(rbac, "/login") {
		t.Errorf("an RBAC failure is advised to /login:\n%s", rbac)
	}

	v := zone.Scan(m.View())
	if !strings.Contains(v, "AUTH") || !strings.Contains(v, "tsh credentials failed · /login") {
		t.Errorf("the header does not flag the lapsed credentials:\n%s", strings.SplitN(v, "\n", 3)[0])
	}
}

// Once the kind on screen has lapsed credentials, a successful login
// reconnects — the informers' own retry would get there too, but not for
// up to a backoff period, with stale rows on screen meanwhile.
func TestLoginSuccessReconnectsWhenCredentialsLapsed(t *testing.T) {
	m := loginModel(t, func(string) (domain.Source, string) {
		return lapsedSource{Source: mock.New(""), err: credFailure{"tsh"}}, ""
	})
	m.Update(m.connectCmd("")())

	if cmd := m.handleLoginExit(loginExitMsg{session: m.connGen, context: "alpha", plugin: "tsh"}); cmd == nil || !m.connecting {
		t.Fatalf("cmd=%v connecting=%v, want a reconnect", cmd != nil, m.connecting)
	}
}
