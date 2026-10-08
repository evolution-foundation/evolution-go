package instance_service

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	event_types "github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
)

// ErrInvalidConnectSettings identifies invalid input without mutating settings.
var ErrInvalidConnectSettings = errors.New("invalid connect settings")

// applyConnectSettings mutates instance only for fields explicitly provided.
// Empty subscribe keeps existing Events; defaults to MESSAGE only when Events is empty.
// Empty producer strings keep existing values; send "disabled" or "false" to turn off.
func applyConnectSettings(instance *instance_model.Instance, data *ConnectStruct) (map[string]interface{}, error) {
	updates := map[string]interface{}{}
	if instance == nil || data == nil {
		return updates, ErrInvalidConnectSettings
	}

	if len(data.Subscribe) > 0 {
		var subscribedEvents []string
		for _, arg := range data.Subscribe {
			arg = strings.TrimSpace(arg)
			if arg == event_types.ALL {
				subscribedEvents = append([]string(nil), event_types.AllEventTypes...)
				break
			}
			if event_types.IsEventType(arg) && !slices.Contains(subscribedEvents, arg) {
				subscribedEvents = append(subscribedEvents, arg)
			}
		}
		if len(subscribedEvents) == 0 {
			return nil, fmt.Errorf("%w: subscribe must contain a valid event or ALL", ErrInvalidConnectSettings)
		}
		eventString := strings.Join(subscribedEvents, ",")
		instance.Events = eventString
		updates["events"] = eventString
	} else if instance.Events == "" {
		instance.Events = event_types.MESSAGE
		updates["events"] = event_types.MESSAGE
	}

	if data.WebhookUrl != "" {
		instance.Webhook = data.WebhookUrl
		updates["webhook"] = data.WebhookUrl
	}
	if data.RabbitmqEnable != "" {
		instance.RabbitmqEnable = data.RabbitmqEnable
		updates["rabbitmq_enable"] = data.RabbitmqEnable
	}
	if data.NatsEnable != "" {
		instance.NatsEnable = data.NatsEnable
		updates["nats_enable"] = data.NatsEnable
	}
	if data.WebSocketEnable != "" {
		instance.WebSocketEnable = data.WebSocketEnable
		updates["web_socket_enable"] = data.WebSocketEnable
	}

	return updates, nil
}

func splitSubscribedEvents(events string) []string {
	if events == "" {
		return []string{event_types.MESSAGE}
	}
	parts := strings.Split(events, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{event_types.MESSAGE}
	}
	return out
}
