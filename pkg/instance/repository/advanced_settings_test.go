package instance_repository

import (
	"encoding/json"
	"reflect"
	"testing"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

func TestBuildAdvancedSettingsUpdates(t *testing.T) {
	trueVal := true

	t.Run("only AlwaysOnline", func(t *testing.T) {
		updates := buildAdvancedSettingsUpdates(&instance_model.AdvancedSettings{
			AlwaysOnline: &trueVal,
		})
		if len(updates) != 1 {
			t.Fatalf("expected only always_online, got %#v", updates)
		}
		if updates["always_online"] != true {
			t.Fatalf("always_online = %#v, want true", updates["always_online"])
		}
	})

	t.Run("explicit false is written", func(t *testing.T) {
		falseVal := false
		updates := buildAdvancedSettingsUpdates(&instance_model.AdvancedSettings{
			IgnoreGroups: &falseVal,
		})
		if len(updates) != 1 {
			t.Fatalf("expected only ignore_groups, got %#v", updates)
		}
		if updates["ignore_groups"] != false {
			t.Fatalf("ignore_groups = %#v, want false", updates["ignore_groups"])
		}
	})

	t.Run("msgRejectCall non-empty", func(t *testing.T) {
		message := "busy"
		updates := buildAdvancedSettingsUpdates(&instance_model.AdvancedSettings{
			MsgRejectCall: &message,
		})
		if updates["msg_reject_call"] != "busy" {
			t.Fatalf("msg_reject_call = %#v, want busy", updates["msg_reject_call"])
		}
	})

	t.Run("nil settings", func(t *testing.T) {
		updates := buildAdvancedSettingsUpdates(nil)
		if len(updates) != 0 {
			t.Fatalf("expected empty map, got %#v", updates)
		}
	})
}

func TestAdvancedSettingsJSONPresence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, body string
		want       map[string]interface{}
	}{
		{"omitted", "{}", map[string]interface{}{}},
		{"null_fields", `{"alwaysOnline":null,"msgRejectCall":null}`, map[string]interface{}{}},
		{"clear_message", `{"msgRejectCall":""}`, map[string]interface{}{"msg_reject_call": ""}},
		{"false_and_empty", `{"alwaysOnline":false,"msgRejectCall":""}`, map[string]interface{}{"always_online": false, "msg_reject_call": ""}},
		{"all_false", `{"alwaysOnline":false,"rejectCall":false,"readMessages":false,"ignoreGroups":false,"ignoreStatus":false}`, map[string]interface{}{
			"always_online": false, "reject_call": false, "read_messages": false, "ignore_groups": false, "ignore_status": false,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var settings instance_model.AdvancedSettings
			if err := json.Unmarshal([]byte(tt.body), &settings); err != nil {
				t.Fatal(err)
			}
			if got := buildAdvancedSettingsUpdates(&settings); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("updates = %#v, want %#v", got, tt.want)
			}
		})
	}
}
