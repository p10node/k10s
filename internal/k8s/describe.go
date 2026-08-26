package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kubectl/pkg/describe"
)

// kindToGK maps our builtin kind keys to the GroupKind kubectl's built-in
// describers are registered under.
var kindToGK = map[string]schema.GroupKind{
	"pods":         {Kind: "Pod"},
	"deployments":  {Group: "apps", Kind: "Deployment"},
	"statefulsets": {Group: "apps", Kind: "StatefulSet"},
	"daemonsets":   {Group: "apps", Kind: "DaemonSet"},
	"jobs":         {Group: "batch", Kind: "Job"},
	"cronjobs":     {Group: "batch", Kind: "CronJob"},
	"services":     {Kind: "Service"},
	"ingresses":    {Group: "networking.k8s.io", Kind: "Ingress"},
	"configmaps":   {Kind: "ConfigMap"},
	"secrets":      {Kind: "Secret"},
	"pvcs":         {Kind: "PersistentVolumeClaim"},
	"nodes":        {Kind: "Node"},
	"namespaces":   {Kind: "Namespace"},
	"events":       {Kind: "Event"},
}

func effectiveNS(ns string) string {
	if ns == "" {
		return "default"
	}
	return ns
}

// Describe renders `kubectl describe`-equivalent output: kubectl's built-in
// describers for the kinds it knows, and a generic unstructured describer
// (name/labels/annotations/spec/status/events) for everything else,
// including CRD instances.
func (s *Store) Describe(kind, ns, name string) (string, error) {
	if kind == "customresources" {
		gvr, namespaced, actualNS, crdKind, ok := s.resolveCR(ns, name)
		if !ok {
			return "", fmt.Errorf("custom resource %q not found", name)
		}
		return s.genericDescribe(gvr, namespaced, actualNS, name, crdKind)
	}

	if gk, ok := kindToGK[kind]; ok {
		if d, ok := describe.DescriberFor(gk, s.c.RestConfig); ok {
			ens := ""
			if findKind(kind) != nil && findKind(kind).Namespaced {
				ens = effectiveNS(ns)
			}
			return d.Describe(ens, name, describe.DescriberSettings{ShowEvents: true})
		}
	}

	if kind == "crds" {
		return s.genericDescribe(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}, false, "", name, "CustomResourceDefinition")
	}

	gvr, namespaced, err := s.gvrFor(kind)
	if err != nil {
		return "", err
	}
	return s.genericDescribe(gvr, namespaced, effectiveNS(ns), name, kind)
}

func (s *Store) genericDescribe(gvr schema.GroupVersionResource, namespaced bool, ns, name, label string) (string, error) {
	gvk, err := s.c.Mapper.KindFor(gvr)
	if err != nil {
		return "", fmt.Errorf("resolve kind for %s: %w", label, err)
	}
	mapping, err := s.c.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return "", fmt.Errorf("resolve mapping for %s: %w", label, err)
	}
	d, ok := describe.GenericDescriberFor(mapping, s.c.RestConfig)
	if !ok {
		return "", fmt.Errorf("no describer available for %s", label)
	}
	nsArg := ""
	if namespaced {
		nsArg = ns
	}
	return d.Describe(nsArg, name, describe.DescriberSettings{ShowEvents: true})
}
