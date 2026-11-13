package whatsmeow_service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/passkey/ceremony"
	poll_service "github.com/evolution-foundation/evolution-go/pkg/poll/service"
)

type fakeLifecycleService struct {
	reconnects atomic.Int32
	webhooks   atomic.Int32
	webhook    chan struct{}
}

func (f *fakeLifecycleService) ConnectOnStartup(string)                     {}
func (f *fakeLifecycleService) StartInstance(string) error                  { return nil }
func (f *fakeLifecycleService) ReconnectClient(string) error                { f.reconnects.Add(1); return nil }
func (f *fakeLifecycleService) StopInstance(string, string, bool) error     { return nil }
func (f *fakeLifecycleService) ClearInstanceCache(string, string) error     { return nil }
func (f *fakeLifecycleService) SendToGlobalQueues(string, []byte, string)   {}
func (f *fakeLifecycleService) ForceUpdateJid(string, string) error         { return nil }
func (f *fakeLifecycleService) UpdateInstanceSettings(string) error         { return nil }
func (f *fakeLifecycleService) UpdateInstanceAdvancedSettings(string) error { return nil }
func (f *fakeLifecycleService) GetPollService() poll_service.PollService    { return nil }
func (f *fakeLifecycleService) PasskeyCeremonyStore() *ceremony.Store       { return nil }
func (f *fakeLifecycleService) SubmitPasskeyResponse(string, *types.WebAuthnResponse) error {
	return nil
}
func (f *fakeLifecycleService) ConfirmPasskey(string) error        { return nil }
func (f *fakeLifecycleService) GetClient(string) *whatsmeow.Client { return nil }
func (f *fakeLifecycleService) IsCurrentClient(string, *whatsmeow.Client) bool {
	return true
}
func (f *fakeLifecycleService) SignalStop(string) {}
func (f *fakeLifecycleService) CallWebhook(*instance_model.Instance, string, []byte) {
	f.webhooks.Add(1)
	select {
	case f.webhook <- struct{}{}:
	default:
	}
}

type fakeInstanceRepository struct {
	connected    atomic.Bool
	qrcodeWrites atomic.Int32
	instance     *instance_model.Instance
}

func (f *fakeInstanceRepository) Create(instance_model.Instance) (*instance_model.Instance, error) {
	return nil, nil
}
func (f *fakeInstanceRepository) GetInstanceByID(string) (*instance_model.Instance, error) {
	if f.instance == nil {
		return nil, nil
	}
	copy := *f.instance
	return &copy, nil
}
func (f *fakeInstanceRepository) GetConnectedInstanceByID(string) (*instance_model.Instance, error) {
	return nil, nil
}
func (f *fakeInstanceRepository) GetInstanceByToken(string) (*instance_model.Instance, error) {
	return nil, nil
}
func (f *fakeInstanceRepository) GetInstanceByName(string) (*instance_model.Instance, error) {
	return nil, nil
}
func (f *fakeInstanceRepository) Update(*instance_model.Instance) error { return nil }
func (f *fakeInstanceRepository) UpdateConnected(_ string, status bool, _ string) error {
	f.connected.Store(status)
	return nil
}
func (f *fakeInstanceRepository) UpdateQrcode(string, string) error {
	f.qrcodeWrites.Add(1)
	return nil
}
func (f *fakeInstanceRepository) UpdateProxy(string, string) error { return nil }
func (f *fakeInstanceRepository) UpdateJid(string, string) error   { return nil }
func (f *fakeInstanceRepository) GetAllConnectedInstances() ([]*instance_model.Instance, error) {
	return nil, nil
}
func (f *fakeInstanceRepository) GetAllConnectedInstancesByClientName(string) ([]*instance_model.Instance, error) {
	return nil, nil
}
func (f *fakeInstanceRepository) GetAll(string) ([]*instance_model.Instance, error) {
	return nil, nil
}
func (f *fakeInstanceRepository) Delete(string) error { return nil }
func (f *fakeInstanceRepository) GetAdvancedSettings(string) (*instance_model.AdvancedSettings, error) {
	return nil, nil
}
func (f *fakeInstanceRepository) UpdateAdvancedSettings(string, *instance_model.AdvancedSettings) error {
	return nil
}

