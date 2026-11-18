package safemap

import (
	"sync"
	"testing"
)

// TestPlainMapRaces is the calibration test: it proves the detector can see the
// bug in the first place.
//
// It drives a plain Go map the way this code used to — N goroutines writing the
// same key. Under -race the detector flags it. Without -race the runtime kills
// the process with "fatal error: concurrent map writes", which is exactly the
// failure this package exists to prevent.
//
// Skipped by default precisely because that failure is not recoverable and
// would take the whole suite down. Run it on purpose to confirm the detector is
// not blind:
//
//	go test -race -run TestPlainMapRaces ./pkg/safemap/
func TestPlainMapRaces(t *testing.T) {
	t.Skip("calibration: run by hand — see the comment above")

	m := map[string]int{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m["key"] = n // <- the race
		}(i)
	}
	wg.Wait()
}

// TestConcurrentAccess runs the same exercise through the guarded map: fifty
// goroutines writing, reading and ranging over the same key at once — the shape
// of a startup where several instances connect simultaneously.
func TestConcurrentAccess(t *testing.T) {
	m := New[int]()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(n int) { defer wg.Done(); m.Set("key", n) }(i)
		go func() { defer wg.Done(); _ = m.Get("key") }()
		go func() {
			defer wg.Done()
			_, _ = m.Lookup("key")
			_ = m.Len()
			for range m.Snapshot() {
			}
		}()
	}
	wg.Wait()

	if _, ok := m.Lookup("key"); !ok {
		t.Fatal("key should still be in the map")
	}
}

// TestGetMissingKeyReturnsZero pins the behaviour the call sites relied on:
// m[k] on an absent key yields the zero value, and Get must do the same —
// otherwise swapping the call sites would have changed logic silently.
func TestGetMissingKeyReturnsZero(t *testing.T) {
	if v := New[*int]().Get("absent"); v != nil {
		t.Fatalf("want nil, got %v", v)
	}
	if n := New[int]().Get("absent"); n != 0 {
		t.Fatalf("want 0, got %d", n)
	}
}
