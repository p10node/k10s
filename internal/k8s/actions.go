package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (s *Store) Delete(kind, ns, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if kind == "customresources" {
		gvr, namespaced, actualNS, _, ok := s.resolveCR(ns, name)
		if !ok {
			return fmt.Errorf("custom resource %q not found", name)
		}
		if namespaced {
			return s.c.Dynamic.Resource(gvr).Namespace(actualNS).Delete(ctx, name, metav1.DeleteOptions{})
		}
		return s.c.Dynamic.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
	}

	gvr, namespaced, err := s.gvrFor(kind)
	if err != nil {
		return err
	}
	if namespaced {
		return s.c.Dynamic.Resource(gvr).Namespace(effectiveNS(ns)).Delete(ctx, name, metav1.DeleteOptions{})
	}
	return s.c.Dynamic.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
}

// Restart triggers a rollout restart by patching the pod template's
// restartedAt annotation, exactly like `kubectl rollout restart`.
func (s *Store) Restart(kind, ns, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ens := effectiveNS(ns)
	patch := fmt.Appendf(nil,
		`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`,
		time.Now().UTC().Format(time.RFC3339))

	switch kind {
	case "deployments":
		_, err := s.c.Clientset.AppsV1().Deployments(ens).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		return err
	case "statefulsets":
		_, err := s.c.Clientset.AppsV1().StatefulSets(ens).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		return err
	case "daemonsets":
		_, err := s.c.Clientset.AppsV1().DaemonSets(ens).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		return err
	}
	return fmt.Errorf("rollout restart is not supported for %s", kind)
}

// Scale patches spec.replicas directly rather than going through the scale
// subresource (GetScale/UpdateScale): same effect on a real cluster, one
// request instead of two, and it doesn't depend on the scale subresource
// being wired up (fake clientsets used in tests don't implement it).
func (s *Store) Scale(kind, ns, name string, replicas int) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ens := effectiveNS(ns)
	patch := fmt.Appendf(nil, `{"spec":{"replicas":%d}}`, replicas)

	switch kind {
	case "deployments":
		_, err := s.c.Clientset.AppsV1().Deployments(ens).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		return replicas, err
	case "statefulsets":
		_, err := s.c.Clientset.AppsV1().StatefulSets(ens).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		return replicas, err
	case "replicasets":
		_, err := s.c.Clientset.AppsV1().ReplicaSets(ens).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		return replicas, err
	}
	return 0, fmt.Errorf("scale is not supported for %s", kind)
}

func (s *Store) Cordon(name string, disabled bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	patch := fmt.Appendf(nil, `{"spec":{"unschedulable":%v}}`, disabled)
	_, err := s.c.Clientset.CoreV1().Nodes().Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	return err
}

// Drain cordons the node then evicts every non-DaemonSet, non-mirror pod
// running on it, like `kubectl drain --ignore-daemonsets`.
func (s *Store) Drain(name string) error {
	if err := s.Cordon(name, true); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pods, err := s.c.Clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + name,
	})
	if err != nil {
		return fmt.Errorf("list pods on %s: %w", name, err)
	}

	var failed []string
	for _, p := range pods.Items {
		if isDaemonSetPod(&p) || isMirrorPod(&p) {
			continue
		}
		ev := &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: p.Name, Namespace: p.Namespace}}
		if err := s.c.Clientset.PolicyV1().Evictions(p.Namespace).Evict(ctx, ev); err != nil {
			failed = append(failed, fmt.Sprintf("%s/%s: %v", p.Namespace, p.Name, err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("drain %s: %d pod(s) failed to evict: %s", name, len(failed), strings.Join(failed, "; "))
	}
	return nil
}

func isDaemonSetPod(p *corev1.Pod) bool {
	for _, o := range p.OwnerReferences {
		if o.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

func isMirrorPod(p *corev1.Pod) bool {
	_, ok := p.Annotations[corev1.MirrorPodAnnotationKey]
	return ok
}
