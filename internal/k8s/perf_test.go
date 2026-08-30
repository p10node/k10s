package k8s

import (
	"fmt"
	"testing"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/p10node/k10s/internal/domain"
)

// bigCluster builds a fake cluster large enough that row-formatting work is
// clearly measurable against merely counting objects.
func bigCluster(t *testing.T, nPods int) *Store {
	t.Helper()
	objs := make([]runtime.Object, 0, nPods+1)
	for i := 0; i < nPods; i++ {
		ns := "default"
		if i%3 == 1 {
			ns = "kube-system"
		} else if i%3 == 2 {
			ns = "monitoring"
		}
		objs = append(objs, pod(ns, fmt.Sprintf("workload-%04d", i), "node-a", true))
	}
	objs = append(objs, node("node-a", false))
	st := newTestStore(t, objs...)
	// Perf assertions are only meaningful against a populated cache.
	syncKinds(t, st, kPods, kNodes)
	return st
}

// TestRowCountDoesNotFormatRows is the real regression guard for the input
// lag: RowCount is called once per resource kind on every repaint, so it
// must only count cached objects, never build the formatted row strings that
// Rows produces (per-pod fmt.Sprintf, metrics lookups, age formatting).
//
// It asserts on allocations rather than wall time, so it stays meaningful on
// a loaded CI box: building N rows necessarily allocates O(N) strings and
// slices, while counting them should allocate a small, N-independent amount.
func TestRowCountDoesNotFormatRows(t *testing.T) {
	const nPods = 2000
	s := bigCluster(t, nPods)

	rowsAllocs := testing.AllocsPerRun(20, func() {
		s.Rows("pods", domain.AllNamespaces)
	})
	countAllocs := testing.AllocsPerRun(20, func() {
		s.RowCount("pods", domain.AllNamespaces)
	})

	t.Logf("Rows: %.0f allocs, RowCount: %.0f allocs (%d pods)", rowsAllocs, countAllocs, nPods)

	// Rows allocates several objects per pod; RowCount should be within a
	// small constant factor of just listing them. A generous bound still
	// catches "RowCount just calls Rows".
	if countAllocs > rowsAllocs/4 {
		t.Errorf("RowCount allocated %.0f vs Rows %.0f — RowCount appears to be building formatted rows instead of counting cached objects",
			countAllocs, rowsAllocs)
	}
}

// TestRowCountStaysCheapPerKind guards the aggregate cost of one repaint:
// the sidebar calls RowCount for every kind, so the sum across all kinds
// must stay far below the cost of formatting the whole cluster.
func TestRowCountStaysCheapPerKind(t *testing.T) {
	s := bigCluster(t, 2000)
	kinds := s.Kinds()

	allKindsAllocs := testing.AllocsPerRun(20, func() {
		for _, k := range kinds {
			s.RowCount(k.Key, domain.AllNamespaces)
		}
	})
	oneRowsAllocs := testing.AllocsPerRun(20, func() {
		s.Rows("pods", domain.AllNamespaces)
	})

	t.Logf("RowCount across %d kinds: %.0f allocs; single Rows(pods): %.0f allocs",
		len(kinds), allKindsAllocs, oneRowsAllocs)

	if allKindsAllocs > oneRowsAllocs {
		t.Errorf("one repaint's RowCount calls (%.0f allocs across %d kinds) cost more than formatting every pod row (%.0f allocs) — the sidebar badges are too expensive",
			allKindsAllocs, len(kinds), oneRowsAllocs)
	}
}

