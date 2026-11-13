package send_service

import "testing"

type fakeConnectionState struct {
	connected bool
	loggedIn  bool
}

func (f fakeConnectionState) IsConnected() bool { return f.connected }
func (f fakeConnectionState) IsLoggedIn() bool  { return f.loggedIn }

func TestClientSessionReadyRequiresConnectedAndLoggedIn(t *testing.T) {
	tests := []struct {
		name      string
		state     fakeConnectionState
		wantReady bool
	}{
		{name: "connected but stale login", state: fakeConnectionState{connected: true, loggedIn: false}, wantReady: false},
		{name: "logged in but socket down", state: fakeConnectionState{connected: false, loggedIn: true}, wantReady: false},
		{name: "active session", state: fakeConnectionState{connected: true, loggedIn: true}, wantReady: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientSessionReady(tt.state); got != tt.wantReady {
				t.Fatalf("clientSessionReady() = %v, want %v", got, tt.wantReady)
			}
		})
	}
}
