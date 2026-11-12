package k8s

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Credential plugins and the terminal.
//
// A kubeconfig user is often not a token but a program — tsh (Teleport),
// aws, gke-gcloud-auth-plugin, kubelogin — that client-go runs to get one.
// Under kubectl that program shares the terminal: it can print "tap your
// security key" and read an MFA code from stdin. Under a TUI it cannot.
// Bubble Tea owns stdin, so a prompt swallows keystrokes meant for k10s, and
// whatever the plugin prints lands on top of the frame. That is the k9s
// failure (derailed/k9s#2048): a session that looks hung, with kill -9 as
// the only way out.
//
// Two rules follow, and this file is both of them.
//
//  1. The plugin never gets the terminal while the TUI is up. New marks
//     stdin unavailable — what kubectl does when stdin is spoken for, as in
//     `kubectl exec -i` — so client-go runs the plugin without it and the
//     plugin fails fast instead of waiting for input nobody can type.
//     Logging in is done outside k10s, in a shell, then `r`.
//
//  2. What the plugin says is kept, not painted over. client-go hands every
//     plugin the os.Stderr of the moment the first client is built, so
//     CaptureStderr swaps a pipe in before that, and the last line a plugin
//     wrote becomes the error the user reads: "Having problems with relogin,
//     please use 'tsh login'" instead of "exit code 1".

// stdinUnavailableMessage is the reason client-go gives when a plugin that
// declares `interactiveMode: Always` is refused before it runs.
const stdinUnavailableMessage = "k10s owns the terminal, so the credential plugin cannot prompt; " +
	"log in with it in another shell, then press r"

// nonInteractive keeps an exec plugin off the terminal (rule 1 above).
// Nothing else about the plugin changes, so its own declaration still
// decides between running without stdin (Never, IfAvailable) and refusing
// to run at all (Always).
func nonInteractive(ep *clientcmdapi.ExecConfig) {
	if ep == nil {
		return
	}
	ep.StdinUnavailable = true
	ep.StdinUnavailableMessage = stdinUnavailableMessage
}

// stderrTap is os.Stderr rerouted through a pipe k10s reads (rule 2). It
// remembers the last non-empty line and when it arrived: plugins say one
// useful thing, at the end, and a message from a different attempt must not
// be pinned on this one. It also keeps the last tailSize bytes of what was
// written, line by line, for the one reader that needs more than a line:
// main, after a panic in the TUI, whose stack trace Bubble Tea writes to
// os.Stderr.
type stderrTap struct {
	mu       sync.Mutex
	terminal *os.File // what os.Stderr was before the swap
	pipe     *os.File // the write end now in os.Stderr
	last     string
	at       time.Time
	tail     []byte        // the last tailSize bytes read, line by line
	read     int64         // bytes read so far, for drain to see the pipe go quiet
	done     chan struct{} // closed when the reader stops
}

// tailSize bounds stderrTap.tail. A stack trace from inside Bubble Tea is a
// few KiB.
const tailSize = 64 << 10

// quietFor is how long drain waits for a write before it takes the pipe as
// quiet.
const quietFor = 50 * time.Millisecond

var (
	captureOnce sync.Once
	tap         = &stderrTap{}
)

// start swaps the pipe in and drains it for as long as the write end lives.
func (t *stderrTap) start() error {
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	t.terminal, t.pipe = os.Stderr, w
	t.done = make(chan struct{})
	os.Stderr = w
	go func() {
		defer close(t.done)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			raw := sc.Text()
			line := strings.TrimSpace(raw)
			t.mu.Lock()
			t.keep(raw)
			t.read += int64(len(raw)) + 1
			if line != "" {
				t.last, t.at = line, time.Now()
			}
			t.mu.Unlock()
		}
		// A line too long for the buffer ends the scan; nothing to do about
		// it but stop reading, and nothing is lost that mattered.
		_ = sc.Err()
		_ = r.Close()
	}()
	return nil
}

// keep adds one line to the tail and drops the oldest bytes past tailSize.
// The line is as the scanner gives it, plus a newline: a \r before the
// newline is gone, and a last line written without one gets one. Neither
// changes a stack trace. The caller holds t.mu.
func (t *stderrTap) keep(line string) {
	t.tail = append(append(t.tail, line...), '\n')
	if over := len(t.tail) - tailSize; over > 0 {
		t.tail = append(t.tail[:0], t.tail[over:]...)
	}
}

// drain waits, up to wait, until nothing new has come through the pipe for
// quietFor. Then it closes the write end, so that the reader can finish what
// is still in the pipe, and returns the tail once it has, or once wait is up
// again: a credential plugin that is still running holds the pipe open too.
//
// The quiet comes first because Run does not wait for a command's goroutine.
// After a panic in one, Bubble Tea ends the program and only then writes the
// stack trace, from that goroutine, which can be after Run has returned; a
// write after the close fails and the trace is lost.
func (t *stderrTap) drain(wait time.Duration) []byte {
	if t.pipe == nil {
		return nil
	}
	deadline := time.Now().Add(wait)
	for prev := int64(-1); time.Now().Before(deadline); time.Sleep(quietFor) {
		t.mu.Lock()
		n := t.read
		t.mu.Unlock()
		if n == prev {
			break
		}
		prev = n
	}
	_ = t.pipe.Close()
	select {
	case <-t.done:
	case <-time.After(wait):
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.tail...)
}

