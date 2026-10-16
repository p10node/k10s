package k8s

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
	"testing"
)

func TestCustomResourceNamespaceRBAC(t *testing.T) {
	s, _, gvr, _ := editFixture(t, true)
	s.crMu.Lock()
	s.crRunning = true
	s.crMu.Unlock() // drive sweeps synchronously
	dyn := s.c.Dynamic.(*dynamicfake.FakeDynamicClient)
	calls := []string{}
	dyn.PrependReactor("list", gvr.Resource, func(a clienttesting.Action) (bool, runtime.Object, error) {
		calls = append(calls, a.GetNamespace())
		if a.GetNamespace() == "" {
			return true, nil, apierrors.NewForbidden(gvr.GroupResource(), "", nil)
		}
		return false, nil, nil
	})
	s.Rows("customresources", "a")
	s.refreshCRs()
	_, rows := s.Rows("customresources", "a")
	if len(rows) != 1 {
		t.Fatalf("namespace a rows=%v; requests=%v", rows, calls)
	}
	if err := s.LoadErrorFor("customresources", "a"); err != nil {
		t.Fatal(err)
	}
	s.Rows("customresources", "b")
	if s.SyncedFor("customresources", "b") {
		t.Fatal("namespace b inherited a's synchronization")
	}
	s.refreshCRs()
	if !s.SyncedFor("customresources", "b") {
		t.Fatal("successful empty b is not synced")
	}
	_, rows = s.Rows("customresources", "b")
	if len(rows) != 0 {
		t.Fatalf("b rows=%v", rows)
	}
	if len(calls) != 2 || calls[0] != "a" || calls[1] != "b" {
		t.Fatalf("requests=%v", calls)
	}
}
func TestCustomResourceStaleSweepCannotPublish(t *testing.T) {
	s, _, gvr, _ := editFixture(t, true)
	s.crMu.Lock()
	s.crRunning = true
	s.crMu.Unlock()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	dyn := s.c.Dynamic.(*dynamicfake.FakeDynamicClient)
	dyn.PrependReactor("list", gvr.Resource, func(a clienttesting.Action) (bool, runtime.Object, error) {
		close(entered)
		<-release
		return false, nil, nil
	})
	s.Rows("customresources", "a")
	go func() { defer close(done); s.refreshCRs() }()
	<-entered
	s.Rows("customresources", "b")
	s.Rows("customresources", "a") // same scope again is still a new generation
	close(release)
	<-done
	if s.SyncedFor("customresources", "a") {
		t.Fatal("stale sweep marked new generation loaded")
	}
	_, rows := s.Rows("customresources", "a")
	if len(rows) != 0 {
		t.Fatalf("stale rows published: %v", rows)
	}
}
