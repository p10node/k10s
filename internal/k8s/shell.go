package k8s

import (
	"context"
	"io"
	"sync"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/p10node/k10s/internal/domain"
)

// execSession is a live `kubectl exec -it` rendered inside k10s rather than
// by handing the whole terminal over: keystrokes are written to stdin, and
// the pod's output arrives as raw bytes for the caller's terminal emulator.
type execSession struct {
	stdin  *io.PipeWriter
	out    chan []byte
	sizes  chan remotecommand.TerminalSize
	cancel context.CancelFunc

	closeOnce sync.Once
}

func (s *execSession) Write(p []byte) (int, error) { return s.stdin.Write(p) }
func (s *execSession) Output() <-chan []byte       { return s.out }

// Resize is best-effort: a dropped resize just means the remote keeps the
// previous size until the next one, which beats blocking the UI thread.
func (s *execSession) Resize(cols, rows int) {
	select {
	case s.sizes <- remotecommand.TerminalSize{Width: uint16(cols), Height: uint16(rows)}:
	default:
	}
}

func (s *execSession) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		s.stdin.Close()
	})
	return nil
}

// sizeQueue feeds resize events to remotecommand.
type sizeQueue struct {
	ch   chan remotecommand.TerminalSize
	done <-chan struct{}
}

func (q sizeQueue) Next() *remotecommand.TerminalSize {
	select {
	case s := <-q.ch:
		return &s
	case <-q.done:
		return nil
	}
}

// ShellSession opens an interactive exec against the pod (or a workload's
// pod) and streams it back for in-panel rendering.
func (s *Store) ShellSession(kind, ns, name string, cols, rows int) (domain.ShellSession, error) {
	pod, container, err := s.logTarget(kind, ns, name)
	if err != nil {
		// logTarget's "no logs" really means "not a pod-backed thing", which
		// is equally true for exec.
		if err == domain.ErrNoLogs {
			return nil, domain.ErrNoShell
		}
		return nil, err
	}

	req := s.c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(effectiveNS(ns)).Name(pod).SubResource("exec")
	req.VersionedParams(ttyExecOptions(container, shellCmd), scheme.ParameterCodec)

	exec, err := newExecutor(s.c.RestConfig, req.URL())
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	sess := &execSession{
		stdin:  pw,
		out:    make(chan []byte, 64),
		sizes:  make(chan remotecommand.TerminalSize, 4),
		cancel: cancel,
	}
	sess.Resize(cols, rows)

	// outWriter hands each chunk to the UI. It never blocks forever: if the
	// UI stops draining (a closed panel), the session is simply done.
	ow := &chanWriter{ch: sess.out, done: ctx.Done()}

	go func() {
		defer close(sess.out)
		defer pr.Close()
		err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  pr,
			Stdout: ow,
			// Stderr stays nil to match the request above: with a TTY the
			// pod has only one output stream.
			Tty:               true,
			TerminalSizeQueue: sizeQueue{ch: sess.sizes, done: ctx.Done()},
		})
		if err != nil && ctx.Err() == nil {
			ow.Write([]byte("\r\n\x1b[31m" + err.Error() + "\x1b[0m\r\n"))
		}
	}()

	return sess, nil
}

type chanWriter struct {
	ch   chan<- []byte
	done <-chan struct{}
}

func (w *chanWriter) Write(p []byte) (int, error) {
	b := make([]byte, len(p)) // the caller reuses its buffer
	copy(b, p)
	select {
	case w.ch <- b:
		return len(p), nil
	case <-w.done:
		return 0, io.ErrClosedPipe
	}
}
