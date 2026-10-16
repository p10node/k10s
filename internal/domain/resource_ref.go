package domain

// ResourceRef is an exact Kubernetes identity, independent of display labels.
// Backend/session ownership belongs to the caller's operation, not this value.
type ResourceRef struct {
	Group, Version, Resource, Kind        string
	Namespaced                            bool
	Namespace, Name, UID, ResourceVersion string
}

// ResourceRow keeps identity and cells in one immutable table snapshot.
type ResourceRow struct {
	Cells []string
	Ref   ResourceRef
}

// CustomResourceSource is optional: Source remains backwards compatible.
// Callers must not fall back to guessing an ambiguous custom resource by name.
type CustomResourceSource interface {
	CustomResourceRows(ns string) (cols []string, rows []ResourceRow)
	DescribeResource(ref ResourceRef) (string, error)
	YAMLResource(ref ResourceRef) (string, error)
	DeleteResource(ref ResourceRef) error
	ApplyResource(ref ResourceRef, manifest string) error
}
