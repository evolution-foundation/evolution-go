package whatsmeow_service

import (
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"go.mau.fi/whatsmeow/store"
	"google.golang.org/protobuf/proto"
)

func TestConfiguredWhatsAppVersionReachesHandshake(t *testing.T) {
	originalVersion := store.GetWAVersion()
	originalDeviceVersion := proto.CloneOf(store.DeviceProps.Version)
	t.Cleanup(func() {
		store.SetWAVersion(originalVersion)
		store.DeviceProps.Version = originalDeviceVersion
	})

	want := store.WAVersionContainer{2, 3000, 1052769028}
	cfg := &config.Config{
		WhatsappVersionMajor: int(want[0]),
		WhatsappVersionMinor: int(want[1]),
		WhatsappVersionPatch: int(want[2]),
		LogDirectory:         t.TempDir(),
	}
	service := whatsmeowService{config: cfg, loggerWrapper: logger_wrapper.NewLoggerManager(cfg)}
	service.configureWhatsAppVersion("test")

	if got := store.GetWAVersion(); got != want {
		t.Errorf("whatsmeow version = %s, want %s", got, want)
	}
	payloadVersion := store.BaseClientPayload.GetUserAgent().GetAppVersion()
	if !proto.Equal(payloadVersion, want.ProtoAppVersion()) {
		t.Errorf("handshake version = %v, want %s", payloadVersion, want)
	}
}
