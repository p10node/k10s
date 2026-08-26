package mock

import (
	"strings"
	"testing"

	"k10s/internal/domain"
)

func TestSourceRowsNamespaceFiltering(t *testing.T) {
	s := New("")

	_, rows := s.Rows("pods", "")
	if len(rows) == 0 {
		t.Fatal("default namespace: expected some pods")
	}
	for _, r := range rows {
		// base Rows are all implicitly "default"; none of the fixture names
		// should be one of the kube-system-only pods.
		if strings.HasPrefix(r[0], "coredns") {
			t.Errorf("default namespace leaked a kube-system pod: %v", r)
		}
	}

	cols, allRows := s.Rows("pods", domain.AllNamespaces)
	if cols[0] != "NAMESPACE" {
		t.Fatalf("ns=all should prepend NAMESPACE column, got %v", cols)
	}
	if len(allRows) <= len(rows) {
		t.Fatalf("ns=all (%d rows) should show more than default namespace alone (%d rows)", len(allRows), len(rows))
	}

	_, ksRows := s.Rows("pods", "kube-system")
	found := false
	for _, r := range ksRows {
		if r[0] == "coredns-6d4b75cb6d-4x9kp" {
			found = true
		}
	}
	if !found {
		t.Errorf("kube-system namespace missing expected coredns pod: %v", ksRows)
	}

	if got, want := s.RowCount("pods", "kube-system"), len(ksRows); got != want {
		t.Errorf("RowCount(kube-system) = %d, want %d", got, want)
	}
}

func TestSourceCordonRoundTrip(t *testing.T) {
	s := New("")
	name := "ip-10-0-1-14.ap-southeast-1"

	_, rows := s.Rows("nodes", "")
	before := statusOf(t, rows, name)
	if strings.Contains(before, "SchedulingDisabled") {
		t.Fatalf("node %q starts cordoned in fixture data: %q", name, before)
	}

	if err := s.Cordon(name, true); err != nil {
		t.Fatalf("Cordon(true): %v", err)
	}
	_, rows = s.Rows("nodes", "")
	if got := statusOf(t, rows, name); !strings.Contains(got, "SchedulingDisabled") {
		t.Errorf("after Cordon(true), STATUS = %q, want SchedulingDisabled suffix", got)
	}

	if err := s.Cordon(name, false); err != nil {
		t.Fatalf("Cordon(false): %v", err)
	}
	_, rows = s.Rows("nodes", "")
	if got := statusOf(t, rows, name); strings.Contains(got, "SchedulingDisabled") {
		t.Errorf("after Cordon(false), STATUS = %q, should not contain SchedulingDisabled", got)
	}
}

func statusOf(t *testing.T, rows [][]string, name string) string {
	t.Helper()
	for _, r := range rows {
		if r[0] == name {
			return r[1]
		}
	}
	t.Fatalf("node %q not found in rows %v", name, rows)
	return ""
}

func TestSourceDelete(t *testing.T) {
	s := New("")
	name := "cache-redis" // a statefulsets fixture row

	before := s.RowCount("statefulsets", "")
	if err := s.Delete("statefulsets", "", name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	after := s.RowCount("statefulsets", "")
	if after != before-1 {
		t.Fatalf("RowCount after delete = %d, want %d", after, before-1)
	}
	_, rows := s.Rows("statefulsets", "")
	for _, r := range rows {
		if r[0] == name {
			t.Fatalf("deleted row %q still present: %v", name, rows)
		}
	}
}

func TestSourceScale(t *testing.T) {
	s := New("")
	n, err := s.Scale("deployments", "", "web-frontend", 5)
	if err != nil {
		t.Fatalf("Scale: %v", err)
	}
	if n != 5 {
		t.Fatalf("Scale returned %d, want 5", n)
	}
	_, rows := s.Rows("deployments", "")
	for _, r := range rows {
		if r[0] == "web-frontend" {
			if r[1] != "5/5" {
				t.Errorf("READY after scale = %q, want 5/5", r[1])
			}
			return
		}
	}
	t.Fatal("web-frontend row not found")
}

func TestSourceSwitchContextCycles(t *testing.T) {
	s := New("")
	first := s.ClusterInfo().Context

	next, err := s.SwitchContext("")
	if err != nil {
		t.Fatalf("SwitchContext(\"\"): %v", err)
	}
	if next.ClusterInfo().Context == first {
		t.Errorf("SwitchContext(\"\") did not advance context, still %q", first)
	}
}

func TestSourceKindsCoverEveryDomainAction(t *testing.T) {
	s := New("")
	kinds := s.Kinds()
	if len(kinds) == 0 {
		t.Fatal("Kinds() returned nothing")
	}
	seenPods := false
	for _, k := range kinds {
		if k.Key == "pods" {
			seenPods = true
			if !k.Can(domain.ADescribe) {
				t.Errorf("pods kind should allow describe")
			}
		}
	}
	if !seenPods {
		t.Fatal("Kinds() missing \"pods\"")
	}
}
