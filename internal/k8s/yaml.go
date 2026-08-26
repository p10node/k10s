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
		gvr, namespaced, actualNS, _, ok := s.resolveCR(ns, name)
		if !ok {
			return "", fmt.Errorf("custom resource %q not found", name)
		}
		if namespaced {
			obj, err = s.c.Dynamic.Resource(gvr).Namespace(actualNS).Get(ctx, name, metav1.GetOptions{})
		} else {
			obj, err = s.c.Dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
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
		gvr, namespaced, actualNS, _, ok := s.resolveCR(ns, name)
		if !ok {
			return fmt.Errorf("custom resource %q not found", name)
		}
		ri := s.c.Dynamic.Resource(gvr)
		var riNS dynamic.ResourceInterface = ri
		if namespaced {
			riNS = ri.Namespace(actualNS)
		}
		cur, err := riNS.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		next.SetResourceVersion(cur.GetResourceVersion())
		_, err = riNS.Update(ctx, &next, metav1.UpdateOptions{})
		return err
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
	cur, err := riNS.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	next.SetResourceVersion(cur.GetResourceVersion())
	_, err = riNS.Update(ctx, &next, metav1.UpdateOptions{})
	return err
}
