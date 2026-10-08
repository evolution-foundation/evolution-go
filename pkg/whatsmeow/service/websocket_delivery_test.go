package whatsmeow_service

import (
	"errors"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
)

type recordedEventDelivery struct {
	queue, payload, destination, owner string
}

type recordingEventProducer struct {
	err        error
	deliveries []recordedEventDelivery
}

func (p *recordingEventProducer) Produce(queue string, payload []byte, destination, owner string) error {
	p.deliveries = append(p.deliveries, recordedEventDelivery{queue, string(payload), destination, owner})
	return p.err
}

func (*recordingEventProducer) CreateGlobalQueues() error { return nil }

func TestWebsocketFailureDoesNotSkipWebhookOrRetry(t *testing.T) {
	for _, flag := range []string{"enabled", "true", "disabled"} {
		for _, failure := range []bool{false, true} {
			name := flag + "/success"
			if failure {
				name = flag + "/failure"
			}
			t.Run(name, func(t *testing.T) {
				ws, webhook := &recordingEventProducer{}, &recordingEventProducer{}
				if failure {
					ws.err = errors.New("partial websocket delivery")
				}
				manager := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir(), LogMaxSize: 1})
				t.Cleanup(func() {
					if err := manager.GetLogger("delivery-instance").Close(); err != nil {
						t.Error(err)
					}
				})
				service := &whatsmeowService{websocketProducer: ws, webhookProducer: webhook, loggerWrapper: manager}
				instance := &instance_model.Instance{
					Id: "delivery-instance", Token: "test-token", WebSocketEnable: flag, Webhook: "http://example.invalid/events",
				}
				service.sendToQueueOrWebhook(instance, "MESSAGE", []byte(`{"event":"message"}`))
				wantWS := 1
				if flag == "disabled" {
					wantWS = 0
				}
				if len(ws.deliveries) != wantWS || len(webhook.deliveries) != 1 {
					t.Fatalf("deliveries: websocket=%d (want %d), webhook=%d (want 1)", len(ws.deliveries), wantWS, len(webhook.deliveries))
				}
				want := recordedEventDelivery{"MESSAGE", `{"event":"message"}`, instance.Webhook, instance.Id}
				if got := webhook.deliveries[0]; got != want {
					t.Fatalf("webhook event = %#v, want %#v", got, want)
				}
				if wantWS != 0 {
					want.destination, want.owner = instance.Id, instance.Token
					if got := ws.deliveries[0]; got != want {
						t.Fatalf("websocket event = %#v, want %#v", got, want)
					}
				}
			})
		}
	}
}
