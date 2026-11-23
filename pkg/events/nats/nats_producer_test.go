package nats_producer

import "testing"

func TestNewNatsProducerDoesNotConnectWithoutURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"Empty URL", ""},
		{"Whitespace URL", "  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			producer, ok := NewNatsProducer(tt.url, true, []string{"event1"}, nil).(*natsProducer)
			if !ok {
				t.Fatal("constructor returned an unexpected producer type")
			}
			if producer.conn != nil {
				t.Fatal("NATS connection must stay nil when NATS_URL is empty")
			}
			if producer.natsGlobalEnabled {
				t.Fatal("NATS global publishing must stay disabled without a URL")
			}
			if producer.natsGlobalEvents != nil {
				t.Fatal("natsGlobalEvents must be nil when NATS_URL is empty")
			}
		})
	}
}
