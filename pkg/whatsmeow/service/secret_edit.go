package whatsmeow_service

// Decryption of secret-encrypted message EDITS.
//
// When a phone edits a message in a @lid-addressed chat, WhatsApp delivers the
// edit as a `secretEncryptedMessage` (MESSAGE_EDIT): the new content is
// encrypted with the "message secret" of the ORIGINAL message — a secret this
// whatsmeow session already holds (it arrived inside the original message
// itself). Without this handling, the encrypted event is forwarded to the
// webhook as-is, so consumers learn THAT an edit happened but not WHAT changed.
// This closes the loop: decrypt with the official primitive
// (Client.DecryptSecretEncryptedMessage, which natively supports
// EncSecretMessageEdit) and rewrite the event into the CANONICAL cleartext
// shape (protocolMessage MESSAGE_EDIT + editedMessage) — the same shape
// BuildEdit produces — so webhook consumers handle both paths identically.
//
// Kept in its own file (plus a single line at the handler call-site) on
// purpose: rebasing over new evolution-go/whatsmeow releases stays trivial.

import (
	"context"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// resolveSecretEncryptedEdit detects a secret-encrypted edit
// (secretEncryptedMessage MESSAGE_EDIT) and, when decryption succeeds,
// rewrites evt.Message into the canonical cleartext shape. Fail-open: on any
// failure (missing secret, unsupported type, …) the event passes through
// untouched — behavior falls back to what it was before this change (webhook
// receives the encrypted payload).
func (mycli *MyClient) resolveSecretEncryptedEdit(evt *events.Message) {
	enc := evt.Message.GetSecretEncryptedMessage()
	if enc == nil || enc.GetSecretEncType() != waE2E.SecretEncryptedMessage_MESSAGE_EDIT {
		return
	}
	decrypted, err := mycli.WAClient.DecryptSecretEncryptedMessage(context.Background(), evt)
	if err != nil {
		// Without the original message's secret (e.g. the message predates this
		// pairing) there is nothing to do — the consumer still receives the
		// cleartext target (targetMessageKey) and can at least flag the message
		// as edited.
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] secret-edit: failed to decrypt edit of %s: %v", mycli.userID, enc.GetTargetMessageKey().GetID(), err)
		return
	}
	mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] secret-edit: edit of %s decrypted — forwarding in cleartext", mycli.userID, enc.GetTargetMessageKey().GetID())
	// Phones encrypt the FULL edit envelope (the plaintext is already a
	// protocolMessage MESSAGE_EDIT with editedMessage inside) — use it as-is;
	// re-wrapping would double the nesting. The fallback covers a plaintext
	// that carries only the new content.
	if pm := decrypted.GetProtocolMessage(); pm != nil &&
		pm.GetType() == waE2E.ProtocolMessage_MESSAGE_EDIT && pm.GetEditedMessage() != nil {
		evt.Message = decrypted
		return
	}
	evt.Message = &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
			Key:           enc.GetTargetMessageKey(),
			EditedMessage: decrypted,
			TimestampMS:   proto.Int64(evt.Info.Timestamp.UnixMilli()),
		},
	}
}
