package ui

import (
	"github.com/p10node/k10s/internal/domain"
	"sync"
)

// A source remains alive until every admitted command has returned. Retirement
// is not cancellation: writes already sent may have taken effect; never retry.
func leasedStop(stop, release func()) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			if stop != nil {
				stop()
			}
			release()
		})
	}
}

type leasedShell struct {
	domain.ShellSession
	stop func()
}

func (s *leasedShell) Close() error { s.stop(); return nil }

func (m *Model) invalidateSession() {
	m.backToTable()
	m.confirm = nil
	for key, stop := range m.portFwds {
		if stop != nil {
			stop()
		}
		delete(m.portFwds, key)
	}
	// Dials still in flight belong to the previous session; their results
	// are discarded on arrival, so the next press must start afresh.
	clear(m.forwardPending)
}

type sourceLease struct {
	mu      sync.Mutex
	src     domain.Source
	refs    int
	retired bool
	closed  bool
}

func (s *sourceLease) acquire() func() {
	s.mu.Lock()
	s.refs++
	s.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { s.mu.Lock(); s.refs--; s.closeIfIdle(); s.mu.Unlock() }) }
}
func (s *sourceLease) retire() { s.mu.Lock(); s.retired = true; s.closeIfIdle(); s.mu.Unlock() }
func (s *sourceLease) closeIfIdle() {
	if s.retired && s.refs == 0 && !s.closed {
		s.closed = true
		// Store.Close waits for informer shutdown; the last release often
		// happens on the Bubble Tea goroutine, which must not block on it.
		go s.src.Close()
	}
}
func (m *Model) leaseSource() func() {
	if m.sourceOwner == nil {
		m.sourceOwner = &sourceLease{src: m.src}
	}
	return m.sourceOwner.acquire()
}
func (m *Model) replaceSource(src domain.Source) {
	// Commands can start on the old source while a connection is pending.
	// Accepting the new backend is a second invalidation boundary.
	m.invalidateSession()
	m.connGen++
	m.crSelection = resourceSelection{}
	m.installSource(src)
}

// installSource swaps the live source and retires the previous owner, so
// leases taken afterwards belong to src. Every assignment to m.src after
// construction goes through here: a bare assignment leaves the owner on the
// old source, and the next swap would close that one instead of this.
func (m *Model) installSource(src domain.Source) {
	if m.sourceOwner == nil {
		m.sourceOwner = &sourceLease{src: m.src}
	}
	old := m.sourceOwner
	m.src = src
	m.sourceOwner = &sourceLease{src: src}
	old.retire()
}
