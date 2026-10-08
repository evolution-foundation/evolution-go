package whatsmeow_service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

const messageEditDecryptTimeout = 5 * time.Second

var errInvalidMessageEdit = errors.New("invalid encrypted message edit")

// prepareIncomingMessageEdit operates on a caller-owned event before any JID
// normalization. The session client owns the secret store; consulting the shared
// client map could select a replacement session or race with its registration.
// A failed edit keeps its encrypted envelope and original target key.
func prepareIncomingMessageEdit(ctx context.Context, client *whatsmeow.Client, evt *events.Message) (bool, error) {
	unwrapMessageForWebhook(evt, evt.Message)
	// Newsletter edits arrive as new content with the original message ID,
	// identified by EditTS rather than an EditedMessage wrapper.
	if evt.NewsletterMeta != nil && !evt.NewsletterMeta.EditTS.IsZero() {
		evt.IsEdit = true
	}
	enc := evt.Message.GetSecretEncryptedMessage()
	if enc == nil || enc.GetSecretEncType() != waE2E.SecretEncryptedMessage_MESSAGE_EDIT {
		return false, nil
	}
	evt.IsEdit = true
	// whatsmeow uses standard AES-GCM with a 12-byte nonce. GCM.Open panics
	// for a nonce of the wrong length, so reject it before calling the library.
	if len(enc.GetEncIV()) != 12 || enc.GetTargetMessageKey().GetID() == "" {
		return true, errInvalidMessageEdit
	}
	if client == nil {
		return true, whatsmeow.ErrClientIsNil
	}
	if client.Store == nil || client.Store.MsgSecrets == nil {
		return true, errors.New("message secret store unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, messageEditDecryptTimeout)
	defer cancel()
	decrypted, err := client.DecryptSecretEncryptedMessage(ctx, evt)
	if err != nil {
		return true, fmt.Errorf("decrypt incoming message edit: %w", err)
	}
	decoded := *evt
	unwrapMessageForWebhook(&decoded, decrypted)
	if pm := decoded.Message.GetProtocolMessage(); pm != nil {
		if pm.Type == nil || pm.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT ||
			pm.GetKey().GetID() != enc.GetTargetMessageKey().GetID() ||
			pm.GetEditedMessage() == nil || proto.Size(pm.GetEditedMessage()) == 0 {
			return true, errInvalidMessageEdit
		}
	} else if decoded.Message != nil && (decoded.Message.Conversation != nil ||
		(decoded.Message.ExtendedTextMessage != nil && decoded.Message.ExtendedTextMessage.Text != nil)) {
		// If an envelope contains the edited text directly, keep the real target
		// key in the same protocol shape consumers use for ordinary message edits.
		decoded.Message = &waE2E.Message{
			MessageContextInfo: decoded.Message.GetMessageContextInfo(),
			ProtocolMessage: &waE2E.ProtocolMessage{
				Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
				Key:           proto.Clone(enc.GetTargetMessageKey()).(*waCommon.MessageKey),
				EditedMessage: decoded.Message,
			},
		}
	} else {
		return true, errInvalidMessageEdit
	}
	*evt = decoded
	return false, nil
}

// UnwrapRaw is safe on this local copy only after temporarily replacing its raw
// pointer. Restore the wire envelope so decrypt never resets from ciphertext.
func unwrapMessageForWebhook(evt *events.Message, message *waE2E.Message) {
	wire := evt.RawMessage
	evt.RawMessage = message
	evt.UnwrapRaw()
	evt.RawMessage = wire
}

func annotateMessageAction(dataMap map[string]interface{}, evt *events.Message, decryptFailed bool) {
	pm := evt.Message.GetProtocolMessage()
	if pm != nil && pm.Type != nil && pm.GetType() == waE2E.ProtocolMessage_REVOKE && pm.GetKey().GetID() != "" {
		dataMap["IsRevoke"] = true
		dataMap["messageType"] = "revoke"
		setProtocolMessageTypeName(dataMap, "REVOKE")
	} else if evt.IsEdit || (pm != nil && pm.Type != nil && pm.GetType() == waE2E.ProtocolMessage_MESSAGE_EDIT) {
		dataMap["IsEdit"] = true
		dataMap["messageType"] = "edit"
		if pm != nil && pm.Type != nil && pm.GetType() == waE2E.ProtocolMessage_MESSAGE_EDIT {
			setProtocolMessageTypeName(dataMap, "MESSAGE_EDIT")
		}
	}
	if decryptFailed {
		dataMap["decryptFailed"] = true
	}
}

// setProtocolMessageTypeName preserves the numeric enum serialized by protobuf.
func setProtocolMessageTypeName(dataMap map[string]interface{}, typeName string) {
	message, ok := dataMap["Message"].(map[string]interface{})
	if !ok {
		return
	}
	pm, ok := message["protocolMessage"].(map[string]interface{})
	if !ok {
		return
	}
	pm["typeName"] = typeName
}