func TestDisconnectedKeepsNativeWhatsmeowLifecycle(t *testing.T) {
	source, err := os.ReadFile("whatsmeow.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(source)
	if !strings.Contains(code, "client.EnableAutoReconnect = true") {
		t.Fatal("WhatsMeow native reconnect must remain enabled")
	}

	start := strings.Index(code, "case *events.Disconnected:")
	end := strings.Index(code[start:], "case *events.LabelEdit:")
	if start < 0 || end < 0 {
		t.Fatal("Disconnected event branch not found")
	}
	disconnectedBranch := code[start : start+end]
	if strings.Contains(disconnectedBranch, "ReconnectClient") {
		t.Fatal("Disconnected event must not create a second client lifecycle")
	}
}

func TestProductionDisconnectedEventUsesNativeReconnectWithoutNewGeneration(t *testing.T) {
	repo := &fakeInstanceRepository{}
	repo.connected.Store(true)
	service := &fakeLifecycleService{webhook: make(chan struct{}, 1)}
	instanceID := "instance-production-trace"
	myClient := &MyClient{
		service:            service,
		userID:             instanceID,
		token:              "redacted-production-token",
		Instance:           &instance_model.Instance{Id: instanceID, Name: "production", Token: "redacted-production-token"},
		instanceRepository: repo,
		userInfoCache:      cache.New(time.Minute, time.Minute),
		config:             &config.Config{},
		loggerWrapper: logger_wrapper.NewLoggerManager(&config.Config{
			LogDirectory: t.TempDir(),
		}),
	}

	myClient.myEventHandler(&events.Disconnected{})
	if service.reconnects.Load() != 0 {
		t.Fatalf("Disconnected spawned %d manual client generations", service.reconnects.Load())
	}
	if repo.connected.Load() {
		t.Fatal("Disconnected did not persist the transient offline state")
	}
	select {
	case <-service.webhook:
	case <-time.After(time.Second):
		t.Fatal("Disconnected event was not forwarded to the existing webhook pipeline")
	}
}

func TestSetClientAndClearInstanceCacheAreRaceSafe(t *testing.T) {
	logDir := t.TempDir()
	service := whatsmeowService{
		killChannel:     make(map[string]chan bool),
		clientPointer:   make(map[string]*whatsmeow.Client),
		myClientPointer: make(map[string]*MyClient),
		userInfoCache:   cache.New(time.Minute, time.Minute),
		loggerWrapper: logger_wrapper.NewLoggerManager(&config.Config{
			LogDirectory: logDir,
		}),
		lifecycleMu:    &sync.Mutex{},
		lifecycleLocks: make(map[string]*sync.Mutex),
		starting:       make(map[string]bool),
		generations:    make(map[string]*lifecycleGeneration),
	}

	const instanceID = "instance-production-trace"
	const secretToken = "must-not-appear-in-logs"
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			service.setClient(instanceID, nil)
			service.setMyClient(instanceID, nil)
		}()
		go func() {
			defer wg.Done()
			if err := service.ClearInstanceCache(instanceID, secretToken); err != nil {
				t.Errorf("ClearInstanceCache() error = %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			_ = service.GetClient(instanceID)
			service.lifecycleMu.Lock()
			generation := service.generations[instanceID]
			service.lifecycleMu.Unlock()
			service.signalStopGeneration(instanceID, generation)
		}()
	}
	wg.Wait()

	logData, err := os.ReadFile(filepath.Join(logDir, instanceID, "instance.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logData), secretToken) {
		t.Fatal("ClearInstanceCache logged the instance token")
	}
}

func TestDuplicateReconnectsShareOneInstanceLock(t *testing.T) {
	service := whatsmeowService{
		lifecycleMu:    &sync.Mutex{},
		lifecycleLocks: make(map[string]*sync.Mutex),
	}

	first := service.lifecycleLock("instance-production-trace")
	second := service.lifecycleLock("instance-production-trace")
	if first != second {
		t.Fatal("duplicate reconnects received different lifecycle locks")
	}

	first.Lock()
	entered := make(chan struct{})
	go func() {
		second.Lock()
		close(entered)
		second.Unlock()
	}()

	select {
	case <-entered:
		t.Fatal("duplicate reconnect entered while the first generation was active")
	case <-time.After(25 * time.Millisecond):
	}
	first.Unlock()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("duplicate reconnect did not continue after lifecycle release")
	}
}

func TestCurrentClientIdentityRejectsStaleGeneration(t *testing.T) {
	service := whatsmeowService{
		clientPointer: make(map[string]*whatsmeow.Client),
		lifecycleMu:   &sync.Mutex{},
	}
	current := &whatsmeow.Client{}
	stale := &whatsmeow.Client{}
	service.setClient("instance-production-trace", current)

	if !service.IsCurrentClient("instance-production-trace", current) {
		t.Fatal("current client generation was rejected")
	}
	if service.IsCurrentClient("instance-production-trace", stale) {
		t.Fatal("stale client generation was accepted")
	}
}

func TestSignalStopBroadcastsToAllLifecycleGoroutines(t *testing.T) {
	service := whatsmeowService{
		killChannel:    make(map[string]chan bool),
		lifecycleMu:    &sync.Mutex{},
		lifecycleLocks: make(map[string]*sync.Mutex),
		starting:       make(map[string]bool),
		generations:    make(map[string]*lifecycleGeneration),
	}
	generation := service.newGeneration("instance-production-trace")
	done := make(chan struct{}, 2)
	for range 2 {
		go func() {
			<-generation.ctx.Done()
			done <- struct{}{}
		}()
	}

	service.signalStopGeneration("instance-production-trace", generation)
	service.signalStopGeneration("instance-production-trace", generation)
	for range 2 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("stop signal did not reach every lifecycle goroutine")
		}
	}

}

