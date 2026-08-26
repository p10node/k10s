package k8s

import (
	"context"
	"time"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// resolveCR finds which discovered CRD a "Custom Resources" row's name
// belongs to under namespace ns (ns == "-" for a cluster-scoped instance),
// so describe/YAML/delete can talk to the right GVR. It tries a live Get
// per CRD — fine for the occasional single-item lookups this backs.
func (s *Store) resolveCR(ns, name string) (gvr schema.GroupVersionResource, namespaced bool, actualNS string, crdKind string, found bool) {
	crds, _ := s.crdLister().List(labels.Everything())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, crd := range crds {
		ver := servedStorageVersion(crd)
		if ver == "" {
			continue
		}
		g := schema.GroupVersionResource{Group: crd.Spec.Group, Version: ver, Resource: crd.Spec.Names.Plural}
		if crd.Spec.Scope == apiextv1.NamespaceScoped {
			if ns == "" || ns == "-" {
				continue
			}
			if _, err := s.c.Dynamic.Resource(g).Namespace(ns).Get(ctx, name, metav1.GetOptions{}); err == nil {
				return g, true, ns, crd.Spec.Names.Kind, true
			}
		} else {
			if _, err := s.c.Dynamic.Resource(g).Get(ctx, name, metav1.GetOptions{}); err == nil {
				return g, false, "", crd.Spec.Names.Kind, true
			}
		}
	}
	return schema.GroupVersionResource{}, false, "", "", false
}
