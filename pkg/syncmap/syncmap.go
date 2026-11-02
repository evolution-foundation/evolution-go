package syncmap

import "sync"

type Map[K comparable, V comparable] struct {
	mu sync.RWMutex
	m  map[K]V
}

func New[K comparable, V comparable]() *Map[K, V] {
	return &Map[K, V]{m: make(map[K]V)}
}

func (s *Map[K, V]) Load(key K) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[key]
	return v, ok
}

func (s *Map[K, V]) Get(key K) V {
	v, _ := s.Load(key)
	return v
}

func (s *Map[K, V]) Store(key K, value V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
}

func (s *Map[K, V]) Delete(key K) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}

// CompareAndDelete removes key only while it still maps to value, so a stale
// goroutine cannot remove the entry a newer client has stored for the same key.
func (s *Map[K, V]) CompareAndDelete(key K, value V) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.m[key]; ok && current == value {
		delete(s.m, key)
		return true
	}
	return false
}