func TestStartThenImmediateStopLeavesNoLateLifecycle(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previousProcs)

	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "dbdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	const instanceID = "instance-production-trace"
	const token = "redacted-production-token"
	repo := &fakeInstanceRepository{instance: &instance_model.Instance{
		Id:         instanceID,
		Name:       "production",
		Token:      token,
		Events:     "MESSAGES_UPSERT",
		OsName:     "Linux",
		ClientName: "evolution",
	}}
	service := whatsmeowService{
		instanceRepository: repo,
		authStore:          &authStoreState{},
		config: &config.Config{
			WhatsappVersionMajor: 2,
			WhatsappVersionMinor: 3000,
			WhatsappVersionPatch: 1053070940,
			LogDirectory:         filepath.Join(baseDir, "logs"),
		},
		killChannel:       make(map[string]chan bool),
		clientPointer:     make(map[string]*whatsmeow.Client),
		myClientPointer:   make(map[string]*MyClient),
		userInfoCache:     cache.New(time.Minute, time.Minute),
		processedMessages: cache.New(time.Minute, time.Minute),
		exPath:            baseDir,
		loggerWrapper: logger_wrapper.NewLoggerManager(&config.Config{
			LogDirectory: filepath.Join(baseDir, "logs"),
		}),
		lifecycleMu:    &sync.Mutex{},
		lifecycleLocks: make(map[string]*sync.Mutex),
		starting:       make(map[string]bool),
		generations:    make(map[string]*lifecycleGeneration),
	}
	t.Cleanup(func() {
		if service.authStore.container != nil {
			_ = service.authStore.container.Close()
		}
	})

	for iteration := 0; iteration < 100; iteration++ {
		if err := service.StartInstance(instanceID); err != nil {
			t.Fatalf("iteration %d StartInstance: %v", iteration, err)
		}
		service.lifecycleMu.Lock()
		generation := service.generations[instanceID]
		service.lifecycleMu.Unlock()
		if generation == nil {
			t.Fatalf("iteration %d did not register generation synchronously", iteration)
		}

		if err := service.StopInstance(instanceID, token, false); err != nil {
			t.Fatalf("iteration %d StopInstance: %v", iteration, err)
		}
		select {
		case <-generation.done:
		default:
			t.Fatalf("iteration %d stop returned before lifecycle acknowledgement", iteration)
		}

		service.lifecycleMu.Lock()
		_, hasGeneration := service.generations[instanceID]
		_, hasKill := service.killChannel[instanceID]
		_, hasClient := service.clientPointer[instanceID]
		_, hasHandler := service.myClientPointer[instanceID]
		_, isStarting := service.starting[instanceID]
		service.lifecycleMu.Unlock()
		if hasGeneration || hasKill || hasClient || hasHandler || isStarting {
			t.Fatalf("iteration %d late lifecycle survived stop: generation=%v kill=%v client=%v handler=%v starting=%v", iteration, hasGeneration, hasKill, hasClient, hasHandler, isStarting)
		}
	}
}

