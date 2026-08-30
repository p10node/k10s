package ui

import (
	"runtime"
	"strings"
	"testing"

	"github.com/p10node/k10s/internal/mock"
)

// run drives one typed line all the way through: the command runs off the
// event loop, so the test does what the event loop would.
func runPrompt(t *testing.T, m *Model, line string) string {
	t.Helper()
	cmd := m.runCommand(line)
	if cmd == nil {
		t.Fatalf("%q produced no command", line)
	}
	msg := cmd()
	m.Update(msg)
	return stripANSI(strings.Join(m.textLines, "\n"))
}

// The whole point of a command box: type something, see what it said. This
// used to be a toast reading "(not executed — read-only passthrough)".
func TestPromptRunsShellCommandsAndShowsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell syntax differs on Windows")
	}
	t.Setenv("SHELL", "/bin/sh")
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	body := runPrompt(t, m, "echo k10s-ok")
	if !strings.Contains(body, "k10s-ok") {
		t.Errorf("output = %q, want the command's own output", body)
	}
	if m.mode != modeText {
		t.Errorf("mode = %v, want the output in the text view", m.mode)
	}
	if m.textTitle != "$ echo k10s-ok" {
		t.Errorf("title = %q, want the command line", m.textTitle)
	}
	if m.busy {
		t.Error("still busy after the command came back")
	}
}

// A failing command's stderr is usually the useful part, so it is kept and
// the exit status is added under it rather than replacing it.
func TestPromptKeepsFailureOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell syntax differs on Windows")
	}
	t.Setenv("SHELL", "/bin/sh")
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	body := runPrompt(t, m, "echo boom >&2; exit 3")
	if !strings.Contains(body, "boom") {
		t.Errorf("output = %q, want the command's stderr", body)
	}
	if !strings.Contains(body, "exit status 3") {
		t.Errorf("output = %q, want the exit status", body)
	}
}

// A command that says nothing still has to look like it ran.
func TestPromptSaysWhenThereIsNoOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell syntax differs on Windows")
	}
	t.Setenv("SHELL", "/bin/sh")
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	if body := runPrompt(t, m, "true"); !strings.Contains(body, "(no output)") {
		t.Errorf("output = %q, want it to say there was none", body)
	}
}

// ctrl+a is off while the AI prompt is disabled: it says so and leaves the
// prompt in command mode, rather than silently doing nothing.
func TestAIModeIsDisabled(t *testing.T) {
	if !aiDisabled {
		t.Skip("AI is enabled again")
	}
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.togglePromptMode()
	if m.pmode != promptCmd {
		t.Error("ctrl+a switched to AI mode while the feature is disabled")
	}
	if m.confirm == nil || !m.confirm.notice {
		t.Fatal("ctrl+a should explain that AI is disabled")
	}

	// Dismissing a notice is not "cancelling" anything.
	m.handleKey(key("esc"))
	if m.confirm != nil {
		t.Error("esc should close the notice")
	}
	if m.toast == "cancelled" {
		t.Errorf("toast = %q — a notice has nothing to cancel", m.toast)
	}
}

// Even reached directly, the AI prompt refuses rather than calling out.
func TestAIPromptRefusesWhileDisabled(t *testing.T) {
	if !aiDisabled {
		t.Skip("AI is enabled again")
	}
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.pmode = promptAI

	if cmd := m.runCommand("why is my pod pending"); cmd != nil {
		t.Error("an AI prompt should not produce a request while disabled")
	}
	if m.confirm == nil || !m.confirm.notice {
		t.Error("submitting an AI prompt should explain that AI is disabled")
	}
	if strings.HasPrefix(m.busyLabel, "$ ") {
		t.Errorf("busy = %q — AI mode must not shell out", m.busyLabel)
	}
}

// ":" and "/" commands are not shell commands, whatever they look like.
func TestCommandsAreNotShelledOut(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.runCommand(":po")
	if strings.HasPrefix(m.busyLabel, "$ ") {
		t.Errorf("busy = %q — \":po\" is a k10s command, not a shell one", m.busyLabel)
	}
	if m.curKind().Key != "pods" {
		t.Errorf("kind = %q, want pods", m.curKind().Key)
	}
}

func TestShellCommandUsesTheUsersShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("$SHELL is a Unix notion")
	}
	t.Setenv("SHELL", "/bin/zsh")
	if sh, flag := shellCommand(); sh != "/bin/zsh" || flag != "-c" {
		t.Errorf("shellCommand() = %q %q, want the user's own shell", sh, flag)
	}
	t.Setenv("SHELL", "")
	if sh, _ := shellCommand(); sh != "/bin/sh" {
		t.Errorf("shellCommand() = %q with no $SHELL, want /bin/sh", sh)
	}
}
