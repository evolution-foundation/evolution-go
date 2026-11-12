package whatsmeow_service

import (
	"testing"

	"go.mau.fi/whatsmeow/appstate"
)

func TestReserveAppStateRecoveryDeduplicatesEachStage(t *testing.T) {
	client := &MyClient{}
	name := appstate.WAPatchRegularLow

	if !client.reserveAppStateRecovery(name, false) {
		t.Fatal("first full-sync attempt should be reserved")
	}
	if client.reserveAppStateRecovery(name, false) {
		t.Fatal("duplicate full-sync attempt should be suppressed during cooldown")
	}
	if !client.reserveAppStateRecovery(name, true) {
		t.Fatal("recovery request is a separate stage and should be reserved")
	}
	if client.reserveAppStateRecovery(name, true) {
		t.Fatal("duplicate recovery request should be suppressed during cooldown")
	}

	if !client.reserveAppStateRecovery(appstate.WAPatchRegularHigh, false) {
		t.Fatal("cooldown for one collection must not suppress another collection")
	}
}
