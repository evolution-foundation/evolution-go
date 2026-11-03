package whatsmeow_service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
)

func TestAppStateRecoveryReserveCooldownPerStageAndCollection(t *testing.T) {
	var s appStateRecoveryState
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	low := appstate.WAPatchRegularLow

	if !s.reserve(low, false, now) {
		t.Fatal("first full sync should be reserved")
	}
	if s.reserve(low, false, now.Add(time.Minute)) {
		t.Fatal("repeated full sync inside cooldown should be suppressed")
	}
	if !s.reserve(low, true, now.Add(time.Minute)) {
		t.Fatal("recovery request is a separate stage")
	}
	if s.reserve(low, true, now.Add(2*time.Minute)) {
		t.Fatal("repeated recovery request inside cooldown should be suppressed")
	}
	if !s.reserve(appstate.WAPatchRegularHigh, false, now.Add(time.Minute)) {
		t.Fatal("cooldown of one collection must not block another")
	}
	if !s.reserve(low, false, now.Add(appStateRecoveryCooldown+time.Second)) {
		t.Fatal("full sync should be allowed again after the cooldown")
	}
}

func TestIsTransientAppStateError(t *testing.T) {
	iq := fmt.Errorf("failed to fetch: %w", &whatsmeow.IQError{Code: 429})
	if !isTransientAppStateError(iq) {
		t.Error("wrapped IQ error should be transient")
	}
	if !isTransientAppStateError(fmt.Errorf("x: %w", whatsmeow.ErrAppStateRateLimited)) {
		t.Error("rate limit should be transient")
	}
	if isTransientAppStateError(errors.New("mismatching LTHash")) {
		t.Error("verification failure must not be treated as transient")
	}
}
