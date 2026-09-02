package k8s

import (
	"context"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/p10node/k10s/internal/domain"
)

// Sidebar badge counts for kinds the user hasn't opened.
//
// Starting an informer just to show a number would mean a cluster-wide
// LIST+WATCH per kind — the thing that made k10s slow. Instead this asks the
// API server for a one-item page and reads ListMeta.RemainingItemCount, so
// each kind costs a single tiny request no matter how many objects exist.
// It runs entirely off the render path; the sidebar only ever reads the map.
//
// Kubernetes has no "count these twenty resources" call, so the work is in
// asking for as few of them as possible:
//
//   - Only kinds the sidebar is actually showing are swept. The render path
//     stamps each kind it draws a badge for (noteInterest); a kind nobody
//     has drawn for countWantTTL drops out. Folding a group away therefore
//     stops its requests, and a fresh k10s on a 30-kind list asks about the
//     dozen or so that fit on screen.
//   - Kinds with a running informer are skipped entirely: their cache
//     already knows the exact number, for free.
//   - Requests are capped at countConcurrency in flight, so a sweep is a
//     trickle rather than a burst of thirty simultaneous LISTs.
//   - A kind the cluster refuses (missing API group, RBAC) is remembered
//     and left alone for countGoneTTL instead of being re-asked every sweep.

// countRefreshInterval is deliberately slow: these numbers are a hint in the
// sidebar, not live data. The kind you actually open is informer-backed and
// updates in real time.
const countRefreshInterval = 30 * time.Second

// countWantTTL is how long a kind stays in the sweep after the last time the
// sidebar drew it. Comfortably longer than the interval, so a kind on screen
// is never dropped between two sweeps, and short enough that one that
// scrolled out of view (or whose group was folded) stops costing requests.
const countWantTTL = 90 * time.Second

// countGoneTTL is how long a kind the cluster wouldn't count is left alone.
const countGoneTTL = 10 * time.Minute

// countConcurrency bounds how many count requests are in flight at once.
const countConcurrency = 6

// countKickDelay coalesces the burst of "I need this one too" that arrives
// when a group is unfolded, so unfolding costs one sweep and not five.
const countKickDelay = 250 * time.Millisecond

// noteInterest records that the sidebar just drew a badge for kind in ns,
// and starts the background sweeper on first call. It is called from the
// render path, so it must stay lock-cheap and never do I/O itself.
func (s *Store) noteInterest(kind, ns string) {
	now := time.Now()

	s.cntMu.Lock()
	changed := s.cntNS != ns
	s.cntNS = ns
	if s.cntWant == nil {
		s.cntWant = map[string]time.Time{}
	}
	_, known := s.cntWant[kind]
	s.cntWant[kind] = now
	already := s.cntRunning
	s.cntRunning = true
	s.cntMu.Unlock()

	// Custom resources are counted by their own sweep, which costs one LIST
	// per CRD — dozens on a cluster running cert-manager, Argo and
	// prometheus-operator. It starts only when that kind is on screen, not
	// merely because the sidebar was drawn.
	if kind == "customresources" {
		s.ensureCRRefresh()
	}

	if !already {
		go s.countLoop()
		return
	}

	// A namespace switch invalidates every number on screen, and a kind
	// nobody had asked about before has no number at all. Both are worth a
	// prompt sweep; anything else waits for the interval. Note that this
	// cannot fire every frame: after the first sweep the kind is "known".
	if changed || !known {
		select {
		case s.cntKick <- struct{}{}:
		default:
		}
	}
}

func (s *Store) countLoop() {
	for {
		s.cntMu.RLock()
		ns := s.cntNS
		s.cntMu.RUnlock()
		s.sweepCounts(ns)

		select {
		case <-s.stop:
			return
		case <-s.cntKick:
			// Wait out the rest of the burst before sweeping again.
			select {
			case <-s.stop:
				return
			case <-time.After(countKickDelay):
			}
		case <-time.After(countRefreshInterval):
		}
	}
}

func countKey(kind, ns string) string { return kind + "/" + ns }

// cachedCount returns a previously swept count, if any.
func (s *Store) cachedCount(kind, ns string) (int, bool) {
	s.cntMu.RLock()
	defer s.cntMu.RUnlock()
	n, ok := s.cnt[countKey(kind, ns)]
	return n, ok
}

// wantedKinds returns the kinds worth a request right now: asked for by the
// sidebar recently, not already informer-backed, and not one the cluster has
// refused lately.
func (s *Store) wantedKinds(ns string) []domain.Kind {
	now := time.Now()

	s.cntMu.RLock()
	want := make(map[string]bool, len(s.cntWant))
	for k, at := range s.cntWant {
		if now.Sub(at) <= countWantTTL {
			want[k] = true
		}
	}
	gone := make(map[string]time.Time, len(s.cntGone))
	for k, at := range s.cntGone {
		gone[k] = at
	}
	s.cntMu.RUnlock()

	var out []domain.Kind
	for _, k := range builtinKinds {
		if !want[k.Key] {
			continue
		}
		// An open kind already has exact numbers from its cache.
		if s.isStarted(k.Key, ns) {
			continue
		}
		if at, skip := gone[k.Key]; skip && now.Sub(at) < countGoneTTL {
			continue
		}
		out = append(out, k)
	}
	return out
}

