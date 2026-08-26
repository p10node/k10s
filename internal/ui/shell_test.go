package ui

import (
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"k10s/internal/domain"
	"k10s/internal/mock"
)

// fakeShell is a ShellSession that records what was typed and lets a test
// push output back, standing in for a real exec stream.
type fakeShell struct {
	mu     sync.Mutex
	typed  []byte
	out    chan []byte
	closed bool
	cols   int
	rows   int
}

func newFakeShell() *fakeShell { return &fakeShell{out: make(chan []byte, 8)} }

func (f *fakeShell) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.typed = append(f.typed, p...)
	return len(p), nil
}
func (f *fakeShell) Output() <-chan []byte { return f.out }
func (f *fakeShell) Resize(c, r int)       { f.mu.Lock(); f.cols, f.rows = c, r; f.mu.Unlock() }
func (f *fakeShell) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.out)
	}
	return nil
}
func (f *fakeShell) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *fakeShell) sent() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return string(f.typed)
}

// shellSource wraps the demo backend with a shell that actually opens.
type shellSource struct {
	domain.Source
	sh *fakeShell
}

func (s shellSource) ShellSession(kind, ns, name string, cols, rows int) (domain.ShellSession, error) {
	s.sh.Resize(cols, rows)
	return s.sh, nil
}

func openShell(t *testing.T, m *Model, sh *fakeShell) {
	t.Helper()
	// Deliberately not drainCmd: the command Update returns here waits on
	// the output stream, which would block a test with nothing to read.
	msg := m.startShellSession("pods", "default", "web-1")()
	m.Update(msg)
	if m.mode != modeShell {
		t.Fatalf("expected shell mode, got %v", m.mode)
	}
}

func newShellModel(t *testing.T) (*Model, *fakeShell) {
	t.Helper()
	sh := newFakeShell()
	m := newTestModel(t, shellSource{mock.New(""), sh})
	dismissOnboarding(m)
	return m, sh
}

func TestShellRendersInsideTheMainPanel(t *testing.T) {
	m, sh := newShellModel(t)
	openShell(t, m, sh)

	// Feed the emulator some output and let the model consume it.
	m.Update(shellOutMsg{gen: m.shellGen, data: []byte("hello from the pod"), ok: true})

	view := stripANSI(m.viewMain(70, 16).String())
	if !strings.Contains(view, "hello from the pod") {
		t.Errorf("shell output not rendered in the panel:\n%s", view)
	}
	if !strings.Contains(view, "shell") {
		t.Error("the panel should be titled as a shell")
	}
	if !strings.Contains(view, detachKey) {
		t.Error("the panel should say how to detach")
	}

	// The rest of the UI is still there — that is the whole point of not
	// handing the terminal over.
	full := stripANSI(m.View())
	if !strings.Contains(full, "Resources") {
		t.Error("the resource pane should remain visible during a shell")
	}
}

func TestShellForwardsKeystrokes(t *testing.T) {
	m, sh := newShellModel(t)
	openShell(t, m, sh)

	for _, r := range "ls -la" {
		m.handleKey(key(string(r)))
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if got := sh.sent(); got != "ls -la\r" {
		t.Errorf("sent %q, want %q", got, "ls -la\\r")
	}
}

func TestShellKeysAreNotAppShortcuts(t *testing.T) {
	m, sh := newShellModel(t)
	openShell(t, m, sh)

	// "q" would normally quit and "/" would open the prompt; inside a shell
	// they belong to the program running in the pod.
	m.handleKey(key("q"))
	m.handleKey(key("/"))
	if m.mode != modeShell {
		t.Error("app shortcuts must not fire while a shell has the keyboard")
	}
	if got := sh.sent(); got != "q/" {
		t.Errorf("sent %q, want the keys forwarded verbatim", got)
	}
}

func TestDetachKeyClosesTheSession(t *testing.T) {
	m, sh := newShellModel(t)
	openShell(t, m, sh)

	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	if m.mode != modeTable {
		t.Errorf("%s should return to the table, mode = %v", detachKey, m.mode)
	}
	if m.shellSess != nil {
		t.Error("detaching should drop the session")
	}
	if !sh.isClosed() {
		t.Error("detaching should close the remote session, not leak it")
	}
}

func TestShellEndsWhenTheStreamCloses(t *testing.T) {
	m, sh := newShellModel(t)
	openShell(t, m, sh)

	m.Update(shellOutMsg{gen: m.shellGen, ok: false})
	if m.mode != modeTable {
		t.Error("a finished shell should return to the table")
	}
	if !strings.Contains(m.toast, "ended") {
		t.Errorf("toast = %q, want it to say the session ended", m.toast)
	}
	_ = sh
}

func TestStaleShellOutputIsIgnored(t *testing.T) {
	m, sh := newShellModel(t)
	openShell(t, m, sh)
	stale := m.shellGen

	m.closeShell("")
	// Output from the session we just left must not resurrect the view.
	m.Update(shellOutMsg{gen: stale, data: []byte("late"), ok: true})
	if m.mode == modeShell {
		t.Error("stale output should not reopen the shell")
	}
}

func TestShellSizedToThePanel(t *testing.T) {
	m, sh := newShellModel(t)
	openShell(t, m, sh)

	wantCols, wantRows := m.shellSize()
	sh.mu.Lock()
	gotCols, gotRows := sh.cols, sh.rows
	sh.mu.Unlock()
	if gotCols != wantCols || gotRows != wantRows {
		t.Errorf("session sized %dx%d, want the panel's %dx%d", gotCols, gotRows, wantCols, wantRows)
	}
}

func TestNoShellFallsBackToAToast(t *testing.T) {
	m := newTestModel(t, mock.New("")) // demo backend has no shell
	dismissOnboarding(m)

	drainCmd(m, m.startShellSession("pods", "default", "web-1"))
	if m.mode == modeShell {
		t.Error("no shell should not open a shell panel")
	}
	if strings.HasPrefix(m.toast, "✗") {
		t.Errorf("toast = %q — a backend without a shell is not an error", m.toast)
	}
}

func TestKeyBytesMapsControlCodes(t *testing.T) {
	cases := []struct {
		msg  tea.KeyMsg
		want string
	}{
		{tea.KeyMsg{Type: tea.KeyCtrlC}, "\x03"},
		{tea.KeyMsg{Type: tea.KeyCtrlD}, "\x04"},
		{tea.KeyMsg{Type: tea.KeyEnter}, "\r"},
		{tea.KeyMsg{Type: tea.KeyTab}, "\t"},
		{tea.KeyMsg{Type: tea.KeyBackspace}, "\x7f"},
		{tea.KeyMsg{Type: tea.KeyUp}, "\x1b[A"},
	}
	for _, c := range cases {
		if got := string(keyBytes(c.msg)); got != c.want {
			t.Errorf("keyBytes(%v) = %q, want %q", c.msg.Type, got, c.want)
		}
	}
}
