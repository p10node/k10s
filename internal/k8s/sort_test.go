package k8s

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/p10node/k10s/internal/domain"
)

func TestSortRowsByName(t *testing.T) {
	rows := [][]string{{"web"}, {"api"}, {"db-10"}, {"db-2"}}
	sortRows(rows, 0)
	want := []string{"api", "db-2", "db-10", "web"}
	for i, w := range want {
		if rows[i][0] != w {
			t.Fatalf("sorted = %v, want order %v", rows, want)
		}
	}
}

func TestRowsAreSortedAlphabetically(t *testing.T) {
	// Deliberately inserted out of order; the informer cache returns them in
	// arbitrary order anyway, which is exactly why Rows must sort.
	s := newTestStore(t,
		pod("default", "zebra", "n1", true),
		pod("default", "alpha", "n1", true),
		pod("default", "mid-10", "n1", true),
		pod("default", "mid-2", "n1", true),
	)
	syncKinds(t, s, kPods)

	_, rows := s.Rows("pods", "default")
	got := make([]string, 0, len(rows))
	for _, r := range rows {
		got = append(got, r[0])
	}
	want := []string{"alpha", "mid-2", "mid-10", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows not sorted: got %v, want %v", got, want)
		}
	}
}

// TestAllNamespacesGroupsByNamespace pins the /ns all ordering: namespace
// first, then name, so objects from one namespace stay together.
func TestAllNamespacesGroupsByNamespace(t *testing.T) {
	s := newTestStore(t,
		pod("zeta", "b", "n1", true),
		pod("alpha", "z", "n1", true),
		pod("alpha", "a", "n1", true),
	)
	syncKinds(t, s, kPods)

	_, rows := s.Rows("pods", domain.AllNamespaces)
	var got [][2]string
	for _, r := range rows {
		got = append(got, [2]string{r[0], r[1]})
	}
	want := [][2]string{{"alpha", "a"}, {"alpha", "z"}, {"zeta", "b"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestEventsStayNewestFirst guards the exception to alphabetical ordering:
// events are time-ordered, and sorting them by their first column (TYPE)
// would bury the newest ones behind every "Normal" event.
func TestEventsStayNewestFirst(t *testing.T) {
	mkEvent := func(name, typ, reason string, ago time.Duration) *corev1.Event {
		ts := metav1.NewTime(time.Now().Add(-ago))
		return &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: name, Namespace: "default", CreationTimestamp: ts},
			Type:           typ,
			Reason:         reason,
			LastTimestamp:  ts,
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: name},
		}
	}
	// "Warning" sorts after "Normal" alphabetically, so if the newest event
	// is a Warning and still comes first, ordering is by time not by name.
	s := newTestStore(t,
		mkEvent("old-one", "Normal", "Pulled", 2*time.Hour),
		mkEvent("newest", "Warning", "BackOff", 1*time.Minute),
		mkEvent("middle", "Normal", "Created", 30*time.Minute),
	)
	syncKinds(t, s, kEvents)

	cols, rows := s.Rows("events", "default")
	if len(rows) != 3 {
		t.Fatalf("got %d event rows, want 3: %v", len(rows), rows)
	}
	objIdx := -1
	for i, c := range cols {
		if c == "OBJECT" {
			objIdx = i
		}
	}
	if objIdx < 0 {
		t.Fatalf("no OBJECT column in %v", cols)
	}
	want := []string{"pod/newest", "pod/middle", "pod/old-one"}
	for i, w := range want {
		if rows[i][objIdx] != w {
			t.Fatalf("events not newest-first: got %v at %d, want %v (rows=%v)",
				rows[i][objIdx], i, w, rows)
		}
	}
}
