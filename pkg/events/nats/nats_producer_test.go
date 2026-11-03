package nats_producer

import "testing"

func TestNewNatsProducerSkipsEmptyURL(t *testing.T) {
	for _, url := range []string{"", "   "} {
		p, ok := NewNatsProducer(url, true, []string{"MESSAGE"}, nil).(*natsProducer)
		if !ok {
			t.Fatal("unexpected producer type")
		}
		if p.conn != nil || p.natsGlobalEnabled || p.natsGlobalEvents != nil {
			t.Fatalf("url %q: producer must stay disabled, got %+v", url, p)
		}
	}
}
