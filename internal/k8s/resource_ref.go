package k8s

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/p10node/k10s/internal/domain"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	sigsyaml "sigs.k8s.io/yaml"
)

var _ domain.CustomResourceSource = (*Store)(nil)

// CustomResourceRows returns cells and references from the same cache version.
func (s *Store) CustomResourceRows(ns string) ([]string, []domain.ResourceRow) {
	s.noteInterest("customresources", ns) // also schedules the CR sweep
	s.crMu.Lock()
	defer s.crMu.Unlock()
	cols := []string{"NAME", "KIND", "API", "AGE"}
	all := ns == domain.AllNamespaces
	if all {
		cols = append([]string{"NAMESPACE"}, cols...)
	}
	var rows []domain.ResourceRow
	for i, r := range s.crCache {
		if i >= len(s.crRefs) {
			continue
		}
		ref := s.crRefs[i]
		if !all && r.ns != effectiveNS(ns) {
			continue
		}
		cells := slices.Clone(r.row)
		if len(cells) == 3 {
			cells = []string{cells[0], cells[1], ref.Group + "/" + ref.Version, cells[2]}
		}
		if all {
			cells = append([]string{r.ns}, cells...)
		}
		rows = append(rows, domain.ResourceRow{Cells: cells, Ref: ref})
	}
	// Same order as applyNamespaceOpt: under :ns all, group by namespace
	// then name so related objects stay together instead of interleaving.
	slices.SortStableFunc(rows, func(a, b domain.ResourceRow) int {
		for _, k := range [][2]string{{a.Ref.Namespace, b.Ref.Namespace}, {a.Ref.Name, b.Ref.Name}} {
			if k[0] != k[1] {
				if domain.NaturalLess(k[0], k[1]) {
					return -1
				}
				return 1
			}
		}
		return cmp.Compare(a.Ref.Group+"/"+a.Ref.Resource, b.Ref.Group+"/"+b.Ref.Resource)
	})
	return cols, rows
}

func (s *Store) resourceClient(ref domain.ResourceRef) (dynamic.ResourceInterface, error) {
	if ref.Name == "" || ref.Version == "" || ref.Resource == "" || ref.Kind == "" {
		return nil, fmt.Errorf("incomplete resource identity")
	}
	if ref.Namespaced && (ref.Namespace == "" || ref.Namespace == domain.AllNamespaces || ref.Namespace == "-") {
		return nil, fmt.Errorf("resource identity requires an exact namespace")
	}
	if !ref.Namespaced && ref.Namespace != "" {
		return nil, fmt.Errorf("cluster resource identity cannot have a namespace")
	}
	ri := s.c.Dynamic.Resource(schema.GroupVersionResource{Group: ref.Group, Version: ref.Version, Resource: ref.Resource})
	if ref.Namespaced {
		return ri.Namespace(ref.Namespace), nil
	}
	return ri, nil
}

func (s *Store) resourceObject(ctx context.Context, ref domain.ResourceRef) (*unstructured.Unstructured, error) {
	ri, err := s.resourceClient(ref)
	if err != nil {
		return nil, err
	}
	obj, err := ri.Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if ref.UID != "" && string(obj.GetUID()) != ref.UID {
		return nil, fmt.Errorf("resource was replaced; select it again")
	}
	return obj, nil
}

func (s *Store) YAMLResource(ref domain.ResourceRef) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	obj, err := s.resourceObject(ctx, ref)
	if err != nil {
		return "", err
	}
	cleanManagedFields(obj)
	b, err := sigsyaml.Marshal(obj.Object)
	return string(b), err
}

func (s *Store) DescribeResource(ref domain.ResourceRef) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := s.resourceObject(ctx, ref); err != nil {
		return "", err
	}
	return s.genericDescribe(schema.GroupVersionResource{Group: ref.Group, Version: ref.Version, Resource: ref.Resource}, ref.Namespaced, ref.Namespace, ref.Name, ref.Kind)
}

func (s *Store) DeleteResource(ref domain.ResourceRef) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ri, err := s.resourceClient(ref)
	if err != nil {
		return err
	}
	if ref.UID == "" {
		return fmt.Errorf("missing resource UID; refresh before deleting")
	}
	uid := types.UID(ref.UID)
	return ri.Delete(ctx, ref.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
}

func (s *Store) ApplyResource(ref domain.ResourceRef, manifest string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ri, err := s.resourceClient(ref)
	if err != nil {
		return err
	}
	var next unstructured.Unstructured
	if err := sigsyaml.Unmarshal([]byte(manifest), &next.Object); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}
	gv := schema.GroupVersion{Group: ref.Group, Version: ref.Version}.String()
	if next.GetName() != ref.Name || next.GetNamespace() != ref.Namespace || next.GetAPIVersion() != gv || next.GetKind() != ref.Kind || ref.UID == "" || string(next.GetUID()) != ref.UID {
		return fmt.Errorf("edited manifest changed resource identity")
	}
	if next.GetResourceVersion() == "" {
		return fmt.Errorf("edited manifest requires original resourceVersion")
	}
	_, err = ri.Update(ctx, &next, metav1.UpdateOptions{})
	return err
}
