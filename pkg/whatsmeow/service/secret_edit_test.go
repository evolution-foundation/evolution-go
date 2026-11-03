package whatsmeow_service

import (
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestSecretEncryptedEditDetection(t *testing.T) {
	edit := &waE2E.Message{SecretEncryptedMessage: &waE2E.SecretEncryptedMessage{
		SecretEncType: waE2E.SecretEncryptedMessage_MESSAGE_EDIT.Enum(),
	}}
	if secretEncryptedEdit(edit) == nil {
		t.Fatal("MESSAGE_EDIT envelope should be detected")
	}
	other := &waE2E.Message{SecretEncryptedMessage: &waE2E.SecretEncryptedMessage{
		SecretEncType: waE2E.SecretEncryptedMessage_EVENT_EDIT.Enum(),
	}}
	if secretEncryptedEdit(other) != nil {
		t.Fatal("non-edit envelope must be ignored")
	}
	if secretEncryptedEdit(nil) != nil || secretEncryptedEdit(&waE2E.Message{Conversation: proto.String("x")}) != nil {
		t.Fatal("plain/nil messages must be ignored")
	}
}

func TestBuildEditProtocolMessageIsTypedAsEdit(t *testing.T) {
	target := &waCommon.MessageKey{ID: proto.String("ORIG")}
	msg := buildEditProtocolMessage(target, &waE2E.Message{Conversation: proto.String("novo texto")}, 1700000000000)
	if msg == nil {
		t.Fatal("expected rebuilt message")
	}
	if got := utils.GetMessageType(msg); got != "edit" {
		t.Fatalf("GetMessageType = %q, want edit", got)
	}
	pm := msg.GetProtocolMessage()
	if pm.GetKey().GetID() != "ORIG" || pm.GetEditedMessage().GetConversation() != "novo texto" || pm.GetTimestampMS() != 1700000000000 {
		t.Fatalf("unexpected protocol message: %v", pm)
	}
	if buildEditProtocolMessage(nil, &waE2E.Message{}, 0) != nil {
		t.Fatal("missing target key must return nil")
	}
}
