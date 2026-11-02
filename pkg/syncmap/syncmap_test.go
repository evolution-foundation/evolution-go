package syncmap

import (
	"strconv"
	"sync"
	"testing"
)

func TestConcurrentAccess(t *testing.T) {
	m := New[string, int]()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := strconv.Itoa(i % 5)
			m.Store(key, i)
			m.Get(key)
			m.Load(key)
			m.CompareAndDelete(key, i)
			m.Delete(key)
		}(i)
	}
	wg.Wait()
}

func TestGetMissingReturnsZero(t *testing.T) {
	m := New[string, *int]()
	if m.Get("x") != nil {
		t.Fatal("expected nil for a missing key")
	}
	if _, ok := m.Load("x"); ok {
		t.Fatal("expected ok=false for a missing key")
	}
}

func TestCompareAndDeleteKeepsNewerValue(t *testing.T) {
	m := New[string, chan bool]()
	old, current := make(chan bool), make(chan bool)
	m.Store("a", current)

	if m.CompareAndDelete("a", old) {
		t.Fatal("must not delete when the stored value was replaced")
	}
	if m.Get("a") != current {
		t.Fatal("newer value was lost")
	}
	if !m.CompareAndDelete("a", current) {
		t.Fatal("expected delete of the matching value")
	}
	if _, ok := m.Load("a"); ok {
		t.Fatal("key still present after CompareAndDelete")
	}
}
