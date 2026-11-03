package whatsmeow_service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

func TestIntegrationDynamicVersionReachesRealWhatsmeowQR(t *testing.T) {
	if os.Getenv("EVOLUTION_GO_WA_INTEGRATION") != "1" {
		t.Skip("set EVOLUTION_GO_WA_INTEGRATION=1 to connect to WhatsApp and await a real QR event")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dynamicVersion, err := fetchWhatsAppWebVersion()
	if err != nil {
		t.Fatalf("fetch dynamic WhatsApp Web version: %v", err)
	}
	originalVersion := store.GetWAVersion()
	originalDeviceVersion := [3]uint32{
		store.DeviceProps.Version.GetPrimary(),
		store.DeviceProps.Version.GetSecondary(),
		store.DeviceProps.Version.GetTertiary(),
	}
	t.Cleanup(func() {
		waVersionApplyMu.Lock()
		defer waVersionApplyMu.Unlock()
		store.SetWAVersion(originalVersion)
		store.DeviceProps.Version.Primary = proto.Uint32(originalDeviceVersion[0])
		store.DeviceProps.Version.Secondary = proto.Uint32(originalDeviceVersion[1])
		store.DeviceProps.Version.Tertiary = proto.Uint32(originalDeviceVersion[2])
	})
	applyWhatsAppVersion(*dynamicVersion)

	want := store.WAVersionContainer{
		uint32(dynamicVersion.Major),
		uint32(dynamicVersion.Minor),
		uint32(dynamicVersion.Patch),
	}
	if got := store.GetWAVersion(); got != want {
		t.Fatalf("applied version %s differs from fetched version %s", got.String(), want.String())
	}
	payloadVersion := store.BaseClientPayload.GetUserAgent().GetAppVersion()
	if got := [3]uint32{payloadVersion.GetPrimary(), payloadVersion.GetSecondary(), payloadVersion.GetTertiary()}; got != want {
		t.Fatalf("handshake payload version %v differs from fetched version %v", got, want)
	}

	dbPath := filepath.Join(t.TempDir(), "wa-integration.db")
	container, err := sqlstore.New(ctx, "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)", nil)
	if err != nil {
		t.Fatalf("create real whatsmeow store: %v", err)
	}
	t.Cleanup(func() { _ = container.Close() })

	client := whatsmeow.NewClient(container.NewDevice(), nil)
	qrEvents, err := client.GetQRChannel(ctx)
	if err != nil {
		t.Fatalf("create QR event channel: %v", err)
	}
	if err = client.Connect(); err != nil {
		t.Fatalf("connect real whatsmeow client: %v", err)
	}
	t.Cleanup(client.Disconnect)

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for real QR event: %v", ctx.Err())
		case event, ok := <-qrEvents:
			if !ok {
				t.Fatal("QR event channel closed before a QR code was received")
			}
			switch event.Event {
			case whatsmeow.QRChannelEventCode:
				t.Logf("real QR event received; QR payload redacted; fetched/applied version=%s", want.String())
				return
			case whatsmeow.QRChannelClientOutdated.Event:
				t.Fatalf("WhatsApp rejected fetched/applied version %s with ClientOutdated (405)", want.String())
			case whatsmeow.QRChannelEventError:
				t.Fatalf("WhatsApp QR flow failed before QR: %v", event.Error)
			case whatsmeow.QRChannelTimeout.Event:
				t.Fatal("WhatsApp disconnected before producing a QR event")
			}
		}
	}
}
