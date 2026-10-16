package ui

import (
	"fmt"
	"strings"
	"testing"

	zone "github.com/lrstanley/bubblezone"
)

func manyContexts(m *Model, n int) {
	for i := 1; i <= n; i++ {
		m.kubeCtxs = append(m.kubeCtxs, fmt.Sprintf("ctx-%02d", i))
	}
}

func rowOnScreen(m *Model) (string, bool) {
	name := m.ctxChoices()[m.ctxIdx]
	row := fmt.Sprintf("%d %s", m.ctxIdx+1, name)
	return name, strings.Contains(stripANSI(zone.Scan(m.View())), row)
}

// Enter used to pick a context below the fold that nobody could see.
func TestContextPickerKeepsTheCursorOnScreen(t *testing.T) {
	m, _ := demoModel(t)
	manyContexts(m, 60)
	m.showContextChooser()

	onScreen := func(when string) {
		t.Helper()
		if want, ok := rowOnScreen(m); !ok {
			t.Errorf("%s: highlighted context %q (row %d of %d) is not on screen",
				when, want, m.ctxIdx+1, len(m.ctxChoices()))
		}
	}

	onScreen("on open")
	for i := 0; i < 45; i++ {
		m.Update(key("down"))
	}
	onScreen("after moving down past the fold")
	for i := 0; i < 100; i++ {
		m.Update(key("down"))
	}
	onScreen("at the end of the list")
	for i := 0; i < 100; i++ {
		m.Update(key("up"))
	}
	onScreen("back at the top")
}

// k10s-demo sorts after most kubeconfig contexts, so the picker opened with nothing highlighted.
func TestContextPickerOpensOnACurrentContextBelowTheFold(t *testing.T) {
	m, _ := demoModel(t)
	manyContexts(m, 60)
	m.Update(m.runSlash("/demo")())
	m.showContextChooser()

	cur := m.ctxChoices()[m.ctxIdx]
	if cur != m.src.ClusterInfo().Context {
		t.Fatalf("picker opened on %q, not the current context %q", cur, m.src.ClusterInfo().Context)
	}
	if _, ok := rowOnScreen(m); !ok {
		t.Errorf("picker opened on %q (row %d of %d) but it is not on screen",
			cur, m.ctxIdx+1, len(m.ctxChoices()))
	}
}

// The legend is the only thing that says k10s-demo is not a real cluster.
func TestContextPickerKeepsTheLegendWithALongList(t *testing.T) {
	m, _ := demoModel(t)
	manyContexts(m, 60)
	m.showContextChooser()

	if screen := zone.Scan(m.View()); !strings.Contains(screen, "leave it") {
		t.Errorf("the demo legend is cut off by a long context list")
	}
}
