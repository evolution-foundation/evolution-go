package whatsmeow_service

import (
	"sync"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestQueryClientReplacementAndIsolation(t *testing.T) {
	old := &whatsmeow.Client{}
	replacement := &whatsmeow.Client{}
	other := &whatsmeow.Client{}
	w := whatsmeowService{queryClients: newQueryClientIndex(map[string]*whatsmeow.Client{"one": old, "two": other, "nil": nil})}
	if w.GetClient("one") != old || w.GetClient("nil") != nil {
		t.Fatal("initial clients were not preserved")
	}
	w.registerQueryClient("one", replacement)
	w.removeQueryClient("one", old)
	if w.GetClient("one") != replacement || w.GetClient("two") != other || w.GetClient("missing") != nil {
		t.Fatal("cleanup removed replacement or crossed instances")
	}
	w.removeQueryClient("one", replacement)
	if w.GetClient("one") != nil || w.GetClient("two") != other {
		t.Fatal("matching cleanup failed")
	}
}

func TestQueryClientConcurrentRegistration(t *testing.T) {
	w := whatsmeowService{queryClients: &sync.Map{}}
	var workers sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			client := &whatsmeow.Client{}
			for j := 0; j < 100; j++ {
				w.registerQueryClient("one", client)
				w.GetClient("one")
				w.removeQueryClient("one", client)
			}
		}()
	}
	close(start)
	workers.Wait()
	replacement := &whatsmeow.Client{}
	w.registerQueryClient("one", replacement)
	if w.GetClient("one") != replacement {
		t.Fatal("final session missing")
	}
}
