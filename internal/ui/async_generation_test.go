package ui

import (
	"github.com/p10node/k10s/internal/plugin"
	"testing"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

type switchingSource struct {
	domain.Source
	entered        chan string
	bReady, cReady chan struct{}
	b, c           domain.Source
}

func (s *switchingSource) SwitchContext(name string) (domain.Source, error) {
	s.entered <- name
	if name == "B" {
		<-s.bReady
		return s.b, nil
	}
	<-s.cReady
	return s.c, nil
}
func TestPluginGenerationAndStaleConfirmation(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.connGen = 9
	item := plugin.Named{Name: "fixture", Plugin: plugin.Plugin{Command: "/nonexistent/k10s-fixture", Background: true}}
	msg := m.runPlugin(item, m.pluginVars())().(actionResultMsg)
	if msg.session != 9 {
		t.Errorf("plugin completion session=%d", msg.session)
	}
	item.Confirm = true
	m.firePlugin(item)
	cb := m.confirm.onOK
	m.connGen++
	if cmd := cb(m); cmd != nil {
		t.Fatal("stale plugin confirmation admitted")
	}
}
func TestAICompletionGeneration(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.connGen = 4
	m.viewGen = 5
	m.cfg.key = ""
	msg := m.askAI("offline fixture")().(textResultMsg)
	if msg.session != m.connGen || msg.view != m.viewGen {
		t.Fatalf("AI result untagged: %+v", msg)
	}
}
func TestLogFallbackActionTarget(t *testing.T) {
	a := &targetSource{Source: mock.New("")}
	b := &targetSource{Source: mock.New("")}
	a.next = b
	m := newTestModel(t, a)
	_, cmd := m.Update(logStartMsg{kind: "pods", ns: "A", name: "same", err: domain.ErrNoLogs})
	m.Update(m.switchContextCmd("B")())
	cmd()
	if a.deletes.Load() != 1 || b.deletes.Load() != 0 {
		t.Fatalf("fallback A=%d B=%d", a.deletes.Load(), b.deletes.Load())
	}
}
func TestShellCommandGeneration(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.connGen = 3
	cmd := m.runShellCmd("printf fixture")
	m.Update(cmd())
	if m.mode != modeText || m.textTitle != "$ printf fixture" {
		t.Fatalf("current shell result discarded: mode=%v title=%s", m.mode, m.textTitle)
	}
	old := m.runShellCmd("printf old")
	m.backToTable()
	m.Update(old())
	if m.mode != modeTable {
		t.Fatal("stale shell command reopened pane")
	}
}
func TestStaleConfirmationRejected(t *testing.T) {
	for _, action := range []string{domain.ADelete, domain.ARestart, domain.ADrain} {
		t.Run(action, func(t *testing.T) {
			a := &targetSource{Source: mock.New("")}
			a.next = mock.New("")
			m := newTestModel(t, a)
			if action == domain.ARestart {
				m.gotoKind("deployments", "")
			}
			if action == domain.ADrain {
				m.gotoKind("nodes", "")
			}
			m.fireAction(Action{ID: action})
			confirm := m.confirm.onOK
			m.Update(m.switchContextCmd("B")())
			if cmd := confirm(m); cmd != nil {
				cmd()
			}
			if a.deletes.Load() != 0 {
				t.Fatal("stale confirmation executed against retired source")
			}
		})
	}
}
func TestStaleTextGenerationCannotReopenPane(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	old := m.runFetch("old", func() (string, error) { return "obsolete", nil })
	current := m.runFetch("new", func() (string, error) { return "current", nil })
	m.Update(current())
	m.Update(old())
	if m.textTitle != "new" {
		t.Fatalf("stale text replaced latest: %q", m.textTitle)
	}
	late := m.runFetch("late", func() (string, error) { return "late", nil })
	m.backToTable()
	m.Update(late())
	if m.mode != modeTable {
		t.Fatal("stale text reopened closed pane")
	}
}

func TestContextSwitchGenerationLatestIntent(t *testing.T) {
	b := &targetSource{Source: mock.New("")}
	c := &targetSource{Source: mock.New("")}
	a := &switchingSource{Source: mock.New(""), entered: make(chan string), bReady: make(chan struct{}), cReady: make(chan struct{}), b: b, c: c}
	m := newTestModel(t, a)
	cb := m.switchContextCmd("B")
	rb := make(chan interface{}, 1)
	go func() { rb <- cb() }()
	<-a.entered
	cc := m.switchContextCmd("C")
	rc := make(chan interface{}, 1)
	go func() { rc <- cc() }()
	<-a.entered
	close(a.cReady)
	m.Update(<-rc)
	close(a.bReady)
	m.Update(<-rb)
	if m.src != c {
		t.Error("older B completion replaced latest C intent")
	}
	awaitCloses(t, &b.closes, 1)
	if c.closes.Load() != 0 {
		t.Error("latest source C was closed")
	}
}
