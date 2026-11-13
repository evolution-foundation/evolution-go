package instance_service

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/passkey/ceremony"
	poll_service "github.com/evolution-foundation/evolution-go/pkg/poll/service"
)

type lifecycleRouteRepository struct {
	mu       sync.Mutex
	instance instance_model.Instance
}

func (r *lifecycleRouteRepository) Create(instance instance_model.Instance) (*instance_model.Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instance = instance
	return &r.instance, nil
}
func (r *lifecycleRouteRepository) GetInstanceByID(string) (*instance_model.Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := r.instance
	return &copy, nil
}
func (r *lifecycleRouteRepository) GetConnectedInstanceByID(string) (*instance_model.Instance, error) {
	return r.GetInstanceByID("")
}
func (r *lifecycleRouteRepository) GetInstanceByToken(string) (*instance_model.Instance, error) {
	return r.GetInstanceByID("")
}
func (r *lifecycleRouteRepository) GetInstanceByName(string) (*instance_model.Instance, error) {
	return r.GetInstanceByID("")
}
func (r *lifecycleRouteRepository) Update(instance *instance_model.Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instance = *instance
	return nil
}
func (r *lifecycleRouteRepository) UpdateConnected(string, bool, string) error { return nil }
func (r *lifecycleRouteRepository) UpdateQrcode(string, string) error          { return nil }
func (r *lifecycleRouteRepository) UpdateProxy(string, string) error           { return nil }
func (r *lifecycleRouteRepository) UpdateJid(string, string) error             { return nil }
func (r *lifecycleRouteRepository) GetAllConnectedInstances() ([]*instance_model.Instance, error) {
	return nil, nil
}
func (r *lifecycleRouteRepository) GetAllConnectedInstancesByClientName(string) ([]*instance_model.Instance, error) {
	return nil, nil
}
func (r *lifecycleRouteRepository) GetAll(string) ([]*instance_model.Instance, error) {
	return nil, nil
}
func (r *lifecycleRouteRepository) Delete(string) error { return nil }
func (r *lifecycleRouteRepository) GetAdvancedSettings(string) (*instance_model.AdvancedSettings, error) {
	return nil, nil
}
func (r *lifecycleRouteRepository) UpdateAdvancedSettings(string, *instance_model.AdvancedSettings) error {
	return nil
}

type lifecycleRouteOwner struct {
	opMu        sync.Mutex
	active      atomic.Int32
	maxActive   atomic.Int32
	handlers    atomic.Int32
	maxHandlers atomic.Int32
	starts      atomic.Int32
}

func (o *lifecycleRouteOwner) run(start, stop bool) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	active := o.active.Add(1)
	if active > o.maxActive.Load() {
		o.maxActive.Store(active)
	}
	defer o.active.Add(-1)

	if start {
		o.starts.Add(1)
		if o.handlers.Load() == 0 {
			o.handlers.Store(1)
		}
		if o.handlers.Load() > o.maxHandlers.Load() {
			o.maxHandlers.Store(o.handlers.Load())
		}
	}
	if stop {
		o.handlers.Store(0)
	}
	time.Sleep(time.Millisecond)
}

func (o *lifecycleRouteOwner) ConnectOnStartup(string) {}
func (o *lifecycleRouteOwner) StartInstance(string) error {
	o.run(true, false)
	return nil
}
func (o *lifecycleRouteOwner) ReconnectClient(string) error {
	o.run(false, false)
	return errors.New("forced reconnect stop after owner operation")
}
func (o *lifecycleRouteOwner) StopInstance(string, string, bool) error {
	o.run(false, true)
	return nil
}
func (o *lifecycleRouteOwner) ClearInstanceCache(string, string) error              { return nil }
func (o *lifecycleRouteOwner) CallWebhook(*instance_model.Instance, string, []byte) {}
func (o *lifecycleRouteOwner) SendToGlobalQueues(string, []byte, string)            {}
func (o *lifecycleRouteOwner) ForceUpdateJid(string, string) error                  { return nil }
func (o *lifecycleRouteOwner) UpdateInstanceSettings(string) error {
	return errors.New("not running")
}
func (o *lifecycleRouteOwner) UpdateInstanceAdvancedSettings(string) error { return nil }
func (o *lifecycleRouteOwner) GetPollService() poll_service.PollService    { return nil }
func (o *lifecycleRouteOwner) PasskeyCeremonyStore() *ceremony.Store       { return nil }
func (o *lifecycleRouteOwner) SubmitPasskeyResponse(string, *types.WebAuthnResponse) error {
	return nil
}
func (o *lifecycleRouteOwner) ConfirmPasskey(string) error        { return nil }
func (o *lifecycleRouteOwner) GetClient(string) *whatsmeow.Client { return nil }
func (o *lifecycleRouteOwner) IsCurrentClient(string, *whatsmeow.Client) bool {
	return true
}
func (o *lifecycleRouteOwner) SignalStop(string) {}

func TestConcurrentLifecycleRoutesUseSingleOwner(t *testing.T) {
	instance := instance_model.Instance{Id: "instance-production-trace", Token: "redacted-token"}
	repository := &lifecycleRouteRepository{instance: instance}
	owner := &lifecycleRouteOwner{}
	service := instances{
		instanceRepository: repository,
		config:             &config.Config{},
		whatsmeowService:   owner,
		loggerWrapper: logger_wrapper.NewLoggerManager(&config.Config{
			LogDirectory: t.TempDir(),
		}),
	}

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		instanceCopy := instance
		_, _, _, _ = service.Connect(&ConnectStruct{}, &instanceCopy)
	}()
	go func() {
		defer wg.Done()
		_ = service.ForceReconnect(instance.Id, "redacted-number")
	}()
	go func() {
		defer wg.Done()
		instanceCopy := instance
		_, _ = service.Logout(&instanceCopy)
	}()
	go func() {
		defer wg.Done()
		_ = service.Delete(instance.Id)
	}()
	wg.Wait()

	if owner.maxActive.Load() != 1 {
		t.Fatalf("lifecycle routes overlapped: max active operations = %d", owner.maxActive.Load())
	}
	if owner.starts.Load() != 1 {
		t.Fatalf("created %d client generations, want exactly one", owner.starts.Load())
	}
	if owner.maxHandlers.Load() > 1 || owner.handlers.Load() > 1 {
		t.Fatalf("event handler leak: current=%d max=%d", owner.handlers.Load(), owner.maxHandlers.Load())
	}
}