// TestRowCountStartsNoInformers is the guard for the startup/switching lag:
// the sidebar asks for a badge count for every kind on every repaint, so
// RowCount must never start a watch. If it does, merely drawing the sidebar
// sets up cluster-wide LIST+WATCH for all kinds — every secret, every event
// — which is what made k10s slow to open and sluggish to navigate.
func TestRowCountStartsNoInformers(t *testing.T) {
	s := newTestStore(t, pod("default", "web-1", "node-a", true))

	for _, k := range s.Kinds() {
		// Custom Resources are the one kind whose number can arrive without
		// an informer: drawing their badge starts the background CRD sweep,
		// and it may have answered by the time the loop comes back round.
		if k.Key == "customresources" {
			continue
		}
		if got := s.RowCount(k.Key, domain.AllNamespaces); got != domain.CountUnknown {
			t.Errorf("RowCount(%q) = %d before the kind was opened, want CountUnknown", k.Key, got)
		}
	}

	for _, k := range s.Kinds() {
		// Same exception, from the other side: the CRD informer belongs to
		// that background sweep, not to the render path. Everything else
		// must still be untouched — drawing the sidebar must not open
		// cluster-wide watches.
		if k.Key == "crds" {
			continue
		}
		if s.isStarted(k.Key) {
			t.Errorf("RowCount started an informer for %q — drawing the sidebar must not open watches", k.Key)
		}
	}
}

// Drawing a sidebar badge for a kind must not set the *custom resource*
// sweep going either: that one costs a LIST per CRD, which on a cluster with
// cert-manager, Argo and prometheus-operator is dozens of requests nobody
// asked for. It starts when Custom Resources is on screen, and not before.
func TestCustomResourceSweepWaitsUntilItIsShown(t *testing.T) {
	s := newTestStore(t, pod("default", "web-1", "node-a", true))

	for _, k := range s.Kinds() {
		if k.Key == "customresources" {
			continue
		}
		s.RowCount(k.Key, domain.AllNamespaces)
	}
	s.crMu.Lock()
	running := s.crRunning
	s.crMu.Unlock()
	if running {
		t.Error("the custom-resource sweep started without that kind being drawn")
	}

	s.RowCount("customresources", domain.AllNamespaces)
	s.crMu.Lock()
	running = s.crRunning
	s.crMu.Unlock()
	if !running {
		t.Error("drawing the Custom Resources badge did not start its sweep")
	}
}

// TestOpeningOneKindWatchesOnlyThatKind pins the lazy-watch behaviour: after
// viewing pods, only the pods informer should be running.
func TestOpeningOneKindWatchesOnlyThatKind(t *testing.T) {
	s := newTestStore(t, pod("default", "web-1", "node-a", true))

	s.Rows("pods", domain.AllNamespaces)

	if !s.isStarted(kPods) {
		t.Fatal("viewing pods did not start the pods informer")
	}
	for _, k := range []string{kSecrets, kEvents, kConfigMaps, kJobs, kIngresses} {
		if s.isStarted(k) {
			t.Errorf("viewing pods also started a watch for %q", k)
		}
	}
}

// TestCustomResourceCountMakesNoLiveCalls guards the worst offender found:
// listing custom resources costs one API call per CRD, so it must happen on
// a background sweep, never on the render path.
func TestCustomResourceCountMakesNoLiveCalls(t *testing.T) {
	crd := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "widgets.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextv1.CustomResourceDefinitionNames{Kind: "Widget", Plural: "widgets", ListKind: "WidgetList"},
			Scope: apiextv1.NamespaceScoped,
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
		},
	}
	s := newTestStoreWithCRDs(t, []runtime.Object{crd})

	dynFake := s.c.Dynamic.(*dynamicfake.FakeDynamicClient)
	dynFake.ClearActions()

	for i := 0; i < 50; i++ {
		s.RowCount("customresources", domain.AllNamespaces)
	}

	for _, a := range dynFake.Actions() {
		t.Errorf("RowCount(customresources) made a live %s call to %s — CR listing must stay off the render path",
			a.GetVerb(), a.GetResource().Resource)
	}
}

func BenchmarkRowsPods(b *testing.B) {
	s := bigCluster(&testing.T{}, 2000)
	for b.Loop() {
		s.Rows("pods", domain.AllNamespaces)
	}
}

func BenchmarkRowCountAllKinds(b *testing.B) {
	s := bigCluster(&testing.T{}, 2000)
	kinds := s.Kinds()
	for b.Loop() {
		for _, k := range kinds {
			s.RowCount(k.Key, domain.AllNamespaces)
		}
	}
}
