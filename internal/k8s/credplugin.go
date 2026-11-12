package k8s

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/p10node/k10s/internal/domain"
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
// remembers only the last non-empty line and when it arrived: plugins say
// one useful thing, at the end, and a message from a different attempt
// must not be pinned on this one.
type stderrTap struct {
	mu       sync.Mutex
	terminal *os.File // what os.Stderr was before the swap
	pipe     *os.File // the write end now in os.Stderr
	last     string
	at       time.Time
}

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
	os.Stderr = w
	go func() {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			t.mu.Lock()
			t.last, t.at = line, time.Now()
			t.mu.Unlock()
		}
		// A line too long for the buffer ends the scan; nothing to do about
		// it but stop reading, and nothing is lost that mattered.
		_ = sc.Err()
		_ = r.Close()
	}()
	return nil
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

// CredentialPlugin names the plugin, for callers that know this error only
// through an interface (the UI, which does not import this package).
func (e *CredentialPluginError) CredentialPlugin() string { return e.Plugin }

// credentialPlugin names this client's plugin, "" when the context signs in
// some other way.
func (s *Store) credentialPlugin() string {
	if s.c == nil || s.c.RestConfig == nil || s.c.RestConfig.ExecProvider == nil {
		return ""
	}
	return filepath.Base(s.c.RestConfig.ExecProvider.Command)
}

// LoginCommand is the way to sign in again with a kubeconfig context's
// credential plugin: the plugin's own command, ready to be handed the
// terminal (tea.ExecProcess). nil when the context has no plugin.
//
// Running the plugin itself, rather than some login command of the tool's
// (`tsh login`, `aws sso login`), is what makes this work for every plugin:
// the exec protocol is the one contract they all honour, and with a
// terminal each does its own relogin, MFA, or device-code dance and caches
// the result where its next non-interactive run will find it.
func LoginCommand(path, context string) *domain.Login {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path != "" {
		rules.ExplicitPath = path
	}
	raw, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).RawConfig()
	if err != nil {
		return nil
	}
	return loginCommand(raw, context)
}

func loginCommand(raw clientcmdapi.Config, context string) *domain.Login {
	if context == "" {
		context = raw.CurrentContext
	}
	ctx := raw.Contexts[context]
	if ctx == nil {
		return nil
	}
	auth := raw.AuthInfos[ctx.AuthInfo]
	if auth == nil || auth.Exec == nil {
		return nil
	}
	ep := auth.Exec

	cmd := exec.Command(ep.Command, ep.Args...)
	cmd.Env = os.Environ()
	for _, kv := range ep.Env {
		cmd.Env = append(cmd.Env, kv.Name+"="+kv.Value)
	}
	cmd.Env = append(cmd.Env, "KUBERNETES_EXEC_INFO="+execInfo(ep, raw.Clusters[ctx.Cluster]))
	// What the plugin prints to stdout is the credential itself — for tsh a
	// certificate and its private key — and belongs to client-go, never on
	// a screen. Prompts go to stderr, which the terminal keeps.
	cmd.Stdout = io.Discard

	return &domain.Login{
		Plugin: filepath.Base(ep.Command),
		Cmd:    cmd,
		Alt:    tshAlt(filepath.Base(ep.Command), ep.Args),
	}
}

// execInfo is the KUBERNETES_EXEC_INFO client-go would set: the request as
// an ExecCredential, saying the plugin has the terminal, and the cluster
// when the plugin asked to be told about it.
func execInfo(ep *clientcmdapi.ExecConfig, cluster *clientcmdapi.Cluster) string {
	spec := map[string]any{"interactive": true}
	if ep.ProvideClusterInfo && cluster != nil {
		c := map[string]any{"server": cluster.Server}
		if len(cluster.CertificateAuthorityData) > 0 {
			c["certificate-authority-data"] = base64.StdEncoding.EncodeToString(cluster.CertificateAuthorityData)
		}
		spec["cluster"] = c
	}
	apiVersion := ep.APIVersion
	if apiVersion == "" {
		apiVersion = "client.authentication.k8s.io/v1beta1"
	}
	b, _ := json.Marshal(map[string]any{"apiVersion": apiVersion, "kind": "ExecCredential", "spec": spec})
	return string(b)
}

// tshAlt is Teleport's other way in. Two setups do not fit the exec plugin
// at all and tsh says so: per-session MFA, whose certificates last about a
// minute, and a proxy behind an L7 load balancer, which refuses direct
// Kubernetes clients. `tsh proxy kube` holds the session in its own
// terminal instead, and the kube cluster's name is in the plugin's args.
func tshAlt(plugin string, args []string) string {
	if plugin != "tsh" {
		return ""
	}
	for i, a := range args {
		if v, ok := strings.CutPrefix(a, "--kube-cluster="); ok && v != "" {
			return "tsh proxy kube " + v + " --exec"
		}
		if a == "--kube-cluster" && i+1 < len(args) {
			return "tsh proxy kube " + args[i+1] + " --exec"
		}
	}
	return ""
}

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
