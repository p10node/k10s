package k8s

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/p10node/k10s/internal/domain"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const logReadMaxBytes = 4 * 1024 * 1024

// A line past this limit ends the scan with bufio.ErrTooLong, which the
// reader reports as a stream error. Match the historical LogsFollow budget so
// one large JSON event or stack dump does not kill the follow stream.
const logReadMaxLine = 1024 * 1024

var logStreamStartupTimeout = 20 * time.Second

type pinnedLogs struct {
	store                     *Store
	namespace, pod, container string
	uid                       types.UID
	ctx                       context.Context
	cancel                    context.CancelFunc
	stream                    io.ReadCloser
	lines                     chan string
	once                      sync.Once
}

func (s *Store) OpenLogSession(kind, ns, name string, tail int) (domain.LogSession, error) {
	pod, container := name, ""
	if kind != kPods {
		var err error
		pod, container, err = s.logTarget(kind, ns, name)
		if err != nil {
			return nil, err
		}
	}
	ns = effectiveNS(ns)
	lookup, cancelLookup := context.WithTimeout(context.Background(), 15*time.Second)
	obj, err := s.c.Clientset.CoreV1().Pods(ns).Get(lookup, pod, metav1.GetOptions{})
	cancelLookup()
	if err != nil {
		return nil, err
	}
	if obj.UID == "" {
		return nil, fmt.Errorf("pod has no UID; refresh before opening logs")
	}
	if container == "" {
		if len(obj.Spec.Containers) == 0 {
			return nil, fmt.Errorf("pod has no containers")
		}
		container = obj.Spec.Containers[0].Name
	}
	ctx, cancel := context.WithCancel(context.Background())
	n := int64(max(1, min(tail, 100000)))
	timer := time.AfterFunc(logStreamStartupTimeout, cancel)
	stream, err := s.c.Clientset.CoreV1().Pods(ns).GetLogs(pod, &corev1.PodLogOptions{Container: container, Follow: true, TailLines: &n, Timestamps: true}).Stream(ctx)
	if !timer.Stop() {
		// The watchdog fired while Stream was returning: ctx is canceled, so
		// a stream that did come back would die on its first read.
		cancel()
		if err == nil {
			_ = stream.Close()
		}
		return nil, fmt.Errorf("log stream for %s/%s did not start within %s", ns, pod, logStreamStartupTimeout)
	}
	if err != nil {
		cancel()
		return nil, err
	}
	l := &pinnedLogs{store: s, namespace: ns, pod: pod, container: container, uid: obj.UID, ctx: ctx, cancel: cancel, stream: stream, lines: make(chan string, 64)}
	go l.read()
	return l, nil
}

func (l *pinnedLogs) Lines() <-chan string { return l.lines }
func (l *pinnedLogs) Close()               { l.once.Do(func() { l.cancel(); _ = l.stream.Close() }) }
func (l *pinnedLogs) read() {
	defer close(l.lines)
	defer func() { _ = l.stream.Close() }()
	sc := bufio.NewScanner(l.stream)
	sc.Buffer(make([]byte, 4096), logReadMaxLine)
	for sc.Scan() {
		select {
		case l.lines <- sc.Text():
		case <-l.ctx.Done():
			return
		}
	}
	if err := sc.Err(); err != nil && l.ctx.Err() == nil {
		select {
		case l.lines <- "── stream error: " + err.Error():
		case <-l.ctx.Done():
		}
	}
}

func (l *pinnedLogs) History(n int) ([]string, bool, error) {
	ctx, cancel := context.WithTimeout(l.ctx, 20*time.Second)
	defer cancel()
	pod, err := l.store.c.Clientset.CoreV1().Pods(l.namespace).Get(ctx, l.pod, metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}
	if pod.UID != l.uid {
		return nil, false, fmt.Errorf("pod was replaced; reopen its logs")
	}
	tail := int64(max(1, min(n, 100000)))
	stream, err := l.store.c.Clientset.CoreV1().Pods(l.namespace).GetLogs(l.pod, &corev1.PodLogOptions{Container: l.container, TailLines: &tail, Timestamps: true}).Stream(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = stream.Close() }()
	sc := bufio.NewScanner(stream)
	sc.Buffer(make([]byte, 4096), logReadMaxLine)
	// Keep the newest lines that fit the byte budget: they are the ones that
	// overlap what the view already holds. Anything older is unreachable
	// through this session, so a truncated response reports no further history.
	var lines []string
	start, bytes, truncated := 0, 0, false
	for sc.Scan() {
		line := sc.Text()
		lines = append(lines, line)
		bytes += len(line) + 1
		for bytes > logReadMaxBytes && start < len(lines) {
			bytes -= len(lines[start]) + 1
			lines[start] = ""
			start++
			truncated = true
		}
		if start > 0 && start >= len(lines)/2 {
			lines = append([]string(nil), lines[start:]...)
			start = 0
		}
	}
	if err := sc.Err(); err != nil {
		return nil, false, fmt.Errorf("read log history: %w", err)
	}
	lines = lines[start:]
	if truncated {
		return lines, false, nil
	}
	return lines, len(lines) >= int(tail), nil
}
