package k8s

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	sigsyaml "sigs.k8s.io/yaml"
)

// cleanManagedFields strips the noisy managedFields block kubectl also
// hides by default from `kubectl get -o yaml`.
func cleanManagedFields(obj *unstructured.Unstructured) {
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
}

func (s *Store) YAML(kind, ns, name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var obj *unstructured.Unstructured
	var err error

	if kind == "customresources" {
		cr, rerr := s.resolveCR(ns, name)
		if rerr != nil {
			return "", rerr
		}
		if cr.namespaced {
			obj, err = s.c.Dynamic.Resource(cr.gvr).Namespace(cr.ns).Get(ctx, name, metav1.GetOptions{})
		} else {
			obj, err = s.c.Dynamic.Resource(cr.gvr).Get(ctx, name, metav1.GetOptions{})
		}
	} else {
		gvr, namespaced, gerr := s.gvrFor(kind)
		if gerr != nil {
			return "", gerr
		}
		if namespaced {
			obj, err = s.c.Dynamic.Resource(gvr).Namespace(effectiveNS(ns)).Get(ctx, name, metav1.GetOptions{})
		} else {
			obj, err = s.c.Dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
		}
	}
	if err != nil {
		return "", err
	}
	cleanManagedFields(obj)
	b, err := sigsyaml.Marshal(obj.Object)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Apply parses y as the new manifest for kind/ns/name and updates the live
// object (used by the 'e' edit action after $EDITOR closes).
func (s *Store) Apply(kind, ns, name, y string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var next unstructured.Unstructured
	if err := sigsyaml.Unmarshal([]byte(y), &next.Object); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}

	if kind == "customresources" {
		cr, rerr := s.resolveCR(ns, name)
		if rerr != nil {
			return rerr
		}
		ri := s.c.Dynamic.Resource(cr.gvr)
		var riNS dynamic.ResourceInterface = ri
		if cr.namespaced {
			riNS = ri.Namespace(cr.ns)
		}
		return updateEdited(ctx, riNS, name, &next)
	}

	gvr, namespaced, err := s.gvrFor(kind)
	if err != nil {
		return err
	}
	ri := s.c.Dynamic.Resource(gvr)
	var riNS dynamic.ResourceInterface = ri
	if namespaced {
		riNS = ri.Namespace(effectiveNS(ns))
	}
	return updateEdited(ctx, riNS, name, &next)
}

// The GET verifies identity only. Its newer resourceVersion must never replace
// the version from the user's edit; the API server arbitrates stale writes.
func updateEdited(ctx context.Context, ri dynamic.ResourceInterface, name string, next *unstructured.Unstructured) error {
	cur, err := ri.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if next.GetResourceVersion() == "" {
		return fmt.Errorf("edited manifest must retain metadata.resourceVersion; reload before editing")
	}
	if next.GetName() != name || next.GetNamespace() != cur.GetNamespace() ||
		next.GetUID() != cur.GetUID() || next.GroupVersionKind() != cur.GroupVersionKind() {
		return fmt.Errorf("edited manifest identity differs from target; reload before editing")
	}
	_, err = ri.Update(ctx, next, metav1.UpdateOptions{})
	return err
}
