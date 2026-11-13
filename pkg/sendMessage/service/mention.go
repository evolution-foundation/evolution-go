package send_service

import (
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// participantMentionJID prefers the phone-number JID when available.
// Some participants expose only a LID. Retain that identifier as a fallback;
// converting its user to a phone-number JID would invent a different identity.
func participantMentionJID(p types.GroupParticipant) string {
	if !p.PhoneNumber.IsEmpty() {
		return p.PhoneNumber.String()
	}
	if p.JID.IsEmpty() {
		return ""
	}
	return p.JID.String()
}

// setMessageMentionedJIDs preserves other ContextInfo fields and reports whether
// mentions were applied. Inspect the payload instead of a separate type label:
// captioned documents, buttons and lists can live inside the same wrapper.
func setMessageMentionedJIDs(msg *waE2E.Message, mentionedJIDs []string) bool {
	if len(mentionedJIDs) == 0 {
		return false
	}
	for msg != nil && msg.DocumentWithCaptionMessage != nil {
		msg = msg.DocumentWithCaptionMessage.Message
	}
	if msg == nil {
		return false
	}

	var contextInfo **waE2E.ContextInfo
	switch {
	case msg.ExtendedTextMessage != nil:
		contextInfo = &msg.ExtendedTextMessage.ContextInfo
	case msg.ImageMessage != nil:
		contextInfo = &msg.ImageMessage.ContextInfo
	case msg.VideoMessage != nil:
		contextInfo = &msg.VideoMessage.ContextInfo
	case msg.PtvMessage != nil:
		contextInfo = &msg.PtvMessage.ContextInfo
	case msg.AudioMessage != nil:
		contextInfo = &msg.AudioMessage.ContextInfo
	case msg.DocumentMessage != nil:
		contextInfo = &msg.DocumentMessage.ContextInfo
	case msg.PollCreationMessage != nil:
		contextInfo = &msg.PollCreationMessage.ContextInfo
	case msg.StickerMessage != nil:
		contextInfo = &msg.StickerMessage.ContextInfo
	case msg.LocationMessage != nil:
		contextInfo = &msg.LocationMessage.ContextInfo
	case msg.ContactMessage != nil:
		contextInfo = &msg.ContactMessage.ContextInfo
	case msg.InteractiveMessage != nil:
		contextInfo = &msg.InteractiveMessage.ContextInfo
	case msg.ButtonsMessage != nil:
		contextInfo = &msg.ButtonsMessage.ContextInfo
	case msg.ListMessage != nil:
		contextInfo = &msg.ListMessage.ContextInfo
	default:
		return false
	}
	if *contextInfo == nil {
		*contextInfo = &waE2E.ContextInfo{}
	}
	(*contextInfo).MentionedJID = mentionedJIDs
	return true
}
