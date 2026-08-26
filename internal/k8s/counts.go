package k8s

import (
	"context"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k10s/internal/domain"
)

// Sidebar badge counts for kinds the user hasn't opened.
//
// Starting an informer just to show a number would mean a cluster-wide
// LIST+WATCH per kind — the thing that made k10s slow. Instead this asks the
// API server for a one-item page and reads ListMeta.RemainingItemCount, so
// each kind costs a single tiny request no matter how many objects exist.
// It runs entirely off the render path; the sidebar only ever reads the map.

// countRefreshInterval is deliberately slow: these numbers are a hint in the
// sidebar, not live data. The kind you actually open is informer-backed and
// updates in real time.
const countRefreshInterval = 30 * time.Second

// noteInterest records the namespace the sidebar is currently showing and
// starts the background sweeper on first call. Called from the render path,
// so it must stay lock-cheap and never do I/O itself.
func (s *Store) noteInterest(ns string) {
	s.cntMu.Lock()
	changed := s.cntNS != ns
	s.cntNS = ns
	already := s.cntRunning
	s.cntRunning = true
	s.cntMu.Unlock()

	if already {
		if changed {
			// Namespace switched: refresh promptly rather than waiting out
			// the interval, so badges match what's on screen.
			select {
			case s.cntKick <- struct{}{}:
			default:
			}
		}
		return
	}

	// Custom resources are counted by their own sweep; let it populate too.
	s.ensureCRRefresh()

	go func() {
		for {
			s.cntMu.RLock()
			ns := s.cntNS
			s.cntMu.RUnlock()
			s.sweepCounts(ns)

			select {
			case <-s.stop:
				return
			case <-s.cntKick:
			case <-time.After(countRefreshInterval):
			}
		}
	}()
}

func countKey(kind, ns string) string { return kind + "/" + ns }

// cachedCount returns a previously swept count, if any.
func (s *Store) cachedCount(kind, ns string) (int, bool) {
	s.cntMu.RLock()
	defer s.cntMu.RUnlock()
	n, ok := s.cnt[countKey(kind, ns)]
	return n, ok
}

// sweepCounts refreshes counts for every kind that isn't informer-backed
// yet, for the namespace currently on screen. Kinds are counted in parallel
// so one slow API group doesn't hold up the rest.
func (s *Store) sweepCounts(ns string) {
	kinds := builtinKinds
	var wg sync.WaitGroup
	results := make([]int, len(kinds))

	for i, k := range kinds {
		// An open kind already has exact numbers from its cache.
		if s.isStarted(k.Key) {
			results[i] = domain.CountUnknown
			continue
		}
		wg.Add(1)
		go func(i int, k domain.Kind) {
			defer wg.Done()
			results[i] = s.countKind(k, ns)
		}(i, k)
	}
	wg.Wait()

	s.cntMu.Lock()
	if s.cnt == nil {
		s.cnt = map[string]int{}
	}
	for i, k := range kinds {
		if results[i] >= 0 {
			s.cnt[countKey(k.Key, ns)] = results[i]
		}
	}
	s.cntMu.Unlock()
}

// countKind returns how many objects of k exist in ns, or CountUnknown if it
// can't be determined (no permission, API group missing, request failed).
//
// It recovers from panics: this runs on a background goroutine, where an
// unhandled panic would take down the whole TUI. A badge that can't be
// computed is not worth crashing a user's session over.
func (s *Store) countKind(k domain.Kind, ns string) (n int) {
	defer func() {
		if recover() != nil {
			n = domain.CountUnknown
		}
	}()
	return s.countKindUnsafe(k, ns)
}

func (s *Store) countKindUnsafe(k domain.Kind, ns string) int {
	if k.Key == "customresources" {
		// Counting these means one request per CRD; the background CR sweep
		// already produces the number when the view is opened.
		s.crMu.Lock()
		cached, have := s.crCache, !s.crAt.IsZero()
		s.crMu.Unlock()
		if !have {
			return domain.CountUnknown
		}
		_, rows := applyNamespace(nil, cached, ns)
		return len(rows)
	}

	gvr, namespaced, err := s.gvrFor(k.Key)
	if err != nil {
		return domain.CountUnknown
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
			return domain.CountUnknown
		}
		items = len(l.Items)
		if l.GetRemainingItemCount() != nil {
			remaining = *l.GetRemainingItemCount()
		}
	} else {
		l, err := s.c.Dynamic.Resource(gvr).List(ctx, opts)
		if err != nil {
			return domain.CountUnknown
		}
		items = len(l.Items)
		if l.GetRemainingItemCount() != nil {
			remaining = *l.GetRemainingItemCount()
		}
	}
	return items + int(remaining)
}
