package whatsmeow_service

import (
	"sync"
	"testing"

	"go.mau.fi/whatsmeow"
)

// GetClient must be safe to call while StartClient/reconnect publish and clear
// runtime pointers. Run with -race: an unsynchronized map read would be reported
// (and in production can crash the process with "concurrent map read and map write").
func TestGetClientConcurrentWithPointerUpdates(t *testing.T) {
	w := whatsmeowService{
		clientPointer:   make(map[string]*whatsmeow.Client),
		myClientPointer: make(map[string]*MyClient),
		killChannel:     make(map[string](chan bool)),
		lifecycle: &runtimeLifecycle{
			starting:     make(map[string]bool),
			restartLocks: make(map[string]*sync.Mutex),
		},
	}

	client := &whatsmeow.Client{}
	mycli := &MyClient{}
	stop := make(chan bool, 1)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			w.setRuntimePointers("a", client, mycli, stop)
			w.clearRuntimePointers("a", client, mycli, stop)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			if got := w.GetClient("a"); got != nil && got != client {
				t.Errorf("unexpected client %p", got)
			}
		}
	}()
	wg.Wait()

	w.setRuntimePointers("a", client, mycli, stop)
	if got := w.GetClient("a"); got != client {
		t.Fatalf("GetClient = %p, want %p", got, client)
	}
	if got := w.GetClient("missing"); got != nil {
		t.Fatalf("GetClient(missing) = %p, want nil", got)
	}
}
