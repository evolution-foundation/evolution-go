package whatsmeow_service

import (
	"context"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// Edições feitas no celular (principalmente em conversas @lid) não chegam mais
// como protocolMessage em texto: o conteúdo novo vem num secretEncryptedMessage
// (MESSAGE_EDIT), cifrado com o segredo da mensagem original. Sem abrir, o
// webhook só recebe o envelope ("secret encrypted") e o consumidor não sabe o
// que mudou. Ref. upstream #153 / #128 / #161 / #122.

// secretEncryptedEdit retorna o envelope de uma edição cifrada, ou nil.
func secretEncryptedEdit(message *waE2E.Message) *waE2E.SecretEncryptedMessage {
	envelope := message.GetSecretEncryptedMessage()
	if envelope == nil || envelope.GetSecretEncType() != waE2E.SecretEncryptedMessage_MESSAGE_EDIT {
		return nil
	}
	return envelope
}

// buildEditProtocolMessage monta o formato que os consumidores já conhecem:
// protocolMessage{type: MESSAGE_EDIT, key: <alvo>, editedMessage: <conteúdo>}.
func buildEditProtocolMessage(target *waCommon.MessageKey, decrypted *waE2E.Message, timestampMS int64) *waE2E.Message {
	if target == nil || decrypted == nil {
		return nil
	}
	pm := &waE2E.ProtocolMessage{
		Key:           target,
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		EditedMessage: decrypted,
	}
	if timestampMS > 0 {
		pm.TimestampMS = proto.Int64(timestampMS)
	}
	return &waE2E.Message{ProtocolMessage: pm}
}

// unwrapSecretEncryptedEdit substitui o envelope pela edição em texto. Usa o
// MessageInfo de antes do swap LID -> telefone (a chave depende dele). Em
// qualquer falha mantém o evento como veio: encaminhar o envelope é o
// comportamento antigo, e descartar perderia o sinal de edição.
func (mycli *MyClient) unwrapSecretEncryptedEdit(evt *events.Message, rawInfo types.MessageInfo) {
	if evt == nil || mycli.WAClient == nil {
		return
	}
	envelope := secretEncryptedEdit(evt.Message)
	if envelope == nil {
		return
	}
	logger := mycli.loggerWrapper.GetLogger(mycli.userID)
	client := mycli.WAClient

	decrypted, err := decryptWithOriginalInfo(evt, rawInfo, func(e *events.Message) (*waE2E.Message, error) {
		return client.DecryptSecretEncryptedMessage(context.Background(), e)
	})
	if err != nil {
		logger.LogError("[%s] Failed to decrypt edited message %s: %v", mycli.userID, evt.Info.ID, err)
		return
	}

	rebuilt := buildEditProtocolMessage(envelope.GetTargetMessageKey(), decrypted, evt.Info.Timestamp.UnixMilli())
	if rebuilt == nil {
		logger.LogWarn("[%s] Decrypted edit %s has no target key; forwarding envelope as-is", mycli.userID, evt.Info.ID)
		return
	}
	evt.Message = rebuilt
	logger.LogInfo("[%s] Decrypted edited message %s targeting %s", mycli.userID, evt.Info.ID, envelope.GetTargetMessageKey().GetID())
}
