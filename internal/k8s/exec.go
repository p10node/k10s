package k8s

import (
	"context"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	kterm "k8s.io/kubectl/pkg/util/term"
)

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
	req.VersionedParams(&corev1.PodExecOptions{
		Container: e.container,
		Command:   e.shellCmd,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(e.store.c.RestConfig, "POST", req.URL())
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
			Stderr:            e.stderr,
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
	shell := []string{"/bin/sh", "-c", "(bash || sh)"}
	return &execCommand{store: s, ns: effectiveNS(ns), pod: name, container: container, shellCmd: shell}, nil
}