// forgetStaleWants drops kinds nobody has drawn in a while, so the map
// doesn't grow into a record of everything ever looked at.
func (s *Store) forgetStaleWants() {
	now := time.Now()
	s.cntMu.Lock()
	for k, at := range s.cntWant {
		if now.Sub(at) > countWantTTL {
			delete(s.cntWant, k)
		}
	}
	s.cntMu.Unlock()
}

// sweepCounts refreshes counts for the kinds wantedKinds selected, for the
// namespace currently on screen. Kinds are counted in parallel — bounded by
// countConcurrency — so one slow API group doesn't hold up the rest without
// the sweep turning into a thundering herd.
func (s *Store) sweepCounts(ns string) {
	s.forgetStaleWants()
	kinds := s.wantedKinds(ns)
	if len(kinds) == 0 {
		return
	}

	results := make([]int, len(kinds))
	unavailable := make([]bool, len(kinds))

	forEachBounded(len(kinds), countConcurrency, s.stop, func(i int) {
		results[i], unavailable[i] = s.countKind(kinds[i], ns)
	})

	now := time.Now()
	s.cntMu.Lock()
	if s.cnt == nil {
		s.cnt = map[string]int{}
	}
	if s.cntGone == nil {
		s.cntGone = map[string]time.Time{}
	}
	for i, k := range kinds {
		if results[i] >= 0 {
			s.cnt[countKey(k.Key, ns)] = results[i]
			delete(s.cntGone, k.Key)
			continue
		}
		if unavailable[i] {
			s.cntGone[k.Key] = now
		}
	}
	s.cntMu.Unlock()
}

// forEachBounded runs fn(0..n-1) concurrently with at most limit in flight,
// and returns once they are all done — or as soon as stop is closed, which
// is what lets a quit not wait on a slow API server.
//
// The bound is the point: thirty kinds firing thirty simultaneous LISTs at
// somebody's API server is a burst worth avoiding for a number that only
// decorates a sidebar.
func forEachBounded(n, limit int, stop <-chan struct{}, fn func(i int)) {
	if limit < 1 {
		limit = 1
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, limit)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Checked before the send: a select with both cases ready picks
			// at random, which would let work start after a quit.
			select {
			case <-stop:
				return
			default:
			}
			select {
			case sem <- struct{}{}:
			case <-stop:
				return
			}
			defer func() { <-sem }()
			fn(i)
		}(i)
	}
	wg.Wait()
}

// countKind returns how many objects of k exist in ns, or CountUnknown if it
// can't be determined. The second result says the answer will not change by
// asking again soon — the API group isn't served, or this user may not list
// it — which is what keeps a locked-down cluster from being re-asked every
// sweep. A timeout or a blip is not "gone": that just retries next time.
//
// It recovers from panics: this runs on a background goroutine, where an
// unhandled panic would take down the whole TUI. A badge that can't be
// computed is not worth crashing a user's session over.
func (s *Store) countKind(k domain.Kind, ns string) (n int, gone bool) {
	defer func() {
		if recover() != nil {
			n, gone = domain.CountUnknown, false
		}
	}()
	return s.countKindUnsafe(k, ns)
}

// permanentish reports whether an error means "don't bother asking again for
// a while", as opposed to a transient failure worth retrying next sweep.
func permanentish(err error) bool {
	return apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) ||
		apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err) ||
		meta.IsNoMatchError(err)
}

func (s *Store) countKindUnsafe(k domain.Kind, ns string) (int, bool) {
	if k.Key == "customresources" {
		// Counting these means one request per CRD; the background CR sweep
		// already produces the number when the view is opened.
		s.crMu.Lock()
		cached, have := s.crCache, !s.crAt.IsZero()
		s.crMu.Unlock()
		if !have {
			return domain.CountUnknown, false
		}
		_, rows := applyNamespace(nil, cached, ns)
		return len(rows), false
	}

	gvr, namespaced, err := s.gvrFor(k.Key)
	if err != nil {
		return domain.CountUnknown, true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Limit:1 keeps the response tiny; the server reports how many more it
	// would have returned, which is all the badge needs.
	opts := metav1.ListOptions{Limit: 1}
	var (
		items     int
		remaining int64
	)
	if namespaced && ns != domain.AllNamespaces {
		eff := ns
		if eff == "" {
			eff = "default"
		}
		l, err := s.c.Dynamic.Resource(gvr).Namespace(eff).List(ctx, opts)
		if err != nil {
			return domain.CountUnknown, permanentish(err)
		}
		items = len(l.Items)
		if l.GetRemainingItemCount() != nil {
			remaining = *l.GetRemainingItemCount()
		}
	} else {
		l, err := s.c.Dynamic.Resource(gvr).List(ctx, opts)
		if err != nil {
			return domain.CountUnknown, permanentish(err)
		}
		items = len(l.Items)
		if l.GetRemainingItemCount() != nil {
			remaining = *l.GetRemainingItemCount()
		}
	}
	return items + int(remaining), false
}
