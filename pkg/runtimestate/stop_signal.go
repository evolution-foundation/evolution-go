package runtimestate

import (
	"fmt"
	"sync"
	"time"
)

// StopSignal broadcasts an idempotent stop to the workers of one generation.
type StopSignal struct {
	init         sync.Once
	mu           sync.Mutex
	started      bool
	stopped      bool
	done         chan struct{}
	finished     chan struct{}
	finishedOnce sync.Once
	workers      sync.WaitGroup
}

func NewStopSignal() *StopSignal { s := &StopSignal{}; s.initialize(); return s }
func (s *StopSignal) initialize() {
	s.init.Do(func() { s.done = make(chan struct{}); s.finished = make(chan struct{}) })
}
func (s *StopSignal) Done() <-chan struct{} {
	if s == nil {
		return nil
	}
	s.initialize()
	return s.done
}
func (s *StopSignal) Begin() bool {
	if s == nil {
		return false
	}
	s.initialize()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.stopped {
		return false
	}
	s.started = true
	return true
}
func (s *StopSignal) Stop() {
	if s == nil {
		return
	}
	s.initialize()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		s.stopped = true
		close(s.done)
	}
	if !s.started {
		s.Complete()
	}
}
func (s *StopSignal) Complete() {
	if s == nil {
		return
	}
	s.initialize()
	s.workers.Wait()
	s.finishedOnce.Do(func() { close(s.finished) })
}
func (s *StopSignal) Wait(timeout time.Duration) bool {
	if s == nil {
		return true
	}
	s.initialize()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.finished:
		return true
	case <-timer.C:
		return false
	}
}

// NewGeneration waits for the previous worker to finish its database/cache cleanup
// before exposing a replacement. Concurrent starts are serialized only for this ID.
func NewGeneration(registry *Map[string, *StopSignal], id string, timeout time.Duration) (*StopSignal, error) {
	unlock := registry.LockKey(id)
	defer unlock()
	return NewGenerationLocked(registry, id, timeout)
}

// NewGenerationLocked requires the caller to hold registry.LockKey(id).
func NewGenerationLocked(registry *Map[string, *StopSignal], id string, timeout time.Duration) (*StopSignal, error) {
	previous := registry.Get(id)
	previous.Stop()
	if !previous.Wait(timeout) {
		return nil, fmt.Errorf("previous connection is still stopping")
	}
	signal := NewStopSignal()
	registry.Set(id, signal)
	return signal, nil
}

// Run tracks side-effect workers so a replacement cannot outlive their cleanup.
func (s *StopSignal) Run(work func()) bool {
	if s == nil {
		return false
	}
	s.initialize()
	s.mu.Lock()
	if s.stopped || !s.started {
		s.mu.Unlock()
		return false
	}
	s.workers.Add(1)
	s.mu.Unlock()
	go func() { defer s.workers.Done(); work() }()
	return true
}

// Sleep waits for rotation timing while remaining responsive to session stop.
func (s *StopSignal) Sleep(duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-s.Done():
		return false
	case <-timer.C:
		return true
	}
}
