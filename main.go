package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"k10s/internal/domain"
	"k10s/internal/k8s"
	"k10s/internal/mock"
	"k10s/internal/ui"
)

// newSource tries the real cluster first (current kubeconfig context) and
// falls back to the offline demo when no cluster is reachable — same
// behaviour k10s has always documented ("mock mode — not connected to a
// real cluster"), just now backed by a real attempt first.
func newSource() (domain.Source, string) {
	store, err := k8s.NewStore("", "")
	if err != nil {
		return mock.New(""), "mock mode — " + err.Error()
	}
	return store, ""
}

func main() {
	k8s.SilenceLogging()

	zone.NewGlobal()
	defer zone.Close()

	src, warn := newSource()
	m := ui.New(src)
	if warn != "" {
		m.SetToast(warn)
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "k10s:", err)
		os.Exit(1)
	}
}