// said returns the last line written at or after since, "" if none. The
// pipe is read on another goroutine, so a line the plugin wrote just before
// exiting may still be in flight when its error arrives; said waits a
// moment for it rather than reporting silence that is not.
func (t *stderrTap) said(since time.Time) string {
	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		t.mu.Lock()
		line, at := t.last, t.at
		t.mu.Unlock()
		if !at.Before(since) {
			// The prefix is the plugin's; the error carries its own.
			line = strings.TrimPrefix(line, "ERROR: ")
			return strings.TrimPrefix(line, "error: ")
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// CaptureStderr routes os.Stderr through k10s for the rest of the process
// and returns what it was, for the few things still allowed to write to the
// terminal: the editor, a foreground plugin, main's own last words. Call it
// once, before the first client is built — client-go copies os.Stderr for
// the plugin at that point and never looks again. If the pipe cannot be
// made nothing is captured and the terminal stays the plugin's stderr, as
// it always was.
func CaptureStderr() (terminal *os.File) {
	captureOnce.Do(func() {
		if err := tap.start(); err != nil {
			tap.terminal = os.Stderr
		}
	})
	return tap.terminal
}

// ReleaseStderr puts the terminal back as os.Stderr. For after the TUI has
// exited: anything said then belongs on the screen, not in the pipe.
func ReleaseStderr() {
	if tap.terminal != nil {
		os.Stderr = tap.terminal
	}
}

// DrainStderr ends the capture and returns the last 64 KiB of what it read,
// line by line, for main when the TUI ended in a panic: Bubble Tea prints
// the panic to stdout but its stack trace to os.Stderr, which was the pipe.
// It waits up to wait for the pipe to go quiet, and up to wait again for the
// reader to finish (see stderrTap.drain). Call it after ReleaseStderr, at
// the end of the process: a credential plugin started after it would find
// its stderr closed. Without a capture it returns nil.
func DrainStderr(wait time.Duration) []byte {
	return tap.drain(wait)
}

// TerminalStderr is the terminal's stderr whether or not CaptureStderr has
// run: the right destination for a process that takes the terminal over.
func TerminalStderr() *os.File {
	if tap.terminal != nil {
		return tap.terminal
	}
	return os.Stderr
}

// CredentialPlugin names the exec credential plugin a kubeconfig context
// signs in with — "tsh", "aws", "kubelogin" — or "" when the context uses
// none. It reads kubeconfig only, so it can be asked before connecting,
// which is when the UI wants to say who it is waiting on.
func CredentialPlugin(path, context string) string {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path != "" {
		rules.ExplicitPath = path
	}
	raw, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).RawConfig()
	if err != nil {
		return ""
	}
	return credentialPlugin(raw, context)
}

func credentialPlugin(raw clientcmdapi.Config, context string) string {
	if context == "" {
		context = raw.CurrentContext
	}
	ctx := raw.Contexts[context]
	if ctx == nil {
		return ""
	}
	auth := raw.AuthInfos[ctx.AuthInfo]
	if auth == nil || auth.Exec == nil {
		return ""
	}
	return filepath.Base(auth.Exec.Command)
}

// CredentialPluginError is the context's exec credential plugin failing,
// which is a different situation from the API server not answering: the
// cluster may be fine, and the fix is to log in again, outside k10s.
type CredentialPluginError struct {
	Plugin string // the command's base name: tsh, aws, kubelogin
	Said   string // the last line it wrote to stderr, "" if it was silent
	Err    error  // client-go's own error, for errors.Is and errors.As
}

func (e *CredentialPluginError) Error() string {
	what := e.Said
	if what == "" {
		what = pluginClause(e.Err)
	}
	return "credential plugin " + e.Plugin + ": " + what
}

func (e *CredentialPluginError) Unwrap() error { return e.Err }

// credentialPluginFailed reports whether err is the plugin failing rather
// than the request it was fetching credentials for. client-go formats the
// plugin's error into the request's with %v, so the chain is cut there and
// the text is all there is to go on.
func credentialPluginFailed(err error) bool {
	return err != nil && strings.Contains(err.Error(), "getting credentials: ")
}

// pluginClause cuts client-go's error down to the part about the plugin:
// `Get "https://…/version": getting credentials: exec: executable tsh
// failed with exit code 1` → `exec: executable tsh failed with exit code 1`.
func pluginClause(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, "getting credentials: "); i >= 0 {
		msg = msg[i+len("getting credentials: "):]
	}
	return msg
}

// requestTimedOut reports whether err is a request's own budget running
// out: rest.Config.Timeout fires through the request context, and the
// transport says so in a net.Error.
func requestTimedOut(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
