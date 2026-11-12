package runtimestate

import (
	"fmt"
	"sync"
	"testing"
)

func TestMapZeroValueAndMissingEntries(t *testing.T) {
	var m Map[string, int]
	if got, ok := m.Load("absent"); ok || got != 0 {
		t.Fatal("missing entry must have zero value")
	}
	m.Set("present", 7)
	if got := m.Get("present"); got != 7 {
		t.Fatalf("got %d", got)
	}
	m.Delete("present")
	m.Delete("present")
	if _, ok := m.Load("present"); ok {
		t.Fatal("cleanup must be idempotent")
	}
}

func TestConcurrentLogoutCleanupPreservesOtherSessions(t *testing.T) {
	clients := New[string, int]()
	metadata := New[string, int]()
	controls := New[string, int]()
	for _, m := range []*Map[string, int]{clients, metadata, controls} {
		m.Set("other-connected-number", 2)
		m.Set("closing", 1)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < 24; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for n := 0; n < 10000; n++ {
				for _, m := range []*Map[string, int]{clients, metadata, controls} {
					m.Delete("closing")
					if m.Get("other-connected-number") != 2 {
						t.Error("unrelated session changed")
						return
					}
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	for _, m := range []*Map[string, int]{clients, metadata, controls} {
		if _, ok := m.Load("closing"); ok {
			t.Fatal("closed entry remains")
		}
		if m.Get("other-connected-number") != 2 {
			t.Fatal("unrelated session missing")
		}
	}
}

func TestConcurrentRegistryReadsWritesAndRemovals(t *testing.T) {
	m := New[string, int]()
	m.Set("untouched", 42)
	var wg sync.WaitGroup
	for worker := 0; worker < 24; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			key := fmt.Sprintf("worker-%d", worker)
			for n := 0; n < 10000; n++ {
				m.Set(key, n)
				if got := m.Get(key); got != n {
					t.Errorf("worker entry changed: %d", got)
					return
				}
				m.Load(key)
				m.Delete(key)
			}
		}(worker)
	}
	wg.Wait()
	if m.Get("untouched") != 42 {
		t.Fatal("unrelated instance was removed")
	}
}
