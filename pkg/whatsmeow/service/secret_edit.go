package whatsmeow_service

import (
	"context"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// secretEncryptedEdit returns the envelope carrying a sealed message edit, or nil when the
// message is not one.
//
// WhatsApp no longer sends message edits as a plaintext protocolMessage: the new content is
// encrypted with a key derived from the target message secret. Without decrypting it, the
// envelope reaches consumers as an opaque blob with no text at all.
func secretEncryptedEdit(message *waE2E.Message) *waE2E.SecretEncryptedMessage {
	if message == nil {
		return nil
	}

	envelope := message.GetSecretEncryptedMessage()
	if envelope == nil || envelope.GetSecretEncType() != waE2E.SecretEncryptedMessage_MESSAGE_EDIT {
		return nil
	}

	return envelope
}

// buildEditProtocolMessage rebuilds the plaintext shape consumers already handle —
// protocolMessage{type: MESSAGE_EDIT, key: <target>, editedMessage: <new content>} — so the
// decrypted edit needs no new contract downstream. Returns nil when there is nothing to
// rebuild, letting callers keep the original envelope.
func buildEditProtocolMessage(target *waCommon.MessageKey, decrypted *waE2E.Message, timestampMS int64) *waE2E.Message {
	if target == nil || decrypted == nil {
		return nil
	}

	protocolMessage := &waE2E.ProtocolMessage{
		Key:           target,
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		EditedMessage: decrypted,
	}

	if timestampMS > 0 {
		protocolMessage.TimestampMS = proto.Int64(timestampMS)
	}

	return &waE2E.Message{ProtocolMessage: protocolMessage}
}

// unwrapSecretEncryptedEdit decrypts a MESSAGE_EDIT envelope in place, so message typing, the
// webhook payload and persistence all see the edited text instead of the sealed envelope.
//
// Every failure path leaves the event untouched on purpose: forwarding the original envelope
// is what happens today, while dropping the event would lose the edit signal entirely.
func (mycli *MyClient) unwrapSecretEncryptedEdit(evt *events.Message) {
	if evt == nil {
		return
	}

	envelope := secretEncryptedEdit(evt.Message)
	if envelope == nil {
		return
	}

	client := mycli.clientPointer[mycli.userID]
	if client == nil {
		return
	}

	targetKey := envelope.GetTargetMessageKey()

	decrypted, err := client.DecryptSecretEncryptedMessage(context.Background(), evt)
	if err != nil {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to decrypt edited message %s: %v", mycli.userID, evt.Info.ID, err)
		return
	}

	rebuilt := buildEditProtocolMessage(targetKey, decrypted, evt.Info.Timestamp.UnixMilli())
	if rebuilt == nil {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Decrypted edit %s has no target message key, forwarding envelope as-is", mycli.userID, evt.Info.ID)
		return
	}

	evt.Message = rebuilt
	mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Decrypted edited message %s targeting %s", mycli.userID, evt.Info.ID, targetKey.GetID())
}
