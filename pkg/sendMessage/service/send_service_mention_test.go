package send_service

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func TestParticipantMentionJID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		p    types.GroupParticipant
		want string
	}{
		{
			name: "prefers_phone_number_over_lid",
			p: types.GroupParticipant{
				JID:         types.NewJID("123456789012345", types.HiddenUserServer),
				PhoneNumber: types.NewJID("5511999999999", types.DefaultUserServer),
			},
			want: "5511999999999@s.whatsapp.net",
		},
		{
			name: "falls_back_to_phone_jid",
			p:    types.GroupParticipant{JID: types.NewJID("5511888888888", types.DefaultUserServer)},
			want: "5511888888888@s.whatsapp.net",
		},
		{
			name: "retains_lid_without_phone_number",
			p:    types.GroupParticipant{JID: types.NewJID("123456789012345", types.HiddenUserServer)},
			want: "123456789012345@lid",
		},
		{
			name: "phone_number_without_primary_jid",
			p:    types.GroupParticipant{PhoneNumber: types.NewJID("5511999999999", types.DefaultUserServer)},
			want: "5511999999999@s.whatsapp.net",
		},
		{name: "missing_identity", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := participantMentionJID(tt.p); got != tt.want {
				t.Fatalf("participantMentionJID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetMessageMentionedJIDs(t *testing.T) {
	t.Parallel()

	mentioned := []string{"5511999999999@s.whatsapp.net", "5511888888888@s.whatsapp.net"}
	tests := []struct {
		name  string
		build func(*waE2E.ContextInfo) *waE2E.Message
	}{
		{"extended_text", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String("@todos"), ContextInfo: ctx,
			}}
		}},
		{"image", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
				Caption: proto.String("caption"), ContextInfo: ctx,
			}}
		}},
		{"video", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
				Caption: proto.String("caption"), ContextInfo: ctx,
			}}
		}},
		{"ptv", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{PtvMessage: &waE2E.VideoMessage{ContextInfo: ctx}}
		}},
		{"audio", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{AudioMessage: &waE2E.AudioMessage{ContextInfo: ctx}}
		}},
		{"document", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
				FileName: proto.String("document.pdf"), Caption: proto.String("doc caption"), ContextInfo: ctx,
			}}
		}},
		{"poll", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{
				Name: proto.String("question"), ContextInfo: ctx,
			}}
		}},
		{"sticker", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{StickerMessage: &waE2E.StickerMessage{ContextInfo: ctx}}
		}},
		{"location", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
				DegreesLatitude: proto.Float64(-15.6), ContextInfo: ctx,
			}}
		}},
		{"contact", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{ContactMessage: &waE2E.ContactMessage{
				DisplayName: proto.String("contact"), ContextInfo: ctx,
			}}
		}},
		{"interactive", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
				Body: &waE2E.InteractiveMessage_Body{Text: proto.String("select")}, ContextInfo: ctx,
			}}
		}},
		{"buttons", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{
				ContentText: proto.String("select"), ContextInfo: ctx,
			}}
		}},
		{"list", func(ctx *waE2E.ContextInfo) *waE2E.Message {
			return &waE2E.Message{ListMessage: &waE2E.ListMessage{
				Title: proto.String("select"), ContextInfo: ctx,
			}}
		}},
	}

	for _, tt := range tests {
		for _, wrapped := range []bool{false, true} {
			for _, existing := range []bool{false, true} {
				name := tt.name
				if wrapped {
					name += "/wrapped"
				}
				if existing {
					name += "/existing_context"
				} else {
					name += "/nil_context"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					var ctx *waE2E.ContextInfo
					if existing {
						ctx = &waE2E.ContextInfo{
							StanzaID:        proto.String("quoted-id"),
							Participant:     proto.String("5511777777777@s.whatsapp.net"),
							QuotedMessage:   &waE2E.Message{Conversation: proto.String("quoted text")},
							ForwardingScore: proto.Uint32(3),
							IsForwarded:     proto.Bool(true),
							MentionedJID:    []string{"old@s.whatsapp.net"},
						}
					}
					msg := tt.build(ctx)
					wantCtx := &waE2E.ContextInfo{}
					if ctx != nil {
						wantCtx = proto.Clone(ctx).(*waE2E.ContextInfo)
					}
					wantCtx.MentionedJID = mentioned
					want := tt.build(wantCtx)
					if wrapped {
						msg = &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: msg}}
						want = &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: want}}
					}
					if !setMessageMentionedJIDs(msg, mentioned) {
						t.Fatal("mentions were not applied")
					}
					// Compare the complete payload to catch loss of quotes, forwarding
					// metadata, caption or wrapper when updating the mention list.
					if !proto.Equal(msg, want) {
						t.Fatalf("message = %v, want %v", msg, want)
					}
				})
			}
		}
	}
}

func TestSetMessageMentionedJIDsRejectsMissingPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  *waE2E.Message
	}{
		{"nil_message", nil},
		{"empty_message", &waE2E.Message{}},
		{"unsupported_payload", &waE2E.Message{Conversation: proto.String("plain text")}},
		{"nil_wrapped_message", &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{}}},
		{"empty_wrapped_message", &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			before := proto.Clone(tt.msg)
			if setMessageMentionedJIDs(tt.msg, []string{"5511999999999@s.whatsapp.net"}) {
				t.Fatal("missing or unsupported payload must report failure")
			}
			if !proto.Equal(tt.msg, before) {
				t.Fatal("rejected message was modified")
			}
		})
	}
}

func TestSetMessageMentionedJIDsEmptyMentionsPreserveMessage(t *testing.T) {
	t.Parallel()

	msg := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text: proto.String("text"),
		ContextInfo: &waE2E.ContextInfo{
			MentionedJID: []string{"5511999999999@s.whatsapp.net"},
			StanzaID:     proto.String("quoted-id"),
		},
	}}
	before := proto.Clone(msg)
	for _, mentioned := range [][]string{nil, {}} {
		if setMessageMentionedJIDs(msg, mentioned) {
			t.Fatal("empty mention list must report no update")
		}
		if !proto.Equal(msg, before) {
			t.Fatal("empty mention list changed the message")
		}
	}
}
