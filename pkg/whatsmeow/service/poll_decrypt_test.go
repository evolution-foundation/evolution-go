package whatsmeow_service

import (
	"errors"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

var (
	voterLID   = types.NewJID("138139133833279", types.HiddenUserServer)
	voterPhone = types.NewJID("559294056867", types.DefaultUserServer)
	errMAC     = errors.New("cipher: message authentication failed")
)

// fakeDecrypt só "decifra" quando Sender e Chat são os da identidade @lid.
func fakeDecrypt(calls *[]types.MessageInfo) func(*events.Message) (*waE2E.PollVoteMessage, error) {
	return func(e *events.Message) (*waE2E.PollVoteMessage, error) {
		*calls = append(*calls, e.Info)
		if e.Info.Sender == voterLID && e.Info.Chat == voterLID {
			return &waE2E.PollVoteMessage{SelectedOptions: [][]byte{{1}}}, nil
		}
		return nil, errMAC
	}
}

func swappedEvent() (*events.Message, types.MessageInfo) {
	raw := types.MessageInfo{MessageSource: types.MessageSource{Sender: voterLID, SenderAlt: voterPhone, Chat: voterLID}}
	evt := &events.Message{Info: raw}
	// O que o handler faz no swap: Sender/Chat viram telefone.
	evt.Info.Sender, evt.Info.SenderAlt, evt.Info.Chat = voterPhone, voterLID, voterPhone
	return evt, raw
}

func TestPollVoteDecryptsWithPreSwapInfo(t *testing.T) {
	evt, raw := swappedEvent()
	var calls []types.MessageInfo

	vote, err := decryptWithOriginalInfo(evt, raw, fakeDecrypt(&calls))
	if err != nil || vote == nil {
		t.Fatalf("expected vote decrypted with original @lid info, got err=%v", err)
	}
	if evt.Info.Sender != voterPhone || evt.Info.Chat != voterPhone {
		t.Fatalf("event info must stay swapped (saved under the phone number), got %+v", evt.Info)
	}
	if len(calls) != 1 {
		t.Fatalf("expected a single decrypt attempt, got %d", len(calls))
	}
}

func TestPollVoteFallsBackToCurrentInfo(t *testing.T) {
	evt, raw := swappedEvent()
	var calls []types.MessageInfo
	// Decrypt que só funciona com o Info atual (telefone).
	dec := func(e *events.Message) (*waE2E.PollVoteMessage, error) {
		calls = append(calls, e.Info)
		if e.Info.Sender == voterPhone {
			return &waE2E.PollVoteMessage{}, nil
		}
		return nil, errMAC
	}
	if _, err := decryptWithOriginalInfo(evt, raw, dec); err != nil {
		t.Fatalf("fallback to current info should succeed: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(calls))
	}
}

func TestPollVoteNoRetryWhenInfoUnchanged(t *testing.T) {
	info := types.MessageInfo{MessageSource: types.MessageSource{Sender: voterPhone, Chat: voterPhone}}
	evt := &events.Message{Info: info}
	var calls []types.MessageInfo
	if _, err := decryptWithOriginalInfo(evt, info, fakeDecrypt(&calls)); !errors.Is(err, errMAC) {
		t.Fatalf("expected original error, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("no retry expected when info did not change, got %d calls", len(calls))
	}
}
