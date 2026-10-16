package k8s

import (
	"context"
	"fmt"
	"time"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// crTarget is a "Custom Resources" row resolved by name to one CRD.
type crTarget struct {
	gvr        schema.GroupVersionResource
	namespaced bool
	ns         string // "" for a cluster-scoped instance
	kind       string
}

// resolveCR finds which discovered CRD a "Custom Resources" row's name
// belongs to under namespace ns (ns == "-" for a cluster-scoped instance),
// so describe/YAML/delete can talk to the right GVR. It tries a live Get
// per CRD — fine for the occasional single-item lookups this backs.
//
// The error tells a missing object apart from an ambiguous name and from a
// lookup that failed. A non-NotFound error on any CRD leaves the name's
// uniqueness unknown and fails closed, except Forbidden: a CRD this identity
// cannot read is not one it could act on, so it cannot make the selection
// ambiguous.
func (s *Store) resolveCR(ns, name string) (crTarget, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	crdList, err := s.apiext.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return crTarget{}, fmt.Errorf("list custom resource definitions: %w", err)
	}

	var hit crTarget
	found := false
	for i := range crdList.Items {
		crd := &crdList.Items[i]
		ver := servedStorageVersion(crd)
		if ver == "" {
			continue
		}
		g := schema.GroupVersionResource{Group: crd.Spec.Group, Version: ver, Resource: crd.Spec.Names.Plural}
		scoped := crd.Spec.Scope == apiextv1.NamespaceScoped
		if scoped && (ns == "" || ns == "-") {
			continue
		}
		ri := s.c.Dynamic.Resource(g)
		var getErr error
		if scoped {
			_, getErr = ri.Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		} else {
			_, getErr = ri.Get(ctx, name, metav1.GetOptions{})
		}
		switch {
		case getErr == nil:
			if found {
				return crTarget{}, fmt.Errorf("custom resource %q is ambiguous: both %s and %s have one; select it from the Custom Resources view", name, hit.gvr.GroupResource(), g.GroupResource())
			}
			hit, found = crTarget{gvr: g, namespaced: scoped, kind: crd.Spec.Names.Kind}, true
			if scoped {
				hit.ns = ns
			}
		case apierrors.IsNotFound(getErr), apierrors.IsForbidden(getErr):
		default:
			return crTarget{}, fmt.Errorf("look up custom resource %q in %s: %w", name, g.GroupResource(), getErr)
		}
	}
	if !found {
		return crTarget{}, fmt.Errorf("custom resource %q not found", name)
	}
	return hit, nil
}
