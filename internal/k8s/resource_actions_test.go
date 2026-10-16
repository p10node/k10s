package k8s

import (
	"fmt"
	"strings"
	"testing"

	"github.com/p10node/k10s/internal/domain"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func identityActionFixture(t *testing.T) (*Store, *dynamicfake.FakeDynamicClient, domain.ResourceRef) {
	t.Helper()
	obj := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "example.test/v1", "kind": "Widget", "metadata": map[string]any{"name": "shared", "namespace": "default", "uid": "old", "resourceVersion": "2"}}}
	dc := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
	s := newTestStore(t)
	s.c.Dynamic = dc
	ref := domain.ResourceRef{Group: "example.test", Version: "v1", Resource: "widgets", Kind: "Widget", Namespaced: true, Namespace: "default", Name: "shared", UID: "old", ResourceVersion: "1"}
	return s, dc, ref
}
func TestResourceIdentityDeleteUIDPrecondition(t *testing.T) {
	s, dc, ref := identityActionFixture(t)
	checked := false
	dc.PrependReactor("delete", "widgets", func(a ktesting.Action) (bool, runtime.Object, error) {
		checked = true
		opts := a.(ktesting.DeleteAction).GetDeleteOptions()
		if opts.Preconditions == nil || opts.Preconditions.UID == nil || string(*opts.Preconditions.UID) != "old" {
			t.Fatal("delete missing immutable UID precondition")
		}
		return true, nil, apierrors.NewConflict(schema.GroupResource{Group: ref.Group, Resource: ref.Resource}, ref.Name, fmt.Errorf("replacement UID"))
	})
	if err := s.DeleteResource(ref); !apierrors.IsConflict(err) || !checked {
		t.Fatalf("replacement delete not refused: %v checked=%v", err, checked)
	}
	dc.ClearActions()
	ref.UID = ""
	if err := s.DeleteResource(ref); err == nil || len(dc.Actions()) != 0 {
		t.Fatal("missing identity reached API")
	}
}
func TestResourceIdentityApplyPreservesVersionAndRejectsDrift(t *testing.T) {
	s, dc, ref := identityActionFixture(t)
	manifest := "apiVersion: example.test/v1\nkind: Widget\nmetadata:\n  name: shared\n  namespace: default\n  uid: old\n  resourceVersion: '1'\n"
	updates := 0
	dc.PrependReactor("update", "widgets", func(a ktesting.Action) (bool, runtime.Object, error) {
		updates++
		obj := a.(ktesting.UpdateAction).GetObject().(*unstructured.Unstructured)
		if obj.GetResourceVersion() != "1" {
			t.Fatal("silently rebased editor buffer on new resourceVersion")
		}
		return true, nil, apierrors.NewConflict(schema.GroupResource{Group: ref.Group, Resource: ref.Resource}, ref.Name, fmt.Errorf("concurrent update"))
	})
	if err := s.ApplyResource(ref, manifest); !apierrors.IsConflict(err) || updates != 1 {
		t.Fatalf("conflict not preserved: %v updates=%d", err, updates)
	}
	for _, pair := range [][2]string{{"name: shared", "name: other"}, {"namespace: default", "namespace: other"}, {"uid: old", "uid: replacement"}, {"kind: Widget", "kind: Different"}, {"example.test/v1", "other.test/v1"}, {"  resourceVersion: '1'\n", ""}} {
		before := updates
		if err := s.ApplyResource(ref, strings.Replace(manifest, pair[0], pair[1], 1)); err == nil || updates != before {
			t.Fatalf("identity drift admitted: %v", pair)
		}
	}
	ref.UID = "replacement"
	if _, err := s.YAMLResource(ref); err == nil {
		t.Fatal("YAML admitted replacement UID")
	}
}
func TestResourceIdentityRejectsWrongScope(t *testing.T) {
	s, dc, ref := identityActionFixture(t)
	ref.Namespaced = false
	if err := s.DeleteResource(ref); err == nil || len(dc.Actions()) != 0 {
		t.Fatal("cluster scope with namespace admitted")
	}
	ref.Namespaced = true
	ref.Namespace = domain.AllNamespaces
	if err := s.DeleteResource(ref); err == nil || len(dc.Actions()) != 0 {
		t.Fatal("all-namespace write admitted")
	}

}
