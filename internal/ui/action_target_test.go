package ui

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

type targetSource struct {
	domain.Source
	deletes          atomic.Int32
	closes           atomic.Int32
	applies          atomic.Int32
	entered, release chan struct{}
	next             domain.Source
}

func (s *targetSource) Delete(kind, ns, name string) error {
	s.deletes.Add(1)
	if s.entered != nil {
		close(s.entered)
		<-s.release
	}
	return nil
}
func (s *targetSource) Apply(kind, ns, name, yaml string) error { s.applies.Add(1); return nil }
func (s *targetSource) Close()                                  { s.closes.Add(1) }

// awaitCloses tolerates the asynchronous Close a retired lease performs.
func awaitCloses(t *testing.T, n *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for n.Load() != want {
		if time.Now().After(deadline) {
			t.Fatalf("source close=%d want %d", n.Load(), want)
		}
		time.Sleep(time.Millisecond)
	}
}
func (s *targetSource) SwitchContext(name string) (domain.Source, error) { return s.next, nil }

func TestConfirmedDeleteCannotRetargetBackend(t *testing.T) {
	for _, switched := range []bool{false, true} {
		t.Run(map[bool]string{false: "control", true: "switch"}[switched], func(t *testing.T) {
			a := &targetSource{Source: mock.New("")}
			b := &targetSource{Source: mock.New("")}
			a.next = b
			m := newTestModel(t, a)
			m.fireAction(Action{ID: domain.ADelete})
			cmd := m.confirm.onOK(m)
			if switched {
				sw := m.switchContextCmd("B")
				m.Update(sw())
			}
			result := cmd()
			if a.deletes.Load() != 1 || b.deletes.Load() != 0 {
				t.Fatalf("delete A=%d B=%d", a.deletes.Load(), b.deletes.Load())
			}
			before := m.toast
			m.Update(result)
			if switched && m.toast != before {
				t.Fatalf("stale success changed B toast: %q", m.toast)
			}
		})
	}
}

func (s *targetSource) Describe(kind, ns, name string) (string, error) {
	s.deletes.Add(1)
	return "describe", nil
}
func (s *targetSource) YAML(kind, ns, name string) (string, error) {
	s.deletes.Add(1)
	return "kind: Pod\n", nil
}
func (s *targetSource) Restart(kind, ns, name string) error    { s.deletes.Add(1); return nil }
func (s *targetSource) Cordon(name string, disable bool) error { s.deletes.Add(1); return nil }
func (s *targetSource) Drain(name string) error                { s.deletes.Add(1); return nil }
func (s *targetSource) Scale(kind, ns, name string, n int) (int, error) {
	s.deletes.Add(1)
	return n, nil
}
func (s *targetSource) TopPod(ns, name string) (string, error) { s.deletes.Add(1); return "top", nil }
func (s *targetSource) TopNode(name string) (string, error)    { s.deletes.Add(1); return "top", nil }

func TestActionTargetAllDeferredRoutes(t *testing.T) {
	for _, tc := range []struct {
		kind   string
		action string
	}{
		{"pods", domain.ADescribe}, {"pods", domain.AYAML}, {"pods", domain.ATop},
		{"deployments", domain.ARestart}, {"nodes", domain.ACordon}, {"nodes", domain.ADrain}, {"nodes", domain.ATop},
	} {
		t.Run(tc.kind+string(tc.action), func(t *testing.T) {
			a := &targetSource{Source: mock.New("")}
			b := &targetSource{Source: mock.New("")}
			a.next = b
			m := newTestModel(t, a)
			m.gotoKind(tc.kind, "")
			cmd := m.fireAction(Action{ID: tc.action})
			if m.confirm != nil {
				cmd = m.confirm.onOK(m)
			}
			if cmd == nil {
				t.Fatal("missing command")
			}
			m.Update(m.switchContextCmd("B")())
			cmd()
			if a.deletes.Load() != 1 || b.deletes.Load() != 0 {
				t.Fatalf("calls A=%d B=%d", a.deletes.Load(), b.deletes.Load())
			}
		})
	}
	t.Run("scale", func(t *testing.T) {
		a := &targetSource{Source: mock.New("")}
		b := &targetSource{Source: mock.New("")}
		a.next = b
		m := newTestModel(t, a)
		m.gotoKind("deployments", "")
		cmd := m.runSlash(":scale 2")
		m.Update(m.switchContextCmd("B")())
		cmd()
		if a.deletes.Load() != 1 || b.deletes.Load() != 0 {
			t.Fatalf("scale A=%d B=%d", a.deletes.Load(), b.deletes.Load())
		}
	})
}

func TestActionTargetLeaseDelaysClose(t *testing.T) {
	a := &targetSource{Source: mock.New(""), entered: make(chan struct{}), release: make(chan struct{})}
	b := &targetSource{Source: mock.New("")}
	a.next = b
	m := newTestModel(t, a)
	m.fireAction(Action{ID: domain.ADelete})
	cmd := m.confirm.onOK(m)
	done := make(chan struct{})
	go func() { cmd(); close(done) }()
	<-a.entered
	sw := m.switchContextCmd("B")
	m.Update(sw())
	early := a.closes.Load()
	close(a.release)
	<-done
	if early != 0 {
		t.Fatalf("source closed during in-flight Delete: %d", early)
	}
	awaitCloses(t, &a.closes, 1)
}
