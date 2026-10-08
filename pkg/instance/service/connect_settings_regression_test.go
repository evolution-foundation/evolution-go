package instance_service

import (
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	event_types "github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
)

func TestApplyConnectSettingsEventValidation(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		subscribe []string
		want      string
		invalid   bool
	}{
		{"unknown_only", []string{"invalid"}, "", true},
		{"blank_only", []string{" ", ""}, "", true},
		{"deduplicated", []string{" MESSAGE ", "MESSAGE", "invalid", "CONNECTION"}, "MESSAGE,CONNECTION", false},
		{"all_in_any_position", []string{"MESSAGE", "ALL"}, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inst := instance_model.Instance{Events: "CALL", Webhook: "old", RabbitmqEnable: "enabled"}
			before := inst
			updates, err := applyConnectSettings(&inst, &ConnectStruct{Subscribe: tt.subscribe, WebhookUrl: "new"})
			if tt.invalid {
				if !errors.Is(err, ErrInvalidConnectSettings) || !reflect.DeepEqual(inst, before) || len(updates) != 0 {
					t.Fatalf("invalid settings changed state: instance=%#v updates=%#v err=%v", inst, updates, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if inst.Events != strings.Join(event_types.AllEventTypes, ",") {
					t.Fatalf("ALL was not expanded: %q", inst.Events)
				}
			} else if inst.Events != tt.want {
				t.Fatalf("Events=%q, want %q", inst.Events, tt.want)
			}
		})
	}
}

func TestConnectProducerSettingsPreserveOmittedValues(t *testing.T) {
	t.Parallel()
	inst := instance_model.Instance{
		Events: "CALL", Webhook: "https://example.com/hook",
		RabbitmqEnable: "enabled", NatsEnable: "enabled", WebSocketEnable: "enabled",
	}
	before := inst
	updates, err := applyConnectSettings(&inst, &ConnectStruct{})
	if err != nil || len(updates) != 0 || !reflect.DeepEqual(inst, before) {
		t.Fatalf("empty settings changed existing values: %#v, %v", updates, err)
	}
	updates, err = applyConnectSettings(&inst, &ConnectStruct{
		WebhookUrl: "disabled", RabbitmqEnable: "false", NatsEnable: "disabled", WebSocketEnable: "false",
	})
	want := map[string]interface{}{
		"webhook": "disabled", "rabbitmq_enable": "false", "nats_enable": "disabled", "web_socket_enable": "false",
	}
	if err != nil || !reflect.DeepEqual(updates, want) || inst.Events != before.Events {
		t.Fatalf("explicit disabling was not applied: %#v, %v", updates, err)
	}
}

type connectSettingsRepositoryStub struct {
	instance_repository.InstanceRepository
	err     error
	updates map[string]interface{}
}

func (s *connectSettingsRepositoryStub) UpdateConnectSettings(_ string, updates map[string]interface{}) error {
	s.updates = updates
	return s.err
}

type connectSettingsRuntimeStub struct {
	whatsmeow_service.WhatsmeowService
	err        error
	syncCalls  int
	startCalls atomic.Int32
}

func (s *connectSettingsRuntimeStub) UpdateInstanceSettings(string) error {
	s.syncCalls++
	return s.err
}

func (s *connectSettingsRuntimeStub) StartClient(*whatsmeow_service.ClientData) {
	s.startCalls.Add(1)
}

func TestConnectSettingsSnapshotAndFailures(t *testing.T) {
	for _, stage := range []string{"success", "persistence_failure", "runtime_failure"} {
		t.Run(stage, func(t *testing.T) {
			inst := instance_model.Instance{Id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Events: "CALL", RabbitmqEnable: "enabled"}
			before := inst
			failure := errors.New("simulated failure")
			repo := &connectSettingsRepositoryStub{}
			runtime := &connectSettingsRuntimeStub{}
			if stage == "persistence_failure" {
				repo.err = failure
			}
			if stage == "runtime_failure" {
				runtime.err = failure
			}
			loggers := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir(), LogMaxSize: 1})
			t.Cleanup(func() {
				if err := loggers.GetLogger(inst.Id).Close(); err != nil {
					t.Error(err)
				}
			})
			svc := instances{
				instanceRepository: repo, whatsmeowService: runtime, loggerWrapper: loggers,
				clientPointer: map[string]*whatsmeow.Client{inst.Id: {}},
			}
			got, _, _, err := svc.Connect(&ConnectStruct{Subscribe: []string{"MESSAGE"}, RabbitmqEnable: "disabled"}, &inst)
			if !reflect.DeepEqual(inst, before) {
				t.Fatalf("Connect mutated the shared source instance: %#v", inst)
			}
			if stage == "success" {
				if err != nil || got == nil || got == &inst || got.Events != "MESSAGE" || got.RabbitmqEnable != "disabled" {
					t.Fatalf("Connect result=%#v err=%v", got, err)
				}
			} else if !errors.Is(err, failure) || got != nil {
				t.Fatalf("failure was not propagated: instance=%#v err=%v", got, err)
			}
			if stage == "persistence_failure" && runtime.syncCalls != 0 {
				t.Fatal("runtime updated despite persistence failure")
			}
			if runtime.startCalls.Load() != 0 {
				t.Fatal("replaced an already running client")
			}
		})
	}
}
