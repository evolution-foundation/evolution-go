package whatsmeow_service

import "testing"

func TestSignalKillDeliversToBlockedLoop(t *testing.T) {
	kill := make(chan bool)
	got := make(chan bool, 1)
	go func() { got <- <-kill }()

	if !signalKill(kill, false) {
		t.Fatal("expected the signal to reach the waiting loop")
	}
	if <-got {
		t.Fatal("expected a quiet stop (false)")
	}
}

func TestSignalKillWithoutChannel(t *testing.T) {
	if signalKill(nil, true) {
		t.Fatal("a nil channel must not report delivery")
	}
}
