package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/p10node/k10s/internal/mock"
)

func TestNamespaceCommandInvalidatesPendingViews(t *testing.T) {
	t.Run("describe", func(t *testing.T) {
		m := newTestModel(t, mock.New(""))
		pending := m.runFetch("namespace A", func() (string, error) { return "old namespace content", nil })
		m.runCommand(":ns kube-system")
		m.Update(pending())
		if m.namespace != "kube-system" || m.mode != modeTable {
			t.Fatalf("namespace=%s mode=%v: stale describe accepted", m.namespace, m.mode)
		}
	})
	t.Run("logs", func(t *testing.T) {
		session := &uiPinnedLogs{ch: make(chan string, 1)}
		m := newTestModel(t, &uiLogOpener{Source: mock.New(""), session: session})
		pending := m.startLogs("pods", "default", "pod")()
		m.runCommand(":ns kube-system")
		m.Update(pending)
		if m.mode != modeTable || !session.closed {
			t.Fatalf("stale logs reopened=%v closed=%v", m.mode != modeTable, session.closed)
		}
	})
	t.Run("editor seed", func(t *testing.T) {
		m := newTestModel(t, mock.New(""))
		path := filepath.Join(t.TempDir(), "seed.yaml")
		if err := os.WriteFile(path, []byte("fixture seed"), 0600); err != nil {
			t.Fatal(err)
		}
		pending := editFetchedMsg{source: m.src, session: m.connGen, view: m.viewGen, kind: "pods", ns: "default", name: "pod", path: path}
		m.runCommand(":ns kube-system")
		_, cmd := m.Update(pending)
		if cmd != nil {
			t.Fatal("old namespace editor opened")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unused editor seed not cleaned: %v", err)
		}
	})
	t.Run("active logs", func(t *testing.T) {
		session := &uiPinnedLogs{ch: make(chan string, 1)}
		m := newTestModel(t, &uiLogOpener{Source: mock.New(""), session: session})
		m.Update(m.startLogs("pods", "default", "pod")())
		m.runCommand(":ns kube-system")
		if !session.closed || m.mode != modeTable || m.logSession != nil {
			t.Fatal("namespace change retained active old log view")
		}
	})
}
