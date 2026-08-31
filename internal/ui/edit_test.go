package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/mock"
)

func TestEditExitRestoresMouseCapture(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	path := filepath.Join(t.TempDir(), "service.yaml")
	if err := os.WriteFile(path, []byte("kind: Service\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, cmd := m.Update(editExitMsg{
		kind: "services",
		ns:   "default",
		name: "api-gateway",
		path: path,
	})
	if cmd == nil {
		t.Fatal("editor exit returned no commands")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("editor exit command = %T, want a mouse-resume + apply batch", msg)
	}
	var resumed, applied bool
	for _, child := range batch {
		msg := child()
		switch msg.(type) {
		case actionResultMsg:
			applied = true
		default:
			// Bubble Tea keeps this message type private; its concrete name is
			// still enough to verify that the command enables cell-motion mode.
			resumed = fmt.Sprintf("%T", msg) == "tea.enableMouseCellMotionMsg"
		}
	}
	if !resumed {
		t.Error("editor exit did not re-enable mouse cell-motion capture")
	}
	if !applied {
		t.Error("editor exit did not preserve the edited-file apply command")
	}
}

func TestEditExitKeepsMouseDisabledInCopyMode(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.mouseOff = true
	if cmd := m.resumeMouseAfterExec(); cmd != nil {
		t.Error("external-process return should not enable mouse capture in copy mode")
	}
}
