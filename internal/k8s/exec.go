package k8s

import (
	"context"
	"fmt"
	"io"
	"net/url"

	tea "github.com/charmbracelet/bubbletea"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	kterm "k8s.io/kubectl/pkg/util/term"
	"k8s.io/streaming/pkg/httpstream"
)

// newExecutor builds the executor every exec path uses: WebSocket first,
// falling back to SPDY when the upgrade is refused. This is the order
// kubectl settled on — SPDY is the old transport and is being switched off
// on newer clusters, while some proxies in front of an API server still
// only speak it.
func newExecutor(cfg *rest.Config, u *url.URL) (remotecommand.Executor, error) {
	spdyExec, err := remotecommand.NewSPDYExecutor(cfg, "POST", u)
	if err != nil {
		return nil, err
	}
	// RFC 6455 §4.1: a WebSocket handshake is a GET.
	wsExec, err := remotecommand.NewWebSocketExecutor(cfg, "GET", u.String())
	if err != nil {
		return nil, err
	}
	return remotecommand.NewFallbackExecutor(wsExec, spdyExec, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})
}

// ttyExecOptions is the request every interactive exec sends.
//
// Stderr is deliberately false. A TTY merges the pod's stdout and stderr
// onto one stream by definition, so a separate stderr stream has nothing to
// carry — and asking for one anyway is not merely redundant: the SPDY
// transport then completes the upgrade and delivers *nothing at all*, which
// looks exactly like a shell that opens onto a blank panel and ignores every
// keystroke. kubectl has always sent stderr=false with a TTY for this
// reason.
func ttyExecOptions(container string, cmd []string) *corev1.PodExecOptions {
	return &corev1.PodExecOptions{
		Container: container,
		Command:   cmd,
		Stdin:     true,
		Stdout:    true,
		Stderr:    false,
		TTY:       true,
	}
}

// shellCmd is what we run inside the pod: bash when the image has it, sh
// otherwise, since a distroless-ish image usually has only the latter.
// Probe quietly and replace the wrapper process with the selected shell so
// Alpine-style images do not begin the session with "bash: not found".
var shellCmd = []string{"/bin/sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh"}

// execCommand implements tea.ExecCommand around client-go's remotecommand,
// so Shell hands a real `kubectl exec -it`-equivalent session straight to
// tea.Exec — no shelling out to a kubectl binary.
type execCommand struct {
	store     *Store
	ns, pod   string
	container string
	shellCmd  []string

	stdin          io.Reader
	stdout, stderr io.Writer
}

func (e *execCommand) SetStdin(r io.Reader)  { e.stdin = r }
func (e *execCommand) SetStdout(w io.Writer) { e.stdout = w }
func (e *execCommand) SetStderr(w io.Writer) { e.stderr = w }

func (e *execCommand) Run() error {
	req := e.store.c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(e.ns).Name(e.pod).SubResource("exec")
	req.VersionedParams(ttyExecOptions(e.container, e.shellCmd), scheme.ParameterCodec)

	exec, err := newExecutor(e.store.c.RestConfig, req.URL())
	if err != nil {
		return fmt.Errorf("build exec: %w", err)
	}

	tty := kterm.TTY{In: e.stdin, Out: e.stdout, Raw: true}
	sizeQueue := tty.MonitorSize(tty.GetSize())

	return tty.Safe(func() error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:             e.stdin,
			Stdout:            e.stdout,
			Tty:               true,
			TerminalSizeQueue: sizeQueueAdapter{sizeQueue},
		})
	})
}

// sizeQueueAdapter bridges kubectl's TerminalSizeQueue (copied to decouple
// packages) to client-go's remotecommand.TerminalSizeQueue — same shape,
// different types.
type sizeQueueAdapter struct{ q kterm.TerminalSizeQueue }

func (a sizeQueueAdapter) Next() *remotecommand.TerminalSize {
	if a.q == nil {
		return nil
	}
	s := a.q.Next()
	if s == nil {
		return nil
	}
	return &remotecommand.TerminalSize{Width: s.Width, Height: s.Height}
}

// Shell returns an interactive exec session into the pod's first container,
// handed to the UI via tea.ExecProcess.
func (s *Store) Shell(kind, ns, name string) (tea.ExecCommand, error) {
	if kind != "pods" {
		return nil, fmt.Errorf("shell is only available for pods")
	}
	container, err := s.podContainer(ns, name)
	if err != nil {
		return nil, err
	}
	return &execCommand{store: s, ns: effectiveNS(ns), pod: name, container: container, shellCmd: shellCmd}, nil
}
