package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Contexts come out of a Go map, whose iteration order is randomised on
// every range. Without a sort the chooser reorders between frames and the
// arrow keys land somewhere different each press.
func TestContextsAreStableAndSorted(t *testing.T) {
	raw := clientcmdapi.Config{Contexts: map[string]*clientcmdapi.Context{
		"zeta-cluster":  {},
		"alpha-cluster": {},
		"prod-10":       {},
		"prod-2":        {},
	}}
	c := &Client{RestConfig: &rest.Config{}, RawConfig: raw, CurrentContext: "zeta-cluster"}

	want := []string{"alpha-cluster", "prod-2", "prod-10", "zeta-cluster"}
	first := c.Contexts()
	if len(first) != len(want) {
		t.Fatalf("got %v, want %v", first, want)
	}
	for i := range want {
		if first[i] != want[i] {
			t.Fatalf("contexts = %v, want %v (natural order)", first, want)
		}
	}

	// Repeat calls must agree — this is the part map iteration breaks.
	for i := 0; i < 50; i++ {
		got := c.Contexts()
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("call %d returned a different order: %v vs %v", i, got, first)
			}
		}
	}
}

func TestNamespacesAreSorted(t *testing.T) {
	ns := func(n string) *corev1.Namespace {
		return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: n}}
	}
	s := newTestStore(t, ns("zeta"), ns("alpha"), ns("ns-10"), ns("ns-2"))
	syncKinds(t, s, kNamespaces)

	want := []string{"alpha", "ns-2", "ns-10", "zeta"}
	got := s.Namespaces()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("namespaces = %v, want %v", got, want)
		}
	}
}
