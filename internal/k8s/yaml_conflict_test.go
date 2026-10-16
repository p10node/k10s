package k8s

import (
	"context"
	"fmt"
	"testing"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
	sigsyaml "sigs.k8s.io/yaml"
)

func editFixture(t *testing.T, custom bool) (*Store, string, schema.GroupVersionResource, *unstructured.Unstructured) {
	t.Helper()
	kind, api, resource, view := "ConfigMap", "v1", "configmaps", "configmaps"
	var crds []runtime.Object
	if custom {
		kind, api, resource, view = "Widget", "example.test/v1", "widgets", "customresources"
		crds = []runtime.Object{&apiextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "widgets.example.test"}, Spec: apiextv1.CustomResourceDefinitionSpec{Group: "example.test", Scope: apiextv1.NamespaceScoped, Names: apiextv1.CustomResourceDefinitionNames{Plural: resource, Kind: kind}, Versions: []apiextv1.CustomResourceDefinitionVersion{{Name: "v1", Served: true, Storage: true}}}}}
	}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": api, "kind": kind, "metadata": map[string]interface{}{"name": "item", "namespace": "a", "uid": "uid-a", "resourceVersion": "1"}, "data": map[string]interface{}{"value": "original"}}}
	s := newTestStoreWithCRDs(t, crds, obj)
	gv, _ := schema.ParseGroupVersion(api)
	return s, view, gv.WithResource(resource), obj
}

func TestApplyPreservesConflictVersion(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(fmt.Sprint(custom), func(t *testing.T) {
			s, kind, gvr, obj := editFixture(t, custom)
			original, err := s.YAML(kind, "a", "item")
			if err != nil {
				t.Fatal(err)
			}
			dyn := s.c.Dynamic.(*dynamicfake.FakeDynamicClient)
			latest := obj.DeepCopy()
			latest.SetResourceVersion("2")
			latest.Object["data"] = map[string]interface{}{"value": "concurrent"}
			if err := dyn.Tracker().Update(gvr, latest, "a"); err != nil {
				t.Fatal(err)
			}
			puts := 0
			dyn.PrependReactor("update", gvr.Resource, func(a clienttesting.Action) (bool, runtime.Object, error) {
				puts++
				next := a.(clienttesting.UpdateAction).GetObject().(*unstructured.Unstructured)
				if next.GetResourceVersion() != "2" {
					return true, nil, apierrors.NewConflict(gvr.GroupResource(), "item", fmt.Errorf("stale version"))
				}
				return false, nil, nil
			})
			if err := s.Apply(kind, "a", "item", original); !apierrors.IsConflict(err) {
				t.Fatalf("expected conflict, got %v", err)
			}
			if puts != 1 {
				t.Fatalf("PUT count %d", puts)
			}
			got, _ := dyn.Resource(gvr).Namespace("a").Get(context.Background(), "item", metav1.GetOptions{})
			if got.Object["data"].(map[string]interface{})["value"] != "concurrent" {
				t.Fatal("concurrent write lost")
			}
			current, _ := sigsyaml.Marshal(latest.Object)
			if err := s.Apply(kind, "a", "item", string(current)); err != nil {
				t.Fatalf("same-version control: %v", err)
			}
		})
	}
}
func TestApplyRejectsIdentityDrift(t *testing.T) {
	for _, custom := range []bool{false, true} {
		for _, field := range []string{"name", "namespace", "uid", "kind", "apiVersion", "missingVersion"} {
			t.Run(fmt.Sprintf("%v/%s", custom, field), func(t *testing.T) {
				s, kind, gvr, obj := editFixture(t, custom)
				switch field {
				case "name":
					obj.SetName("sibling")
				case "namespace":
					obj.SetNamespace("b")
				case "uid":
					obj.SetUID("replacement")
				case "kind":
					obj.SetKind("Other")
				case "apiVersion":
					obj.SetAPIVersion("other.test/v1")
				case "missingVersion":
					obj.SetResourceVersion("")
				}
				dyn := s.c.Dynamic.(*dynamicfake.FakeDynamicClient)
				puts := 0
				dyn.PrependReactor("update", gvr.Resource, func(a clienttesting.Action) (bool, runtime.Object, error) {
					puts++
					return true, a.(clienttesting.UpdateAction).GetObject(), nil
				})
				manifest, _ := sigsyaml.Marshal(obj.Object)
				if err := s.Apply(kind, "a", "item", string(manifest)); err == nil {
					t.Error("identity drift accepted")
				}
				if puts != 0 {
					t.Fatalf("sent %d unsafe PUTs", puts)
				}
			})
		}
	}
}
