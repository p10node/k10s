package k8s

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/p10node/k10s/internal/domain"
)

func (s *Store) podContainer(ns, name string) (string, error) {
	ens := effectiveNS(ns)
	p, err := s.podLister(ens).Pods(ens).Get(name)
	if err != nil {
		return "", err
	}
	if len(p.Spec.Containers) == 0 {
		return "", fmt.Errorf("%s has no containers", name)
	}
	return p.Spec.Containers[0].Name, nil
}

// logTarget resolves what to actually read logs from. Pods are themselves;
// a workload resolves to one of its pods, because "show me the logs of this
// Deployment" plainly means the logs of something it is running. Kinds with
// no logs at all return domain.ErrNoLogs so the caller can fall back to
// describe rather than reporting a failure.
func (s *Store) logTarget(kind, ns, name string) (pod, container string, err error) {
	ens := effectiveNS(ns)

	switch kind {
	case kPods:
		c, err := s.podContainer(ens, name)
		return name, c, err

	case kDeployments, kStatefulSet, kDaemonSets, kJobs, kReplicaSets:
		sel, err := s.workloadSelector(kind, ens, name)
		if err != nil {
			return "", "", err
		}
		pods, err := s.podLister(ens).Pods(ens).List(sel)
		if err != nil || len(pods) == 0 {
			return "", "", fmt.Errorf("no running pods found for %s/%s", kind, name)
		}
		// Prefer a running pod; a crash-looping one still has logs worth
		// reading, so fall back to the first either way.
		target := pods[0]
		for _, p := range pods {
			if p.Status.Phase == corev1.PodRunning {
				target = p
				break
			}
		}
		if len(target.Spec.Containers) == 0 {
			return "", "", fmt.Errorf("%s has no containers", target.Name)
		}
		return target.Name, target.Spec.Containers[0].Name, nil
	}

	return "", "", domain.ErrNoLogs
}

// workloadSelector reads the label selector off a workload so its pods can
// be found.
func (s *Store) workloadSelector(kind, ns, name string) (labels.Selector, error) {
	var m *metav1.LabelSelector
	switch kind {
	case kDeployments:
		d, err := s.deployLister(ns).Deployments(ns).Get(name)
		if err != nil {
			return nil, err
		}
		m = d.Spec.Selector
	case kStatefulSet:
		d, err := s.stsLister(ns).StatefulSets(ns).Get(name)
		if err != nil {
			return nil, err
		}
		m = d.Spec.Selector
	case kDaemonSets:
		d, err := s.dsLister(ns).DaemonSets(ns).Get(name)
		if err != nil {
			return nil, err
		}
		m = d.Spec.Selector
	case kJobs:
		d, err := s.jobLister(ns).Jobs(ns).Get(name)
		if err != nil {
			return nil, err
		}
		m = d.Spec.Selector
	case kReplicaSets:
		d, err := s.rsLister(ns).ReplicaSets(ns).Get(name)
		if err != nil {
			return nil, err
		}
		m = d.Spec.Selector
	default:
		return nil, domain.ErrNoLogs
	}
	if m == nil {
		return labels.Everything(), nil
	}
	return metav1.LabelSelectorAsSelector(m)
}

// Logs returns the last chunk of the target's log.
func (s *Store) Logs(kind, ns, name string) (string, error) {
	lines, _, err := s.LogsTail(kind, ns, name, defaultLogTail)
	if err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

// defaultLogTail is how much history the viewer opens with. Scrolling up
// asks for progressively more.
const defaultLogTail = 500

// LogsTail returns the last n lines, and whether older ones remain. The
// Kubernetes log API has no backwards cursor, so "older" means re-reading
// with a larger tail — which is exactly what the viewer does as you scroll
// up. more=false once the server returns fewer lines than asked for, i.e.
// the whole log now fits.
func (s *Store) LogsTail(kind, ns, name string, n int) ([]string, bool, error) {
	pod, container, err := s.logTarget(kind, ns, name)
	if err != nil {
		return nil, false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tail := int64(n)
	data, err := s.c.Clientset.CoreV1().Pods(effectiveNS(ns)).
		GetLogs(pod, &corev1.PodLogOptions{Container: container, TailLines: &tail, Timestamps: true}).
		DoRaw(ctx)
	if err != nil {
		return nil, false, err
	}

	lines := splitLogLines(string(data))
	return lines, len(lines) >= n, nil
}

// splitLogLines trims the trailing newline the API always sends, so an empty
// last line doesn't masquerade as a log entry.
func splitLogLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// LogsFollow streams new lines from the pod's first container (like
// `kubectl logs -f`) until stop is called.
func (s *Store) LogsFollow(kind, ns, name string) (<-chan string, func(), error) {
	pod, container, err := s.logTarget(kind, ns, name)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Tail 0: the caller has already loaded the history it wants via
	// LogsTail, so replaying any here would show every one of those lines
	// twice — the stream's job is only the lines that arrive from now on.
	tail := int64(0)
	req := s.c.Clientset.CoreV1().Pods(effectiveNS(ns)).
		GetLogs(pod, &corev1.PodLogOptions{Container: container, Follow: true, TailLines: &tail, Timestamps: true})
	stream, err := req.Stream(ctx)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	lines := make(chan string, 256)
	go func() {
		defer close(lines)
		defer stream.Close()
		sc := bufio.NewScanner(stream)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-ctx.Done():
				return
			}
		}
		if err := sc.Err(); err != nil && err != io.EOF && ctx.Err() == nil {
			select {
			case lines <- "── stream error: " + err.Error():
			default:
			}
		}
	}()

	return lines, cancel, nil
}
