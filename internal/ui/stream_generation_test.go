package ui

import (
	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
	"testing"
)

type streamSource struct {
	domain.Source
	stopped int
}

func (s *streamSource) LogsTail(kind, ns, name string, n int) ([]string, bool, error) {
	return []string{ns + "/" + name}, true, nil
}
func (s *streamSource) LogsFollow(kind, ns, name string) (<-chan string, func(), error) {
	ch := make(chan string)
	return ch, func() { s.stopped++; close(ch) }, nil
}
func TestStaleLogHistoryAndLiveGeneration(t *testing.T) {
	s := &streamSource{Source: mock.New("")}
	m := newTestModel(t, s)
	m.Update(m.startLogs("pods", "A", "same")())
	older := m.loadOlderLogs()
	m.Update(m.startLogs("pods", "B", "same")())
	m.textLines = []string{"keep"}
	result := older().(logOlderMsg)
	result.lines = []string{"old-A", "keep"}
	m.Update(result)
	if len(m.textLines) != 1 || m.textLines[0] != "keep" {
		t.Errorf("stale history changed B: %v", m.textLines)
	}
	gen := m.logGen
	m.backToTable()
	m.Update(logLineMsg{gen: gen, line: "old live", ok: true})
	if len(m.textLines) != 1 {
		t.Errorf("stale live changed table: %v", m.textLines)
	}
}

func TestStaleShellGenerationCleanup(t *testing.T) {
	m, sh := newShellModel(t)
	cmd := m.startShellSession("pods", "A", "same")
	m.backToTable()
	m.Update(cmd())
	if m.mode == modeShell || !sh.isClosed() {
		t.Fatalf("stale shell reopened=%v closed=%v", m.mode == modeShell, sh.isClosed())
	}
}

func TestStaleInitialLogGenerationCleanup(t *testing.T) {
	s := &streamSource{Source: mock.New("")}
	m := newTestModel(t, s)
	old := m.startLogs("pods", "A", "same")
	current := m.startLogs("pods", "B", "same")
	m.Update(current())
	m.Update(old())
	if m.logNS != "B" {
		t.Fatalf("stale initial log changed namespace: %q", m.logNS)
	}
	if s.stopped != 1 {
		t.Fatalf("stale stream stop count=%d", s.stopped)
	}
	m.backToTable()
	if s.stopped != 2 {
		t.Fatalf("current stream stop count=%d", s.stopped)
	}
}
