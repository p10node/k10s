package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Plain text typed at the prompt — anything that is not a "/" or ":" command,
// and not an AI question — is run as a shell command and its output opens in
// the main panel.
//
// It used to be echoed and thrown away ("read-only passthrough"), which meant
// the one thing a command box is for did not happen: typing `date`, or
// `kubectl get pods -o wide`, produced a toast and nothing else.
//
// It runs off the event loop with a timeout, so a command that hangs (or
// wants a terminal k10s cannot give it) costs a spinner and then an error,
// never a frozen UI. Nothing is executed that the user did not type.

// shellTimeout bounds one command. Long enough for a slow kubectl call,
// short enough that an interactive program left waiting for a TTY gives up
// on its own.
const shellTimeout = 30 * time.Second

// shellCommand is the shell and flag used to run a typed line: the user's
// own $SHELL where there is one, so their aliases, functions and PATH are
// the ones they expect.
func shellCommand() (string, string) {
	if runtime.GOOS == "windows" {
		if sh := os.Getenv("COMSPEC"); sh != "" {
			return sh, "/c"
		}
		return "cmd", "/c"
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh, "-c"
	}
	return "/bin/sh", "-c"
}

// runShellCmd runs line and returns its combined output as a text view.
func (m *Model) runShellCmd(line string) tea.Cmd {
	title := "$ " + line
	m.startBusy(trunc(title, 48))
	m.toast = "… " + trunc(title, 48)

	sh, flag := shellCommand()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
		defer cancel()

		c := exec.CommandContext(ctx, sh, flag, line)
		c.Stdin = nil // no TTY to offer: an interactive program must not wait on one
		out, err := c.CombinedOutput()
		body := strings.TrimRight(string(out), "\n")

		switch {
		case ctx.Err() == context.DeadlineExceeded:
			body = appendLine(body, fmt.Sprintf("✗ timed out after %s — k10s runs commands without a terminal, so anything interactive will hang", shellTimeout))
		case err != nil:
			body = appendLine(body, "✗ "+err.Error())
		case body == "":
			body = "(no output)"
		}
		return textResultMsg{title: title, body: body}
	}
}

// appendLine adds a status line under whatever output there was, keeping the
// output itself — a failing command's stderr is usually the useful part.
func appendLine(body, line string) string {
	if body == "" {
		return line
	}
	return body + "\n\n" + line
}
