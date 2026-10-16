package mock

import (
	"fmt"
	"strings"

	"github.com/p10node/k10s/internal/domain"
	"sigs.k8s.io/yaml"
)

func (s *Source) CustomResourceRows(ns string) ([]string, []domain.ResourceRow) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cr, defs := findResource(s.resources, "customresources"), findResource(s.resources, "crds")
	cols := []string{"NAME", "KIND", "API", "AGE"}
	all := ns == domain.AllNamespaces
	if all {
		cols = append([]string{"NAMESPACE"}, cols...)
	}
	var out []domain.ResourceRow
	if cr == nil || defs == nil {
		return cols, out
	}
	for _, r := range cr.Extra {
		if len(r.Row) < 3 {
			continue
		}
		for _, d := range defs.Rows {
			if len(d) < 5 || d[4] != r.Row[1] {
				continue
			}
			namespaced := d[3] == "Namespaced"
			if !all && r.NS != ns {
				continue
			}
			namespace := r.NS
			if !namespaced {
				namespace = ""
			}
			resource, _, _ := strings.Cut(d[0], ".")
			ref := domain.ResourceRef{Group: d[1], Version: d[2], Resource: resource, Kind: d[4], Namespaced: namespaced, Namespace: namespace, Name: r.Row[0], UID: "demo/" + d[0] + "/" + namespace + "/" + r.Row[0], ResourceVersion: "1"}
			cells := []string{ref.Name, ref.Kind, ref.Group + "/" + ref.Version, r.Row[2]}
			if all {
				cells = append([]string{r.NS}, cells...)
			}
			out = append(out, domain.ResourceRow{Cells: cells, Ref: ref})
		}
	}
	return cols, out
}

func (s *Source) validResource(ref domain.ResourceRef) bool {
	_, rows := s.CustomResourceRows(domain.AllNamespaces)
	for _, r := range rows {
		if r.Ref == ref {
			return true
		}
	}
	return false
}
func (s *Source) YAMLResource(ref domain.ResourceRef) (string, error) {
	if !s.validResource(ref) {
		return "", fmt.Errorf("demo resource no longer exists")
	}
	meta := map[string]any{"name": ref.Name, "uid": ref.UID, "resourceVersion": ref.ResourceVersion}
	if ref.Namespaced {
		meta["namespace"] = ref.Namespace
	}
	b, err := yaml.Marshal(map[string]any{"apiVersion": ref.Group + "/" + ref.Version, "kind": ref.Kind, "metadata": meta, "spec": map[string]any{}})
	return string(b), err
}
func (s *Source) DescribeResource(ref domain.ResourceRef) (string, error) { return s.YAMLResource(ref) }
func (s *Source) DeleteResource(ref domain.ResourceRef) error {
	if !s.validResource(ref) {
		return fmt.Errorf("demo resource no longer exists")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cr := findResource(s.resources, "customresources")
	ns := ref.Namespace
	if !ref.Namespaced {
		ns = "-"
	}
	for i, r := range cr.Extra {
		if r.NS == ns && len(r.Row) > 1 && r.Row[0] == ref.Name && r.Row[1] == ref.Kind {
			cr.Extra = append(cr.Extra[:i], cr.Extra[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("demo resource no longer exists")
}
func (s *Source) ApplyResource(ref domain.ResourceRef, manifest string) error {
	if !s.validResource(ref) {
		return fmt.Errorf("demo resource no longer exists")
	}
	return s.Apply("customresources", ref.Namespace, ref.Name, manifest)
}
