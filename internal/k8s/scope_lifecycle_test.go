package k8s

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	kt "k8s.io/client-go/testing"
	"testing"
)

func TestNamespaceScopesBoundedAndRecreated(t *testing.T) {
	s := newTestStore(t)
	cs := s.c.Clientset.(*fake.Clientset)
	opened := make(chan *watch.RaceFreeFakeWatcher, 32)
	cs.PrependWatchReactor("pods", func(kt.Action) (bool, watch.Interface, error) {
		w := watch.NewRaceFreeFake()
		opened <- w
		return true, w, nil
	})
	var watches []*watch.RaceFreeFakeWatcher
	for i := 0; i < 12; i++ {
		ns := fmt.Sprintf("ns-%d", i)
		s.ensure(kPods, ns)
		awaitHealth(t, func() bool { return s.SyncedFor(kPods, ns) })
		watches = append(watches, <-opened)
	}
	s.infMu.Lock()
	n := len(s.namespacedFactories)
	s.infMu.Unlock()
	if n > 4 {
		t.Fatalf("retained %d namespace factories, want bounded default <=4", n)
	}
	awaitHealth(t, func() bool { return watches[0].IsStopped() })
	if s.isStarted(kPods, "ns-0") {
		t.Fatal("evicted scope still drives metrics")
	}
	if !s.isStarted(kPods, "ns-11") {
		t.Fatal("current scope evicted")
	}
	_, err := cs.CoreV1().Pods("ns-0").Create(context.Background(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "fresh", Namespace: "ns-0"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s.ensure(kPods, "ns-0")
	awaitHealth(t, func() bool { return s.SyncedFor(kPods, "ns-0") })
	watches = append(watches, <-opened)
	_, rows := s.Rows(kPods, "ns-0")
	if len(rows) != 1 {
		t.Fatal("recreated scope did not relist")
	}
	s.Close()
	s.Close()
	for _, w := range watches {
		awaitHealth(t, w.IsStopped)
	}
	s.ensure(kPods, "after-close")
	if s.isStarted(kPods, "after-close") {
		t.Fatal("Close allowed new watcher")
	}
}
