// Command shot renders the TUI headlessly so layouts can be inspected without
// an interactive terminal. Usage: shot <w> <h> <comma-separated key tokens>
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/muesli/termenv"

	"k10s/internal/ui"
)

var special = map[string]tea.KeyType{
	"tab": tea.KeyTab, "enter": tea.KeyEnter, "esc": tea.KeyEsc,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"pgdown": tea.KeyPgDown, "pgup": tea.KeyPgUp,
	"ctrl+a": tea.KeyCtrlA, "backspace": tea.KeyBackspace,
}

func main() {
	// Never touch the user's real ~/.k10s from the dev renderer, and never
	// share state between invocations either — each run gets its own throwaway
	// path unless the caller sets K10S_CONFIG explicitly (e.g. to test
	// persistence across two shot calls on purpose).
	if os.Getenv("K10S_CONFIG") == "" {
		dir, err := os.MkdirTemp("", "k10s-shot-")
		if err == nil {
			os.Setenv("K10S_CONFIG", filepath.Join(dir, "config.yaml"))
		}
	}
	w, h := 140, 44
	if len(os.Args) > 2 {
		w, _ = strconv.Atoi(os.Args[1])
		h, _ = strconv.Atoi(os.Args[2])
	}
	seq := ""
	if len(os.Args) > 3 {
		seq = os.Args[3]
	}

	lipgloss.SetColorProfile(termenv.TrueColor)
	zone.NewGlobal()
	defer zone.Close()

	var m tea.Model = ui.New()
	m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})

	for _, tok := range strings.Split(seq, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if t, ok := special[tok]; ok {
			m, _ = m.Update(tea.KeyMsg{Type: t})
			continue
		}
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tok)})
	}
	fmt.Print(m.View())
}
