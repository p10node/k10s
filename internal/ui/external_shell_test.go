package ui

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

// fakeExec stands in for the k8s exec command S hands to main.go.
type fakeExec struct{}

func (fakeExec) SetStdin(io.Reader)  {}
func (fakeExec) SetStdout(io.Writer) {}
func (fakeExec) SetStderr(io.Writer) {}
func (fakeExec) Run() error          { return nil }

// execSource wraps the demo backend with a Shell that resolves, and records
// whether anything closed it.
type execSource struct {
	domain.Source
	cmd tea.ExecCommand
	err error

	mu     sync.Mutex
	closed bool
}

func (s *execSource) Shell(kind, ns, name string) (tea.ExecCommand, error) { return s.cmd, s.err }
func (s *execSource) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}
func (s *execSource) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func shellOutAction(t *testing.T) Action {
	t.Helper()
	for _, a := range Actions {
		if a.ID == domain.AShellOut {
			return a
		}
	}
	t.Fatal("no S action in the action table")
	return Action{}
}

// pressShellOut fires S and feeds the resolved shell back, returning what
// Update answered with.
func pressShellOut(t *testing.T, m *Model) tea.Cmd {
	t.Helper()
	cmd := m.fireAction(shellOutAction(t))
	if cmd == nil {
		t.Fatalf("S returned no command; toast = %q", m.toast)
	}
	_, next := m.Update(cmd())
	return next
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestShellOutQuitsAndHandsTheShellToMain(t *testing.T) {
	src := &execSource{Source: mock.New(""), cmd: fakeExec{}}
	m := newTestModel(t, src)
	dismissOnboarding(m)
	name := m.curName()

	if !isQuit(pressShellOut(t, m)) {
		t.Fatal("S should quit k10s once the shell is ready")
	}
	cmd, target := m.ExitShell()
	if cmd == nil {
		t.Fatal("ExitShell() = nil after S, main.go would have nothing to run")
	}
	if want := "po/" + name; !strings.Contains(target, want) {
		t.Errorf("target = %q, want it to name %s", target, want)
	}
	if m.busy {
		t.Error("the loading indicator is still on after the shell resolved")
	}
}

// The exec command builds its request from the store when main.go runs it,
// after the program has exited, so quitting must not close the source.
func TestShellOutKeepsTheSourceOpen(t *testing.T) {
	src := &execSource{Source: mock.New(""), cmd: fakeExec{}}
	m := newTestModel(t, src)
	dismissOnboarding(m)

	pressShellOut(t, m)
	if m.sourceOwner != nil && m.sourceOwner.retired {
		t.Error("S retired the source it is about to exec through")
	}
	if src.isClosed() {
		t.Error("S closed the source it is about to exec through")
	}
}

func TestShellOutWithoutAShellStaysInK10s(t *testing.T) {
	m := newTestModel(t, mock.New("")) // the demo backend has no real shell
	dismissOnboarding(m)

	if isQuit(pressShellOut(t, m)) {
		t.Fatal("S quit k10s with no shell to run")
	}
	if cmd, _ := m.ExitShell(); cmd != nil {
		t.Error("ExitShell() is set without a shell")
	}
	if !strings.Contains(m.toast, "no interactive shell") {
		t.Errorf("toast = %q, want the no-shell notice", m.toast)
	}
}

func TestShellOutReportsAFailedLookup(t *testing.T) {
	src := &execSource{Source: mock.New(""), err: errors.New("no running pods found for deployments/web")}
	m := newTestModel(t, src)
	dismissOnboarding(m)

	if isQuit(pressShellOut(t, m)) {
		t.Fatal("S quit k10s after the lookup failed")
	}
	if !strings.Contains(m.toast, "no running pods") {
		t.Errorf("toast = %q, want the lookup error", m.toast)
	}
}

// A shell that resolves after the user has already moved on must not quit
// k10s under them.
func TestShellOutIgnoresAStaleResult(t *testing.T) {
	src := &execSource{Source: mock.New(""), cmd: fakeExec{}}
	m := newTestModel(t, src)
	dismissOnboarding(m)

	cmd := m.fireAction(shellOutAction(t))
	m.backToTable() // what opening another view does
	if _, next := m.Update(cmd()); isQuit(next) {
		t.Fatal("a stale shell result quit k10s")
	}
	if c, _ := m.ExitShell(); c != nil {
		t.Error("a stale shell result was kept")
	}
}

func TestShellOutIsOnShiftS(t *testing.T) {
	src := &execSource{Source: mock.New(""), cmd: fakeExec{}}
	m := newTestModel(t, src)
	dismissOnboarding(m)

	cmd := m.handleKey(key("S"))
	if cmd == nil {
		t.Fatalf("S did nothing on a pod; toast = %q", m.toast)
	}
	if _, next := m.Update(cmd()); !isQuit(next) {
		t.Error("S on a pod should quit into the shell")
	}
}

func TestActionsPaneListsShellOutForPods(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	pane := stripANSI(strings.Join(m.viewActions(40, 30).Lines, "\n"))
	if !strings.Contains(pane, "Shell (terminal)") {
		t.Errorf("Actions pane lacks the S action:\n%s", pane)
	}
}