func TestStaleQRWorkerCannotPublishOrStopNextGeneration(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previousProcs)

	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "dbdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	const instanceID = "instance-production-trace"
	const token = "redacted-production-token"
	repo := &fakeInstanceRepository{instance: &instance_model.Instance{
		Id:         instanceID,
		Name:       "production",
		Token:      token,
		Events:     "MESSAGE",
		OsName:     "Linux",
		ClientName: "evolution",
	}}
	service := whatsmeowService{
		instanceRepository: repo,
		authStore:          &authStoreState{},
		config: &config.Config{
			WhatsappVersionMajor: 2,
			WhatsappVersionMinor: 3000,
			WhatsappVersionPatch: 1053070940,
			LogDirectory:         filepath.Join(baseDir, "logs"),
		},
		killChannel:       make(map[string]chan bool),
		clientPointer:     make(map[string]*whatsmeow.Client),
		myClientPointer:   make(map[string]*MyClient),
		userInfoCache:     cache.New(time.Minute, time.Minute),
		processedMessages: cache.New(time.Minute, time.Minute),
		exPath:            baseDir,
		loggerWrapper: logger_wrapper.NewLoggerManager(&config.Config{
			LogDirectory: filepath.Join(baseDir, "logs"),
		}),
		lifecycleMu:    &sync.Mutex{},
		lifecycleLocks: make(map[string]*sync.Mutex),
		starting:       make(map[string]bool),
		generations:    make(map[string]*lifecycleGeneration),
	}
	t.Cleanup(func() {
		if service.authStore.container != nil {
			_ = service.authStore.container.Close()
		}
	})

	for iteration := 0; iteration < 100; iteration++ {
		repo.qrcodeWrites.Store(0)
		outbound := &fakeLifecycleService{webhook: make(chan struct{}, 1)}
		generationA := service.newGeneration(instanceID)
		clientA := whatsmeow.NewClient(&store.Device{}, nil)
		entered := make(chan struct{})
		release := make(chan struct{})
		myClientA := &MyClient{
			service:            outbound,
			WAClient:           clientA,
			userID:             instanceID,
			token:              token,
			Instance:           repo.instance,
			instanceRepository: repo,
			config:             service.config,
			loggerWrapper:      service.loggerWrapper,
			generation:         generationA,
			lifecycle:          &service,
			killSignal:         generationA.ctx.Done(),
			qrBeforePublish: func(ctx context.Context) bool {
				close(entered)
				<-release
				return ctx.Err() == nil
			},
		}
		if !service.publishClient(instanceID, generationA, clientA) || !service.publishMyClient(instanceID, generationA, myClientA) {
			t.Fatalf("iteration %d failed to publish generation A", iteration)
		}
		go func() {
			<-generationA.ctx.Done()
			service.finishGeneration(instanceID, generationA, clientA, myClientA)
		}()
		myClientA.handleQRCodes([]string{"stale-qr-must-not-publish"})
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatalf("iteration %d QR worker did not pause", iteration)
		}

		stopDone := make(chan error, 1)
		go func() { stopDone <- service.StopInstance(instanceID, token, false) }()
		select {
		case <-generationA.ctx.Done():
		case <-time.After(time.Second):
			t.Fatalf("iteration %d stop did not cancel generation A", iteration)
		}
		startDone := make(chan error, 1)
		go func() { startDone <- service.StartInstance(instanceID) }()
		select {
		case err := <-startDone:
			t.Fatalf("iteration %d generation B started before A acknowledged stop: %v", iteration, err)
		case <-time.After(time.Millisecond):
		}

		close(release)
		if err := <-stopDone; err != nil {
			t.Fatalf("iteration %d stop generation A: %v", iteration, err)
		}
		if err := <-startDone; err != nil {
			t.Fatalf("iteration %d start generation B: %v", iteration, err)
		}
		service.lifecycleMu.Lock()
		generationB := service.generations[instanceID]
		service.lifecycleMu.Unlock()
		if generationB == nil || generationB == generationA || generationB.ctx.Err() != nil {
			t.Fatalf("iteration %d generation B is not active", iteration)
		}
		if repo.qrcodeWrites.Load() != 0 || outbound.webhooks.Load() != 0 {
			t.Fatalf("iteration %d stale generation A published QR state: db=%d webhook=%d", iteration, repo.qrcodeWrites.Load(), outbound.webhooks.Load())
		}
		if err := service.StopInstance(instanceID, token, false); err != nil {
			t.Fatalf("iteration %d stop generation B: %v", iteration, err)
		}
	}
}
