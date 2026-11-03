package runtimestate

import (
	"sync"
	"testing"
	"time"
)

func TestStopSignalReleasesAllWorkersAndIsIdempotent(t *testing.T) {
	var signal StopSignal
	var workers, stoppers sync.WaitGroup
	for n := 0; n < 24; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			select {
			case <-signal.Done():
			case <-time.After(2 * time.Second):
				t.Error("stop was not broadcast")
			}
		}()
	}
	for n := 0; n < 24; n++ {
		stoppers.Add(1)
		go func() {
			defer stoppers.Done()
			for i := 0; i < 100; i++ {
				signal.Stop()
			}
		}()
	}
	stoppers.Wait()
	workers.Wait()
	signal.Stop()
}
func TestStoppedGenerationDoesNotStopReplacement(t *testing.T) {
	old := NewStopSignal()
	next := NewStopSignal()
	old.Stop()
	select {
	case <-next.Done():
		t.Fatal("replacement stopped")
	default:
	}
}

func TestOldGenerationCleanupCannotRemoveReplacement(t *testing.T) {
	clients := New[string, int]()
	signals := New[string, *StopSignal]()
	old := NewStopSignal()
	next := NewStopSignal()
	clients.Set("number", 2)
	signals.Set("number", next)
	if clients.CompareAndDelete("number", 1) {
		t.Fatal("old client removed replacement")
	}
	if signals.CompareAndDelete("number", old) {
		t.Fatal("old signal removed replacement")
	}
	if clients.Get("number") != 2 || signals.Get("number") != next {
		t.Fatal("replacement changed")
	}
}

func TestNewGenerationWaitsForOldCleanup(t *testing.T) {
	registry := New[string, *StopSignal]()
	old := NewStopSignal()
	if !old.Begin() {
		t.Fatal("begin")
	}
	registry.Set("number", old)
	result := make(chan *StopSignal, 1)
	failure := make(chan error, 1)
	go func() { next, err := NewGeneration(registry, "number", time.Second); result <- next; failure <- err }()
	<-old.Done()
	if registry.Get("number") != old {
		t.Fatal("replacement exposed before cleanup")
	}
	old.Complete()
	next := <-result
	if err := <-failure; err != nil {
		t.Fatal(err)
	}
	if registry.Get("number") != next || !next.Begin() {
		t.Fatal("replacement not active")
	}
	next.Stop()
	next.Complete()
}

func TestNewGenerationTimeoutRetainsOldHandle(t *testing.T) {
	registry := New[string, *StopSignal]()
	old := NewStopSignal()
	old.Begin()
	registry.Set("number", old)
	if _, err := NewGeneration(registry, "number", time.Millisecond); err == nil {
		t.Fatal("must wait for cleanup")
	}
	if registry.Get("number") != old {
		t.Fatal("timed out start replaced old session")
	}
	old.Complete()
}

func TestSupersededUnstartedGenerationCannotStart(t *testing.T) {
	registry := New[string, *StopSignal]()
	first, err := NewGeneration(registry, "number", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	next, err := NewGeneration(registry, "number", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if first.Begin() {
		t.Fatal("superseded generation started")
	}
	if !next.Begin() {
		t.Fatal("current generation did not start")
	}
	next.Stop()
	next.Complete()
}

func TestGenerationCompletionWaitsForChildWorkers(t *testing.T) {
	signal := NewStopSignal()
	signal.Begin()
	entered := make(chan struct{})
	release := make(chan struct{})
	if !signal.Run(func() { close(entered); <-release }) {
		t.Fatal("worker not accepted")
	}
	<-entered
	signal.Stop()
	go signal.Complete()
	if signal.Wait(time.Millisecond) {
		t.Fatal("finished before worker cleanup")
	}
	close(release)
	if !signal.Wait(time.Second) {
		t.Fatal("worker never finished")
	}
	if signal.Run(func() { t.Error("worker started after stop") }) {
		t.Fatal("stopped generation accepted worker")
	}
}

func TestRotationSleepIsInterruptedByStop(t *testing.T) {
	signal := NewStopSignal()
	result := make(chan bool, 1)
	go func() { result <- signal.Sleep(time.Hour) }()
	signal.Stop()
	select {
	case elapsed := <-result:
		if elapsed {
			t.Fatal("old QR rotation continued")
		}
	case <-time.After(time.Second):
		t.Fatal("rotation did not stop")
	}
}

func TestLifecycleLockDoesNotBlockAnotherNumber(t *testing.T) {
	registry := New[string, int]()
	unlock := registry.LockKey("one")
	defer unlock()
	done := make(chan struct{})
	go func() { release := registry.LockKey("other"); release(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("per-instance operation locked every instance")
	}
}
