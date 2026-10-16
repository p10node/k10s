package ui

import (
	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
	"strings"
	"testing"
)

type ownedSource struct {
	*targetSource
	sh    *fakeShell
	stops int
}

func (s *ownedSource) ShellSession(kind, ns, name string, c, r int) (domain.ShellSession, error) {
	return s.sh, nil
}
func (s *ownedSource) PortForward(kind, ns, name string) (string, func(), error) {
	return "localhost:1234", func() { s.stops++ }, nil
}
func TestSessionResourceLease(t *testing.T) {
	for _, shell := range []bool{true, false} {
		t.Run(map[bool]string{true: "shell", false: "forward"}[shell], func(t *testing.T) {
			a := &ownedSource{targetSource: &targetSource{Source: mock.New("")}, sh: newFakeShell()}
			a.next = mock.New("")
			m := newTestModel(t, a)
			var cmd = m.startPortForward
			if shell {
				cmd = m.startShellSession
			}
			pending := cmd("pods", "A", "same")
			m.Update(m.switchContextCmd("B")())
			if a.closes.Load() != 0 {
				t.Error("source closed before pending resource returned")
			}
			m.Update(pending())
			if shell && !a.sh.isClosed() {
				t.Error("stale shell leaked")
			}
			if !shell && (a.stops != 1 || len(m.portFwds) != 0) {
				t.Errorf("stale forward leaked: stops=%d registered=%d", a.stops, len(m.portFwds))
			}
			awaitCloses(t, &a.closes, 1)
		})
	}
}
func TestContextSwitchCleansActiveResources(t *testing.T) {
	a := &ownedSource{targetSource: &targetSource{Source: mock.New("")}, sh: newFakeShell()}
	a.next = mock.New("")
	m := newTestModel(t, a)
	m.Update(m.startPortForward("pods", "A", "same")())
	m.Update(m.startShellSession("pods", "A", "same")())
	m.Update(m.switchContextCmd("B")())
	if !a.sh.isClosed() || a.stops != 1 || len(m.portFwds) != 0 {
		t.Fatalf("active resources survived switch: shell=%v stops=%d", a.sh.isClosed(), a.stops)
	}
}

// A second press while the dial is in flight cancels it instead of opening a
// second tunnel; the orphaned result is stopped when it lands, and a later
// press starts afresh.
func TestForwardSecondPressCancelsInFlight(t *testing.T) {
	a := &ownedSource{targetSource: &targetSource{Source: mock.New("")}, sh: newFakeShell()}
	m := newTestModel(t, a)
	pending := m.startPortForward("pods", "A", "same")
	if pending == nil || !m.busy {
		t.Fatal("first press did not start a dial")
	}
	if cmd := m.startPortForward("pods", "A", "same"); cmd != nil {
		t.Fatal("second press started another dial")
	}
	if m.busy || !strings.Contains(m.toast, "cancelled") {
		t.Fatalf("cancel not acknowledged: busy=%v toast=%q", m.busy, m.toast)
	}
	m.Update(pending())
	if a.stops != 1 || len(m.portFwds) != 0 {
		t.Fatalf("orphaned forward kept: stops=%d registered=%d", a.stops, len(m.portFwds))
	}
	again := m.startPortForward("pods", "A", "same")
	if again == nil {
		t.Fatal("third press did not start a fresh dial")
	}
	m.Update(again())
	if len(m.portFwds) != 1 || a.stops != 1 {
		t.Fatalf("fresh forward not registered: stops=%d registered=%d", a.stops, len(m.portFwds))
	}
	for _, stop := range m.portFwds {
		stop()
	}
}
