package k8s

// Retain four namespace caches for cheap tab backtracking. The most recently
// requested scope is never the victim. Cluster-wide/cluster-scoped informers
// are a separate fixed set. Eviction drops Store ownership, not outstanding
// lister snapshots; actions use direct requests and do not lease informers.
const maxNamespaceScopes = 4

func (s *Store) makeScopeRoomLocked() {
	if len(s.namespacedFactories) < maxNamespaceScopes {
		return
	}
	var victim string
	oldest := ^uint64(0)
	for ns := range s.namespacedFactories {
		if s.scopeUsed[ns] < oldest {
			victim = ns
			oldest = s.scopeUsed[ns]
		}
	}
	s.evictScopeLocked(victim)
}

func (s *Store) evictScopeLocked(ns string) {
	factory := s.namespacedFactories[ns]
	if factory == nil {
		return
	}
	close(s.scopeStops[ns])
	delete(s.scopeStops, ns)
	delete(s.scopeUsed, ns)
	delete(s.namespacedFactories, ns)
	for key := range s.started {
		if key.namespace == ns {
			delete(s.started, key)
			delete(s.health, key)
		}
	}
	// Joining informer goroutines must not block the render path. Close waits
	// for these owned shutdown tasks, including already-evicted generations.
	s.scopeWG.Add(1)
	go func() { defer s.scopeWG.Done(); factory.Shutdown() }()
}
