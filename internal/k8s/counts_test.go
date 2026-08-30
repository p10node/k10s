package k8s

import (
	"sync"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	clienttesting "k8s.io/client-go/testing"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	apiextfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"

	"k8s.io/client-go/rest"

	"github.com/p10node/k10s/internal/domain"
)

// listSpy counts the badge-count requests the sweeper actually sends, which
// is the whole point of the exercise: every one of them is a round trip to
// somebody's API server.
type listSpy struct {
	mu    sync.Mutex
	calls map[string]int
	fail  map[string]error
}

func (l *listSpy) react(a clienttesting.Action) (bool, runtime.Object, error) {
	res := a.GetResource().Resource

	l.mu.Lock()
	l.calls[res]++
	err := l.fail[res]
	l.mu.Unlock()

	if err != nil {
		return true, nil, err
	}
	return false, nil, nil // fall through to the fake's own tracker
}

func (l *listSpy) count(res string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls[res]
}

func (l *listSpy) total() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, c := range l.calls {
		n += c
	}
	return n
}

// newSpyStore builds a Store whose dynamic client reports every LIST, with
// the background sweeper marked as already running so tests drive
// sweepCounts themselves instead of racing a goroutine.
func newSpyStore(t *testing.T, objs ...runtime.Object) (*Store, *listSpy) {
	t.Helper()
	spy := &listSpy{calls: map[string]int{}, fail: map[string]error{}}

	dyn := dynamicfake.NewSimpleDynamicClient(scheme.Scheme, objs...)
	dyn.PrependReactor("list", "*", spy.react)

	c := &Client{
		RestConfig:     &rest.Config{Host: "https://fake"},
		Clientset:      fake.NewSimpleClientset(objs...),
		Dynamic:        dyn,
		Metrics:        metricsfake.NewSimpleClientset(),
		CurrentContext: "test-context",
	}
	s, err := newStoreFrom(c, apiextfake.NewSimpleClientset())
	if err != nil {
		t.Fatalf("newStoreFrom: %v", err)
	}
	t.Cleanup(s.Close)

	s.cntMu.Lock()
	s.cntRunning = true // keep the real loop out of the test's way
	s.cntMu.Unlock()
	return s, spy
}

// The sidebar decides what gets counted: a kind nobody drew a badge for
// costs nothing. This is what makes folding a group cheap — a folded group
// draws no rows, so its kinds never enter the sweep.
func TestSweepOnlyCountsWhatTheSidebarDrew(t *testing.T) {
	s, spy := newSpyStore(t)

	for _, k := range []string{"pods", "services", "nodes"} {
		s.RowCount(k, "default") // what viewList does for each visible row
	}
	s.sweepCounts("default")

	for _, res := range []string{"pods", "services", "nodes"} {
		if spy.count(res) != 1 {
			t.Errorf("%s: %d requests, want exactly 1", res, spy.count(res))
		}
	}
	for _, res := range []string{"secrets", "clusterroles", "persistentvolumes", "rolebindings"} {
		if n := spy.count(res); n != 0 {
			t.Errorf("%s was counted %d times without ever being on screen", res, n)
		}
	}
	if got, want := spy.total(), 3; got != want {
		t.Errorf("one sweep sent %d requests, want %d — one per drawn kind", got, want)
	}
}

// Once a kind is dropped from the sidebar (its group folded, say) it must
// fall out of the sweep rather than being counted forever.
func TestSweepForgetsKindsNoLongerOnScreen(t *testing.T) {
	s, spy := newSpyStore(t)

	s.RowCount("secrets", "default")
	s.sweepCounts("default")
	if spy.count("secrets") != 1 {
		t.Fatalf("secrets: %d requests, want 1", spy.count("secrets"))
	}

	// Nobody has drawn it since: age the interest past its TTL.
	s.cntMu.Lock()
	s.cntWant["secrets"] = s.cntWant["secrets"].Add(-2 * countWantTTL)
	s.cntMu.Unlock()

	s.sweepCounts("default")
	if n := spy.count("secrets"); n != 1 {
		t.Errorf("secrets was counted again (%d requests) after leaving the screen", n)
	}
	s.cntMu.RLock()
	_, still := s.cntWant["secrets"]
	s.cntMu.RUnlock()
	if still {
		t.Error("a stale interest was never forgotten")
	}
}

