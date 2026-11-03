package whatsmeow_service

import (
	"testing"

	"go.mau.fi/whatsmeow/store"
)

func TestSetWAVersionUpdatesBaseClientPayload(t *testing.T) {
	testVersion := store.WAVersionContainer{2, 3000, 1052812464}
	store.SetWAVersion(testVersion)

	gotVersion := store.GetWAVersion()
	if gotVersion != testVersion {
		t.Fatalf("expected waVersion to be %v, got %v", testVersion, gotVersion)
	}

	appVersion := store.BaseClientPayload.UserAgent.AppVersion
	if appVersion == nil {
		t.Fatal("expected BaseClientPayload.UserAgent.AppVersion to not be nil")
	}

	if *appVersion.Primary != 2 || *appVersion.Secondary != 3000 || *appVersion.Tertiary != 1052812464 {
		t.Fatalf("expected payload version 2.3000.1052812464, got %d.%d.%d",
			*appVersion.Primary, *appVersion.Secondary, *appVersion.Tertiary)
	}
}
