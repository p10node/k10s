package k8s

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (s *Store) Delete(kind, ns, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if kind == "customresources" {
		return fmt.Errorf("custom-resource delete requires exact GVR and UID; use DeleteResource")
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
