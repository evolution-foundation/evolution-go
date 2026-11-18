// Package safemap holds the process-wide maps that are shared across goroutines.
//
// WHY THIS EXISTS
//
// The three state maps of the process (clientPointer, myClientPointer and
// killChannel) are created once at startup and passed BY REFERENCE into eleven
// packages. Every one of them read and wrote those maps directly, from different
// goroutines, with no synchronisation at all.
//
// Go does not tolerate that. Concurrent writes do not corrupt silently: the
// runtime kills the process with
//
//	fatal error: concurrent map writes
//
// and a fatal error is NOT recoverable — a deferred recover() does not catch it.
//
// Measured in production: a process died at StartClient while five instances
// were connecting at the same time. The container orchestrator brought it back
// within seconds, but every number was offline in the meantime, and it repeats
// on every restart that brings up more than a couple of instances at once.
//
// WHY A SEPARATE PACKAGE, AND WHY GENERICS
//
// MyClient lives in pkg/whatsmeow/service. If the safe type lived there, the
// other ten packages would have to import that package and an import cycle
// would appear. This package has no internal dependencies — only the standard
// library — so anything can use it.
package safemap

import "sync"

// Map is a map[string]T guarded by a read/write mutex.
//
// Always use the pointer (*Map[T]): copying the struct would copy the mutex,
// and a copied mutex stops protecting the original — two owners, one lock each,
// and the race comes back silently.
type Map[T any] struct {
	mu sync.RWMutex
	m  map[string]T
}

// New returns an empty map, ready to use.
func New[T any]() *Map[T] {
	return &Map[T]{m: make(map[string]T)}
}

// Get returns the value, or the zero value of T when the key is absent —
// exactly like the m[k] it replaces, so swapping call sites changes no
// behaviour.
func (s *Map[T]) Get(k string) T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m[k]
}

// Lookup is the v, ok := m[k] form.
func (s *Map[T]) Lookup(k string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[k]
	return v, ok
}

// Set is the m[k] = v form.
func (s *Map[T]) Set(k string, v T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
}

// Delete is the delete(m, k) form.
func (s *Map[T]) Delete(k string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, k)
}

// Len is the len(m) form.
func (s *Map[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}

// Snapshot returns a copy for range loops.
//
// Ranging over the map from the inside would hold the lock for the whole loop,
// and the loop bodies here talk to WhatsApp — the lock would be held across
// network calls. The snapshot costs one copy of the pointers and releases the
// lock immediately.
func (s *Map[T]) Snapshot() map[string]T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]T, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out
}
