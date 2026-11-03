package instance_service

import (
	"testing"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/syncmap"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
)

type fakeWhatsmeowService struct {
	whatsmeow_service.WhatsmeowService
	startCalls     int
	reconnectCalls int
}

func (f *fakeWhatsmeowService) StartInstance(string) error {
	f.startCalls++
	return nil
}

func (f *fakeWhatsmeowService) ReconnectClient(string) error {
	f.reconnectCalls++
	return nil
}

func newReconnectSubject(withClient bool) (instances, *fakeWhatsmeowService) {
	fake := &fakeWhatsmeowService{}
	clients := syncmap.New[string, *whatsmeow.Client]()
	if withClient {
		clients.Store("inst-1", &whatsmeow.Client{})
	}
	return instances{clientPointer: clients, whatsmeowService: fake}, fake
}

func TestReconnectWithoutClientStartsInstance(t *testing.T) {
	svc, fake := newReconnectSubject(false)

	if err := svc.Reconnect(&instance_model.Instance{Id: "inst-1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.startCalls != 1 || fake.reconnectCalls != 0 {
		t.Fatalf("start=%d reconnect=%d, want 1 and 0", fake.startCalls, fake.reconnectCalls)
	}
}

func TestReconnectWithDisconnectedClientReconnects(t *testing.T) {
	svc, fake := newReconnectSubject(true)

	if err := svc.Reconnect(&instance_model.Instance{Id: "inst-1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.startCalls != 0 || fake.reconnectCalls != 1 {
		t.Fatalf("start=%d reconnect=%d, want 0 and 1", fake.startCalls, fake.reconnectCalls)
	}
}
