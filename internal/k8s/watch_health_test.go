package k8s

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	kt "k8s.io/client-go/testing"
)

// The disconnect case must still degrade promptly under test.
func init() { watchReconnectGrace = 10 * time.Millisecond }

func awaitHealth(t *testing.T, f func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !f() {
		select {
		case <-ctx.Done():
			t.Fatal("watch state did not converge")
		case <-ticker.C:
		}
	}
}

func TestWatchFailureSurvivesSyncedGetter(t *testing.T) {
	for _, failure := range []string{"forbidden", "disconnect"} {
		t.Run(failure, func(t *testing.T) {
			s := newTestStore(t, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "retained", Namespace: "a"}})
			cs := s.c.Clientset.(*fake.Clientset)
			first := watch.NewRaceFreeFake()
			second := watch.NewRaceFreeFake()
			reconnect := make(chan struct{})
			release := make(chan struct{})
			defer close(release)
			calls := 0
			cs.PrependWatchReactor("pods", func(a kt.Action) (bool, watch.Interface, error) {
				calls++
				if calls == 1 {
					return true, first, nil
				}
				if calls == 2 {
					close(reconnect)
					<-release
				}
				return true, second, nil
			})
			s.ensure(kPods, "a")
			awaitHealth(t, func() bool { return s.SyncedFor(kPods, "a") })
			if failure == "forbidden" {
				first.Error(&metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonForbidden, Code: 403, Message: "watch revoked"})
			} else {
				first.Stop()
			}
			awaitHealth(t, func() bool { return s.LoadErrorFor(kPods, "a") != nil })
			if !s.SyncedFor(kPods, "a") {
				t.Fatal("historical initial sync lost")
			}
			if s.LoadErrorFor(kPods, "a") == nil {
				t.Fatal("sync getter erased degraded health")
			}
			_, rows := s.Rows(kPods, "a")
			if len(rows) != 1 {
				t.Fatal("lost last-known rows")
			}
			select {
			case <-reconnect:
			case <-time.After(5 * time.Second):
				t.Fatal("no reconnect")
			}
			release <- struct{}{}
			awaitHealth(t, func() bool { return s.LoadErrorFor(kPods, "a") == nil })
		})
	}
}

// The reflector's own Stop closes the upstream too; that is not a disconnect
// and must not arm the degrade timer, whichever closed channel select picks.
func TestHealthWatchStopIsNotADisconnect(t *testing.T) {
	h := &watchHealth{}
	for i := 0; i < 50; i++ {
		w := &healthWatch{upstream: watch.NewRaceFreeFake(), events: make(chan watch.Event), done: make(chan struct{})}
		go w.run(context.Background(), h)
		w.Stop()
		for range w.events {
		}
	}
	time.Sleep(5 * watchReconnectGrace)
	if err := h.get(); err != nil {
		t.Fatalf("deliberate stop degraded health: %v", err)
	}
}
