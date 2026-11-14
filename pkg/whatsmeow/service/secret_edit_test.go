package whatsmeow_service

import (
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func secretEditEnvelope(encType waE2E.SecretEncryptedMessage_SecretEncType) *waE2E.Message {
	return &waE2E.Message{
		SecretEncryptedMessage: &waE2E.SecretEncryptedMessage{
			TargetMessageKey: &waCommon.MessageKey{ID: stringPtr("ORIGINAL_ID")},
			EncPayload:       []byte("ciphertext"),
			EncIV:            []byte("iv"),
			SecretEncType:    encType.Enum(),
		},
	}
}

func TestSecretEncryptedEdit(t *testing.T) {
	envelope := secretEncryptedEdit(secretEditEnvelope(waE2E.SecretEncryptedMessage_MESSAGE_EDIT))
	if envelope == nil {
		t.Fatal("expected MESSAGE_EDIT envelope to be detected as an edit")
	}

	if envelope.GetTargetMessageKey().GetID() != "ORIGINAL_ID" {
		t.Fatalf("expected the target message key to be reachable, got %q", envelope.GetTargetMessageKey().GetID())
	}

	if secretEncryptedEdit(secretEditEnvelope(waE2E.SecretEncryptedMessage_POLL_EDIT)) != nil {
		t.Fatal("expected POLL_EDIT envelope not to be treated as a message edit")
	}

	if secretEncryptedEdit(&waE2E.Message{Conversation: stringPtr("plain text")}) != nil {
		t.Fatal("expected a plain message not to be treated as an edit")
	}

	if secretEncryptedEdit(nil) != nil {
		t.Fatal("expected a nil message not to be treated as an edit")
	}
}

func TestBuildEditProtocolMessageRestoresPlaintextShape(t *testing.T) {
	target := &waCommon.MessageKey{ID: stringPtr("ORIGINAL_ID")}
	decrypted := &waE2E.Message{Conversation: stringPtr("corrected text")}

	rebuilt := buildEditProtocolMessage(target, decrypted, 1754216820000)
	if rebuilt == nil {
		t.Fatal("expected a rebuilt message, got nil")
	}

	protocolMessage := rebuilt.GetProtocolMessage()
	if protocolMessage.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT {
		t.Fatalf("expected type MESSAGE_EDIT, got %v", protocolMessage.GetType())
	}

	if protocolMessage.GetKey().GetID() != "ORIGINAL_ID" {
		t.Fatalf("expected the target message key to be preserved, got %q", protocolMessage.GetKey().GetID())
	}

	if protocolMessage.GetEditedMessage().GetConversation() != "corrected text" {
		t.Fatalf("expected the decrypted text, got %q", protocolMessage.GetEditedMessage().GetConversation())
	}

	if protocolMessage.GetTimestampMS() != 1754216820000 {
		t.Fatalf("expected the edit timestamp to be carried over, got %d", protocolMessage.GetTimestampMS())
	}
}

func TestBuildEditProtocolMessageOmitsUnknownTimestamp(t *testing.T) {
	rebuilt := buildEditProtocolMessage(
		&waCommon.MessageKey{ID: stringPtr("ORIGINAL_ID")},
		&waE2E.Message{Conversation: stringPtr("corrected text")},
		0,
	)

	if rebuilt.GetProtocolMessage().TimestampMS != nil {
		t.Fatal("expected no timestamp to be set when the event carries none")
	}
}

func TestBuildEditProtocolMessageRequiresTargetAndContent(t *testing.T) {
	decrypted := &waE2E.Message{Conversation: stringPtr("corrected text")}

	if buildEditProtocolMessage(nil, decrypted, 0) != nil {
		t.Fatal("expected nil without a target message key")
	}

	if buildEditProtocolMessage(&waCommon.MessageKey{ID: stringPtr("ORIGINAL_ID")}, nil, 0) != nil {
		t.Fatal("expected nil without decrypted content")
	}
}

// The envelope is typed as "secret encrypted", which carries no text. Rebuilding it as a
// protocolMessage is what makes the pipeline treat it as an edit.
func TestRebuiltEditIsTypedAsEdit(t *testing.T) {
	sealed := secretEditEnvelope(waE2E.SecretEncryptedMessage_MESSAGE_EDIT)
	if got := utils.GetMessageType(sealed); got == "edit" {
		t.Fatalf("expected the sealed envelope not to be typed as an edit, got %q", got)
	}

	rebuilt := buildEditProtocolMessage(
		secretEncryptedEdit(sealed).GetTargetMessageKey(),
		&waE2E.Message{Conversation: stringPtr("corrected text")},
		0,
	)

	if got := utils.GetMessageType(rebuilt); got != "edit" {
		t.Fatalf("expected the rebuilt message to be typed as %q, got %q", "edit", got)
	}
}