// A kind whose informer is running is already counted exactly, for free.
func TestSweepSkipsInformerBackedKinds(t *testing.T) {
	s, spy := newSpyStore(t, pod("default", "web-1", "node-a", true))

	s.RowCount("pods", "default")
	syncKinds(t, s, kPods)
	s.sweepCounts("default")

	if n := spy.count("pods"); n != 0 {
		t.Errorf("pods was asked for %d times while its informer was running", n)
	}
	if got := s.RowCount("pods", "default"); got != 1 {
		t.Errorf("RowCount(pods) = %d, want the informer's exact 1", got)
	}
}

// On a locked-down cluster half these kinds come back Forbidden. Asking
// again every thirty seconds is noise in someone's audit log and nothing
// else, so a refusal is remembered.
func TestForbiddenKindsAreNotReasked(t *testing.T) {
	s, spy := newSpyStore(t)
	spy.mu.Lock()
	spy.fail["secrets"] = apierrors.NewForbidden(
		schema.GroupResource{Resource: "secrets"}, "", nil)
	spy.mu.Unlock()

	s.RowCount("secrets", "default")
	s.RowCount("services", "default")
	s.sweepCounts("default")
	s.sweepCounts("default")

	if n := spy.count("secrets"); n != 1 {
		t.Errorf("a forbidden kind was asked %d times, want 1", n)
	}
	if n := spy.count("services"); n != 2 {
		t.Errorf("services: %d requests over two sweeps, want 2", n)
	}
	if got := s.RowCount("secrets", "default"); got != domain.CountUnknown {
		t.Errorf("RowCount(secrets) = %d, want CountUnknown — no badge beats a wrong one", got)
	}
}

// A blip is not a refusal: a timeout or a 500 must be retried next sweep.
func TestTransientFailuresAreRetried(t *testing.T) {
	s, spy := newSpyStore(t)
	spy.mu.Lock()
	spy.fail["services"] = apierrors.NewServiceUnavailable("cluster having a moment")
	spy.mu.Unlock()

	s.RowCount("services", "default")
	s.sweepCounts("default")
	s.sweepCounts("default")

	if n := spy.count("services"); n != 2 {
		t.Errorf("a transient failure was retried %d times, want 2", n)
	}
}

// One sweep must be a trickle, not thirty simultaneous LISTs: the bound is
// what keeps a sidebar decoration from looking like a burst of load.
//
// This drives the helper directly rather than through the fake clientset,
// whose Invokes() serialises every request behind one mutex — real
// concurrency is unobservable through it.
func TestSweepBoundsRequestsInFlight(t *testing.T) {
	const jobs = 30

	var mu sync.Mutex
	var open sync.Once
	inFlight, peak, ran := 0, 0, 0
	gate := make(chan struct{})

	forEachBounded(jobs, countConcurrency, make(chan struct{}), func(int) {
		mu.Lock()
		inFlight++
		ran++
		if inFlight > peak {
			peak = inFlight
		}
		saturated := inFlight == countConcurrency
		mu.Unlock()

		if saturated {
			// Everything the semaphore allows is running: unblock them all
			// so the sweep can finish.
			open.Do(func() { close(gate) })
		}
		<-gate

		mu.Lock()
		inFlight--
		mu.Unlock()
	})

	if ran != jobs {
		t.Errorf("%d of %d jobs ran", ran, jobs)
	}
	if peak > countConcurrency {
		t.Errorf("%d requests in flight at once, want at most %d", peak, countConcurrency)
	}
	if peak < 2 {
		t.Errorf("peak concurrency %d — the sweep is serial, which is slow for no reason", peak)
	}
}

// Quitting must not wait on a slow API server.
func TestBoundedWorkStopsWhenAsked(t *testing.T) {
	stop := make(chan struct{})
	close(stop)

	ran := 0
	var mu sync.Mutex
	forEachBounded(50, 2, stop, func(int) {
		mu.Lock()
		ran++
		mu.Unlock()
	})
	if ran != 0 {
		t.Errorf("%d jobs ran after stop was closed", ran)
	}
}
