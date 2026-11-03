package instance_service

import (
	"encoding/json"
	"testing"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	event_types "github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
)

func savedInstance() *instance_model.Instance {
	return &instance_model.Instance{
		Id:              "i1",
		Events:          "MESSAGE,CONNECTION",
		Webhook:         "https://hook.example/old",
		RabbitmqEnable:  "enabled",
		NatsEnable:      "disabled",
		WebSocketEnable: "enabled",
	}
}

func decodeConnect(t *testing.T, body string) *ConnectStruct {
	t.Helper()
	var data *ConnectStruct
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatalf("unmarshal %s: %v", body, err)
	}
	return data
}

func TestConnectPartialBodyPreservesSettings(t *testing.T) {
	for _, body := range []string{`{}`, `{"immediate":true}`, `null`} {
		inst := savedInstance()
		events, _ := applyConnectSettings(decodeConnect(t, body), inst)

		if events != "MESSAGE,CONNECTION" || inst.Events != events {
			t.Errorf("%s: events = %q, want saved subscriptions", body, inst.Events)
		}
		if inst.Webhook != "https://hook.example/old" {
			t.Errorf("%s: webhook = %q, want preserved", body, inst.Webhook)
		}
		if inst.RabbitmqEnable != "enabled" || inst.NatsEnable != "disabled" || inst.WebSocketEnable != "enabled" {
			t.Errorf("%s: producers changed: %+v", body, inst)
		}
	}
}

func TestConnectExplicitFieldsAreApplied(t *testing.T) {
	inst := savedInstance()
	data := decodeConnect(t, `{"webhookUrl":"","subscribe":["GROUP","NOPE"],"rabbitmqEnable":"disabled"}`)
	events, discarded := applyConnectSettings(data, inst)

	if inst.Webhook != "" {
		t.Errorf("explicit empty webhookUrl should clear it, got %q", inst.Webhook)
	}
	if events != event_types.GROUP {
		t.Errorf("events = %q, want %q", events, event_types.GROUP)
	}
	if len(discarded) != 1 || discarded[0] != "NOPE" {
		t.Errorf("discarded = %v, want [NOPE]", discarded)
	}
	if inst.RabbitmqEnable != "disabled" {
		t.Errorf("rabbitmqEnable = %q, want disabled", inst.RabbitmqEnable)
	}
	if inst.NatsEnable != "disabled" || inst.WebSocketEnable != "enabled" {
		t.Errorf("omitted producers changed: %+v", inst)
	}
}

func TestConnectDefaultsForNewInstance(t *testing.T) {
	inst := &instance_model.Instance{Id: "new"}
	events, _ := applyConnectSettings(decodeConnect(t, `{}`), inst)
	if events != event_types.MESSAGE {
		t.Errorf("new instance events = %q, want %q", events, event_types.MESSAGE)
	}
}

func TestConnectStructBuiltInGoAppliesAllFields(t *testing.T) {
	inst := savedInstance()
	applyConnectSettings(&ConnectStruct{WebhookUrl: "https://new"}, inst)
	if inst.Webhook != "https://new" || inst.RabbitmqEnable != "" {
		t.Errorf("Go-built struct should keep legacy overwrite semantics, got %+v", inst)
	}
	if inst.Events != event_types.MESSAGE {
		t.Errorf("Go-built struct with no Subscribe should default to MESSAGE, got %q", inst.Events)
	}
}
