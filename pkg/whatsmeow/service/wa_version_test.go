package whatsmeow_service

import (
	"testing"

	"go.mau.fi/whatsmeow/store"
)

func TestApplyWAVersionUpdatesHandshakeVersion(t *testing.T) {
	original := store.GetWAVersion()
	t.Cleanup(func() { store.SetWAVersion(original) })

	applyWAVersion(clientVersion{Major: 2, Minor: 3000, Patch: 1052878631})

	want := store.WAVersionContainer{2, 3000, 1052878631}
	if got := store.GetWAVersion(); got != want {
		t.Fatalf("handshake version = %v, want %v", got, want)
	}
	if got := store.BaseClientPayload.GetUserAgent().GetAppVersion().GetTertiary(); got != 1052878631 {
		t.Fatalf("client payload tertiary = %d, want 1052878631", got)
	}
}

func TestApplyWAVersionIgnoresZero(t *testing.T) {
	original := store.GetWAVersion()
	applyWAVersion(clientVersion{})
	if got := store.GetWAVersion(); got != original {
		t.Fatalf("zero version changed handshake version to %v", got)
	}
}
