package whatsmeow_service

import "testing"

func TestIsConnectionEvent(t *testing.T) {
	for _, name := range []string{"Connected", "PairSuccess", "TemporaryBan", "LoggedOut", "ConnectFailure", "Disconnected", "ClientOutdated", "StreamReplaced"} {
		if !isConnectionEvent(name) {
			t.Errorf("isConnectionEvent(%q) = false, want true", name)
		}
	}
	if isConnectionEvent("Message") {
		t.Error("isConnectionEvent(\"Message\") = true, want false")
	}
}
