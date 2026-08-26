package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hinshun/vt10x"

	"k10s/internal/theme"
)

// An interactive shell rendered inside the main panel.
//
// The alternative — handing the whole terminal over with tea.Exec — means
// the rest of k10s disappears while you are in a pod. Keeping the shell in
// its pane means the resource list, the header and the status bar stay
// visible, at the cost of running a small terminal emulator (vt10x) over
// the exec stream.
//
// detachKey is how you get out. ctrl+] is the long-standing telnet/docker
// convention and, unlike esc or ctrl+c, is not something a shell or the
// program running in it wants for itself.
const detachKey = "ctrl+]"

// shellOutMsg carries a chunk of terminal output; shellEndMsg says the
// session finished on its own (the shell exited, the pod died).
type shellOutMsg struct {
	gen  int
	data []byte
	ok   bool
}

// startShellSession opens an in-panel shell, sized to the pane it will be
// drawn in.
func (m *Model) startShellSession(kind, ns, name string) tea.Cmd {
	cols, rows := m.shellSize()
	src := m.src
	m.startBusy("shell " + name)

	return func() tea.Msg {
		sess, err := src.ShellSession(kind, ns, name, cols, rows)
		return shellStartMsg{kind: kind, ns: ns, name: name, sess: sess, cols: cols, rows: rows, err: err}
	}
}

// shellSize is the emulator geometry for the current layout — the main
// panel's interior, minus the status row the viewer adds.
func (m *Model) shellSize() (cols, rows int) {
	l := m.layout()
	cols = maxi(20, l.mainW-2)
	rows = maxi(5, l.midH-3)
	return cols, rows
}

func waitShellOut(gen int, ch <-chan []byte) tea.Cmd {
	return func() tea.Msg {
		b, ok := <-ch
		return shellOutMsg{gen: gen, data: b, ok: ok}
	}
}

// handleShellKey forwards a keystroke to the running shell. Everything goes
// through except the detach key — inside a shell, the shell owns the
// keyboard.
func (m *Model) handleShellKey(msg tea.KeyMsg) tea.Cmd {
	if msg.String() == detachKey {
		m.closeShell("detached — the session was ended")
		return nil
	}
	if m.shellSess == nil {
		return nil
	}
	if b := keyBytes(msg); len(b) > 0 {
		m.shellSess.Write(b)
	}
	return nil
}

// keyBytes turns a bubbletea key event into the bytes a terminal would have
// sent. bubbletea has already decoded them, so this maps back.
func keyBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyRunes:
		s := string(msg.Runes)
		if msg.Alt {
			return append([]byte{0x1b}, s...)
		}
		return []byte(s)
	case tea.KeySpace:
		return []byte(" ")
	case tea.KeyEnter:
		return []byte("\r")
	case tea.KeyTab:
		return []byte("\t")
	case tea.KeyBackspace:
		return []byte{0x7f}
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	case tea.KeyEsc:
		return []byte{0x1b}
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyHome:
		return []byte("\x1b[H")
	case tea.KeyEnd:
		return []byte("\x1b[F")
	case tea.KeyPgUp:
		return []byte("\x1b[5~")
	case tea.KeyPgDown:
		return []byte("\x1b[6~")
	}
	// Ctrl combinations map to their control codes: ctrl+a is 0x01 and so
	// on, which is what the terminal driver would have produced.
	if s := msg.String(); strings.HasPrefix(s, "ctrl+") && len(s) == 6 {
		c := s[5]
		if c >= 'a' && c <= 'z' {
			return []byte{c - 'a' + 1}
		}
	}
	return nil
}

// closeShell tears down the session and returns to the table.
func (m *Model) closeShell(toast string) {
	if m.shellSess != nil {
		m.shellSess.Close()
		m.shellSess = nil
	}
	m.shellTerm = nil
	m.shellGen++
	m.mode = modeTable
	if toast != "" {
		m.toast = toast
	}
}

// ---- rendering -----------------------------------------------------------

// vtColor maps a terminal colour onto the active theme. Default fg/bg follow
// the theme so an idle shell doesn't punch a differently-coloured hole in
// the UI; the 16 ANSI colours map onto theme colours where there is an
// obvious counterpart, and 256-colour indices fall back to the theme's
// foreground rather than guessing.
func vtColor(th theme.Theme, c vt10x.Color, isBG bool) lipgloss.Color {
	switch c {
	case vt10x.DefaultBG:
		return th.Bg
	case vt10x.DefaultFG, vt10x.DefaultCursor:
		return th.Fg
	}
	switch c {
	case vt10x.Black:
		return th.Bg
	case vt10x.Red, vt10x.LightRed:
		return th.Err
	case vt10x.Green, vt10x.LightGreen:
		return th.Ok
	case vt10x.Yellow, vt10x.LightYellow:
		return th.Warn
	case vt10x.Blue, vt10x.LightBlue:
		return th.Accent
	case vt10x.Magenta, vt10x.LightMagenta:
		return th.Accent2
	case vt10x.Cyan, vt10x.LightCyan:
		return th.Accent2
	case vt10x.LightGrey, vt10x.DarkGrey:
		return th.Subtle
	case vt10x.White:
		return th.Fg
	}
	if isBG {
		return th.Bg
	}
	return th.Fg
}

// shellBody renders the emulator's screen into panel lines.
func (m *Model) shellBody(inner, rows int) []string {
	th := m.th()
	if m.shellTerm == nil {
		return []string{lipgloss.NewStyle().Background(th.Bg).Foreground(th.Subtle).Render(" starting shell…")}
	}

	viewRows := maxi(1, rows-1)
	cols, trows := m.shellTerm.Size()
	cur := m.shellTerm.Cursor()
	showCur := m.shellTerm.CursorVisible()

	out := make([]string, 0, rows)
	for y := 0; y < trows && len(out) < viewRows; y++ {
		var b strings.Builder
		for x := 0; x < cols && x < inner; x++ {
			g := m.shellTerm.Cell(x, y)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			fg, bg := vtColor(th, g.FG, false), vtColor(th, g.BG, true)
			// Draw the cursor as a block so you can see where typing lands.
			if showCur && x == cur.X && y == cur.Y {
				fg, bg = th.Bg, th.Accent
			}
			b.WriteString(lipgloss.NewStyle().Background(bg).Foreground(fg).Render(string(ch)))
		}
		out = append(out, padBG(b.String(), inner, th.Bg))
	}
	for len(out) < viewRows {
		out = append(out, "")
	}

	out = append(out, m.shellStatusLine(inner))
	return out
}

func (m *Model) shellStatusLine(inner int) string {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	left := s(th.Ok).Bold(true).Render(" ● shell") +
		s(th.Subtle).Render("  keys go to the pod")
	right := detachKey + " to detach"
	gap := inner - lipgloss.Width(left) - len(right) - 1
	if gap < 1 {
		gap = 1
	}
	return left + s(th.Bg).Render(spaces(gap)) + s(th.Warn).Render(right) + s(th.Bg).Render(" ")
}
