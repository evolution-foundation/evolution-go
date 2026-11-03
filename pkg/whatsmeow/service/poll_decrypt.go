package whatsmeow_service

import (
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// decryptPollVoteWithOriginalInfo descriptografa um voto de enquete usando o
// MessageInfo original (antes do nosso swap LID -> telefone).
//
// A chave do voto é derivada da identidade do votante (Info.Sender) e o segredo
// da enquete é buscado por Info.Chat. O WhatsApp usa a identidade @lid; depois do
// swap, Sender (e, em conversa privada, Chat) viram @s.whatsapp.net, o GCM falha
// com "message authentication failed" e o voto se perde. Tenta primeiro com o
// Info original e, se falhar e ele for diferente, com o Info atual. O evento
// recebido não é alterado, então o voto continua salvo com o número real.
// Ref.: fork Aryzonna eba29515 (lá só o Sender era revertido).
func decryptPollVoteWithOriginalInfo(
	evt *events.Message,
	rawInfo types.MessageInfo,
	decrypt func(*events.Message) (*waE2E.PollVoteMessage, error),
) (*waE2E.PollVoteMessage, error) {
	original := *evt
	original.Info = rawInfo
	vote, err := decrypt(&original)
	if err == nil {
		return vote, nil
	}
	if rawInfo.Sender == evt.Info.Sender && rawInfo.Chat == evt.Info.Chat {
		return nil, err
	}
	if vote, retryErr := decrypt(evt); retryErr == nil {
		return vote, nil
	}
	return nil, err
}
