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

	"github.com/p10node/k10s/internal/mock"
	"github.com/p10node/k10s/internal/ui"
)

var special = map[string]tea.KeyType{
	"tab": tea.KeyTab, "enter": tea.KeyEnter, "esc": tea.KeyEsc,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"pgdown": tea.KeyPgDown, "pgup": tea.KeyPgUp,
	"ctrl+a": tea.KeyCtrlA, "backspace": tea.KeyBackspace,
	"ctrl+s": tea.KeyCtrlS, "ctrl+p": tea.KeyCtrlP, "shift+tab": tea.KeyShiftTab,
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

	var m tea.Model = ui.New(mock.New(""))
	m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})

	for _, tok := range strings.Split(seq, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		var cmd tea.Cmd
		if t, ok := special[tok]; ok {
			m, cmd = m.Update(tea.KeyMsg{Type: t})
		} else {
			m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tok)})
		}
		m = drain(m, cmd)
	}
	fmt.Print(m.View())
}

// drain synchronously runs cmd, and keeps following the chain as long as
// each step's message is one of the ui package's own async-result messages
// (describe/yaml/logs/actions/AI) — that's what lets this headless renderer
// resolve those without a real tea.Program event loop. Anything else
// (cursor blink, ticks, textinput internals) is applied once for state
// consistency and then left alone, since those recur forever by design and
// were never run by this renderer before it started resolving async
// commands at all.
func drain(m tea.Model, cmd tea.Cmd) tea.Model {
	for cmd != nil {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				m = drain(m, c)
			}
			return m
		}
		if !ui.IsAsyncMsg(msg) {
			m, _ = m.Update(msg)
			return m
		}
		var next tea.Cmd
		m, next = m.Update(msg)
		cmd = next
	}
	return m
}
