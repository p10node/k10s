package ui

import (
	"testing"

	"github.com/p10node/k10s/internal/config"
	"github.com/p10node/k10s/internal/mock"
)

// Zooming is a layout choice that should outlive the session, like folding
// a group: the next launch opens the way the last one was left.
func TestZoomPersists(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.setZoomed(true)
	if !m.zoomed {
		t.Fatal("setZoomed(true) did not zoom")
	}
	c, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !c.Zoomed {
		t.Error("the saved config does not record the zoom")
	}

	restarted := New(mock.New(""))
	if !restarted.zoomed {
		t.Error("zoom was lost after a restart")
	}

	restarted.setZoomed(false)
	c, err = config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if c.Zoomed {
		t.Error("restoring the panes did not clear the saved zoom")
	}
}
