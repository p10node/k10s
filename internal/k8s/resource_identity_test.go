package k8s

import (
	"context"
	"github.com/p10node/k10s/internal/domain"
	"strings"
	"testing"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func identityFixture(t *testing.T) (*Store, []schema.GroupVersionResource) {
	t.Helper()
	var crds []runtime.Object
	var objs []runtime.Object
	var gvrs []schema.GroupVersionResource
	for _, group := range []string{"alpha.test", "beta.test"} {
		crds = append(crds, &apiextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "widgets." + group}, Spec: apiextv1.CustomResourceDefinitionSpec{Group: group, Names: apiextv1.CustomResourceDefinitionNames{Plural: "widgets", Kind: "Widget", ListKind: "WidgetList"}, Scope: apiextv1.NamespaceScoped, Versions: []apiextv1.CustomResourceDefinitionVersion{{Name: "v1", Served: true, Storage: true}}}})
		gvrs = append(gvrs, schema.GroupVersionResource{Group: group, Version: "v1", Resource: "widgets"})
		objs = append(objs, &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": group + "/v1", "kind": "Widget", "metadata": map[string]interface{}{"name": "shared", "namespace": "default", "uid": group, "resourceVersion": "1"}, "spec": map[string]interface{}{"owner": group}}})
	}
	return newTestStoreWithCRDs(t, crds, objs...), gvrs
}

func TestCustomResourceIdentitySnapshotAndActions(t *testing.T) {
	s, gvrs := identityFixture(t)
	cap, ok := any(s).(domain.CustomResourceSource)
	if !ok {
		t.Fatal("backend does not expose identity-carrying custom resource operations")
	}
	// Select the scope before this synchronous fixture sweep; no CR worker
	// has been started yet.
	s.crNS = "default"
	s.refreshCRs()
	_, rows := cap.CustomResourceRows("default")
	if len(rows) != 2 {
		t.Fatalf("want two rows, got %d", len(rows))
	}
	for _, row := range rows {
		if row.Ref.Name != "shared" || row.Ref.UID != row.Ref.Group {
			t.Fatalf("bad ref: %+v", row.Ref)
		}
		y, err := cap.YAMLResource(row.Ref)
		if err != nil || !strings.Contains(y, "owner: "+row.Ref.Group) {
			t.Fatalf("wrong object: %s %v", y, err)
		}
	}
	chosen := rows[1].Ref
	if err := cap.DeleteResource(chosen); err != nil {
		t.Fatal(err)
	}
	for _, g := range gvrs {
		_, err := s.c.Dynamic.Resource(g).Namespace("default").Get(context.Background(), "shared", metav1.GetOptions{})
		if (err != nil) != (g.Group == chosen.Group) {
			t.Fatalf("wrong delete target %s: %v", g, err)
		}
	}
}

func TestCustomResourceLegacyDeleteRefusesEvenOneMatch(t *testing.T) {
	s, gvrs := identityFixture(t)
	if err := s.c.Dynamic.Resource(gvrs[1]).Namespace("default").Delete(context.Background(), "shared", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("customresources", "default", "shared"); err == nil {
		t.Fatal("legacy delete guessed identity from the one remaining type")
	}
	if _, err := s.c.Dynamic.Resource(gvrs[0]).Namespace("default").Get(context.Background(), "shared", metav1.GetOptions{}); err != nil {
		t.Fatalf("wrong resource removed: %v", err)
	}
}

func TestCustomResourceAmbiguousLegacyDeleteFailsClosed(t *testing.T) {
	s, gvrs := identityFixture(t)
	if err := s.Delete("customresources", "default", "shared"); err == nil {
		t.Fatal("ambiguous custom-resource Delete must refuse, not pick the first GVR")
	}
	for _, gvr := range gvrs {
		if _, err := s.c.Dynamic.Resource(gvr).Namespace("default").Get(context.Background(), "shared", metav1.GetOptions{}); err != nil {
			t.Fatalf("sibling %s changed: %v", gvr, err)
		}
	}
}

// Legacy by-name lookups fail closed on an ambiguous name, and say so rather
// than claiming the object does not exist.
func TestLegacyCustomResourceLookupReportsAmbiguity(t *testing.T) {
	s, _ := identityFixture(t)
	if _, err := s.YAML("customresources", "default", "shared"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous name resolved or misreported: %v", err)
	}
	if _, err := s.Describe("customresources", "default", "absent"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing object misreported: %v", err)
	}
}
