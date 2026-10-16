package k8s

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"strings"
	"sync"
	"time"
)

const drainTimeout = 5 * time.Minute

// Drain preflights before cordoning, evicts the remaining pods concurrently
// and waits for the original pod UIDs to disappear. Failures after cordon are
// partial; no automatic uncordon occurs.
func (s *Store) Drain(name string) error {
	return s.drain(context.Background(), name, drainWait)
}

// drainErrors keeps every pod's failure reachable through errors.Is/As while
// rendering on one line for the toast.
type drainErrors []error

func (e drainErrors) Error() string {
	parts := make([]string, len(e))
	for i, err := range e {
		parts[i] = err.Error()
	}
	return strings.Join(parts, "; ")
}
func (e drainErrors) Unwrap() []error { return e }

// One deadline covers preflight, cordon, eviction and termination. Pods are
// evicted concurrently so the deadline bounds the slowest pod, not the sum of
// every termination grace period. The wait seam permits deterministic
// cancellation/retry tests without wall-clock sleeps.
// kubectl v0.36.4's helper cannot be used for eviction: it lacks UID
// preconditions, can retry a replacement, sleeps on 429 and falls back to DELETE.
func (s *Store) drain(parent context.Context, name string, wait func(context.Context, time.Duration) error) error {
	ctx, cancel := context.WithTimeout(parent, drainTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	pods, err := s.c.Clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + name})
	if err != nil {
		return fmt.Errorf("drain %s preflight: %w", name, err)
	}
	var targets []*corev1.Pod
	var blockers []string
	for i := range pods.Items {
		p := &pods.Items[i]
		if p.Spec.NodeName != name || isDaemonSetPod(p) || isMirrorPod(p) {
			continue
		}
		if p.UID == "" {
			blockers = append(blockers, fmt.Sprintf("%s/%s: missing UID; refresh pod inventory before draining", p.Namespace, p.Name))
		}
		// emptyDir volumes are ephemeral by definition and their pods are
		// evicted like any other; only a pod nothing would recreate blocks.
		if metav1.GetControllerOf(p) == nil {
			blockers = append(blockers, fmt.Sprintf("%s/%s: unmanaged pod; arrange a controller or relocate it manually", p.Namespace, p.Name))
		}
		targets = append(targets, p)
	}
	if len(blockers) > 0 {
		return fmt.Errorf("drain %s blocked before cordon: %s", name, strings.Join(blockers, "; "))
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = s.c.Clientset.CoreV1().Nodes().Patch(ctx, name, types.StrategicMergePatchType, []byte(`{"spec":{"unschedulable":true}}`), metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("drain %s: cordon failed (state may be uncertain; inspect node): %w", name, err)
	}
	results := make([]error, len(targets))
	var wg sync.WaitGroup
	for i, p := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.drainPod(ctx, p, wait); err != nil {
				results[i] = fmt.Errorf("pod %s/%s (UID %s): %w", p.Namespace, p.Name, p.UID, err)
			}
		}()
	}
	wg.Wait()
	var failed drainErrors
	for _, err := range results {
		if err != nil {
			failed = append(failed, err)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("drain %s incomplete; node remains cordoned; %w", name, failed)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("drain %s incomplete; node remains cordoned: %w", name, err)
	}
	return nil
}

func (s *Store) drainPod(ctx context.Context, p *corev1.Pod, wait func(context.Context, time.Duration) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ev := &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: p.Name, Namespace: p.Namespace}, DeleteOptions: &metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &p.UID}}}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.c.Clientset.PolicyV1().Evictions(p.Namespace).Evict(ctx, ev)
		if err == nil || apierrors.IsNotFound(err) {
			break
		}
		if !apierrors.IsTooManyRequests(err) {
			return err
		}
		delay := time.Second
		if seconds, ok := apierrors.SuggestsClientDelay(err); ok && seconds > 0 {
			delay = time.Duration(seconds) * time.Second
		}
		if err := wait(ctx, delay); err != nil {
			return fmt.Errorf("waiting to retry PDB eviction: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := s.c.Clientset.CoreV1().Pods(p.Namespace).Get(ctx, p.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) || (err == nil && current.UID != p.UID) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := s.c.Clientset.CoreV1().Pods(p.Namespace).Get(ctx, p.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) || (err == nil && current.UID != p.UID) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := wait(ctx, time.Second); err != nil {
			return err
		}
	}
}

func drainWait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func isDaemonSetPod(p *corev1.Pod) bool {
	owner := metav1.GetControllerOf(p)
	return owner != nil && owner.Kind == "DaemonSet" && owner.APIVersion == "apps/v1"
}
func isMirrorPod(p *corev1.Pod) bool {
	_, ok := p.Annotations[corev1.MirrorPodAnnotationKey]
	return ok
}
