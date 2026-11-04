package whatsmeow_service

import (
	"sync"
	"testing"

	"go.mau.fi/whatsmeow/store"
	"google.golang.org/protobuf/proto"
)

func preserveWhatsAppVersionGlobals(t *testing.T) {
	t.Helper()
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
}

func TestApplyDynamicWAVersionUpdatesHandshakeState(t *testing.T) {
	preserveWhatsAppVersionGlobals(t)

	// Exact mismatch captured in the production incident: discovery resolved
	// 1052878631 while the Noise handshake still advertised 1042386815.
	store.SetWAVersion(store.WAVersionContainer{2, 3000, 1042386815})

	dynamicVersion := clientVersion{Major: 2, Minor: 3000, Patch: 1052878631}
	want := store.WAVersionContainer{2, 3000, 1052878631}
	applyWhatsAppVersion(dynamicVersion)

	if got := store.GetWAVersion(); got != want {
		t.Fatalf("store.GetWAVersion() = %s, want dynamically fetched %s", got.String(), want.String())
	}
	payloadVersion := store.BaseClientPayload.GetUserAgent().GetAppVersion()
	if got := [3]uint32{payloadVersion.GetPrimary(), payloadVersion.GetSecondary(), payloadVersion.GetTertiary()}; got != want {
		t.Fatalf("BaseClientPayload UserAgent AppVersion = %v, want %v", got, want)
	}
	deviceVersion := store.DeviceProps.GetVersion()
	if got := [3]uint32{deviceVersion.GetPrimary(), deviceVersion.GetSecondary(), deviceVersion.GetTertiary()}; got != want {
		t.Fatalf("DeviceProps Version = %v, want %v", got, want)
	}
}

func TestApplyInvalidWAVersionLeavesHandshakeStateUnchanged(t *testing.T) {
	preserveWhatsAppVersionGlobals(t)
	originalVersion := store.GetWAVersion()

	applyWhatsAppVersion(clientVersion{})

	if got := store.GetWAVersion(); got != originalVersion {
		t.Fatalf("zero version changed handshake state from %s to %s", originalVersion.String(), got.String())
	}
}

func TestConcurrentWAVersionApplicationIsRaceFree(t *testing.T) {
	preserveWhatsAppVersionGlobals(t)

	version := clientVersion{Major: 2, Minor: 3000, Patch: 1052878631}
	const starts = 16
	var wg sync.WaitGroup
	for range starts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			applyWhatsAppVersion(version)
		}()
	}
	wg.Wait()

	want := store.WAVersionContainer{2, 3000, 1052878631}
	if got := store.GetWAVersion(); got != want {
		t.Fatalf("concurrent application left version %s, want %s", got.String(), want.String())
	}
}
