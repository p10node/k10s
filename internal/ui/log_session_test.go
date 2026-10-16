package ui

import (
	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
	"testing"
)

type uiPinnedLogs struct {
	ch              chan string
	closed, history bool
}

func (s *uiPinnedLogs) Lines() <-chan string { return s.ch }
func (s *uiPinnedLogs) Close()               { s.closed = true }
func (s *uiPinnedLogs) History(int) ([]string, bool, error) {
	s.history = true
	return []string{"old", "initial"}, false, nil
}

type uiLogOpener struct {
	domain.Source
	session *uiPinnedLogs
	opened  bool
}

func (s *uiLogOpener) OpenLogSession(_, _, _ string, _ int) (domain.LogSession, error) {
	s.opened = true
	return s.session, nil
}
func TestUILogsUsePinnedSessionForStreamAndHistory(t *testing.T) {
	session := &uiPinnedLogs{ch: make(chan string, 1)}
	src := &uiLogOpener{Source: mock.New(""), session: session}
	m := newTestModel(t, src)
	cmd := m.startLogs("pods", "default", "pod")
	msg := cmd()
	m.Update(msg)
	if !src.opened || m.logCh != session.ch {
		t.Fatal("initial log path bypassed pinned session")
	}
	m.textLines = []string{"initial"}
	m.logMore = true
	history := m.loadOlderLogs()
	if history == nil {
		t.Fatal("history command missing")
	}
	history()
	if !session.history {
		t.Fatal("history re-resolved mutable workload instead of pinned session")
	}
	m.backToTable()
	if !session.closed {
		t.Fatal("leaving log view leaked session")
	}
}

func TestHistoryOnlyLogsDoNotReuseRetiredSession(t *testing.T) {
	session := &uiPinnedLogs{ch: make(chan string, 1)}
	m := newTestModel(t, &uiLogOpener{Source: mock.New(""), session: session})
	m.Update(m.startLogs("pods", "default", "pod")())
	m.backToTable()
	if m.logSession != nil {
		t.Fatal("leaving logs retained the previous backend log session")
	}
	m.replaceSource(mock.New(""))
	m.Update(m.startLogs("pods", "default", "pod")())
	if cmd := m.loadOlderLogs(); cmd != nil {
		msg := cmd().(logOlderMsg)
		if msg.err != nil || len(msg.lines) == 0 {
			t.Fatalf("demo history did not load: %v", msg.err)
		}
	} else {
		t.Fatal("history command missing")
	}
	if session.history {
		t.Fatal("history-only view queried retired backend session")
	}
}
