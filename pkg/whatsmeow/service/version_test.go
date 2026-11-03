package whatsmeow_service

import (
	"testing"

	"go.mau.fi/whatsmeow/store"
)

func TestApplyWhatsAppHandshakeVersion(t *testing.T) {
	original := store.GetWAVersion()
	t.Cleanup(func() { store.SetWAVersion(original) })

	applied, ok := applyWhatsAppHandshakeVersion(clientVersion{Major: 2, Minor: 3000, Patch: 1052855845})
	if !ok {
		t.Fatal("expected version to be applied")
	}
	if applied != "2.3000.1052855845" {
		t.Fatalf("applied = %s", applied)
	}
	if got := store.GetWAVersion().String(); got != "2.3000.1052855845" {
		t.Fatalf("handshake version = %s", got)
	}
}

func TestApplyWhatsAppHandshakeVersionIgnoresIncomplete(t *testing.T) {
	original := store.GetWAVersion()
	t.Cleanup(func() { store.SetWAVersion(original) })

	for _, v := range []clientVersion{{}, {Major: 2, Minor: 3000}, {Minor: 3000, Patch: 1}, {Major: 2, Minor: -1, Patch: 1}} {
		if applied, ok := applyWhatsAppHandshakeVersion(v); ok || applied != "" {
			t.Fatalf("%+v: applied = %q ok = %v", v, applied, ok)
		}
		if got := store.GetWAVersion(); got != original {
			t.Fatalf("%+v: library default changed from %s to %s", v, original, got)
		}
	}
}
