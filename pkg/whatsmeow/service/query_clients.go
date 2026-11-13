package whatsmeow_service

import (
	"sync"

	"go.mau.fi/whatsmeow"
)

func newQueryClientIndex(initial map[string]*whatsmeow.Client) *sync.Map {
	// Construction happens before session workers start. Do not read the legacy
	// map again from HTTP queries; writers maintain the owned index below.
	index := &sync.Map{}
	for id, client := range initial {
		if client != nil {
			index.Store(id, client)
		}
	}
	return index
}

// GetClient returns the session registered for an instance without reading the
// legacy shared map. The query index is owned by StartClient and cache cleanup;
// callers must still check connection and login before using the client.
func (w whatsmeowService) GetClient(instanceID string) *whatsmeow.Client {
	if w.queryClients == nil {
		return nil
	}
	client, ok := w.queryClients.Load(instanceID)
	if !ok {
		return nil
	}
	return client.(*whatsmeow.Client)
}

func (w whatsmeowService) registerQueryClient(instanceID string, client *whatsmeow.Client) {
	w.queryClients.Store(instanceID, client)
}

func (w whatsmeowService) removeQueryClient(instanceID string, client *whatsmeow.Client) {
	// Cleanup from an old session must not remove a replacement client.
	w.queryClients.CompareAndDelete(instanceID, client)
}
