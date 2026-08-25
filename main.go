package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"k10s/internal/ui"
)

func main() {
	zone.NewGlobal()
	defer zone.Close()

	p := tea.NewProgram(ui.New(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "k10s:", err)
		os.Exit(1)
	}
}
