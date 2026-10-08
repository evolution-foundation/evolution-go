package whatsmeow_service

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func TestPrepareIncomingMessageEditDecryptsWireIdentities(t *testing.T) {
	for _, source := range []string{"incoming_lid", "group_lid", "outgoing_echo"} {
		for _, shape := range []string{"protocol", "legacy_wrapper", "ephemeral_wrapper", "plain_text", "empty_text", "extended_text"} {
			t.Run(source+"/"+shape, func(t *testing.T) {
				info := editTestInfo()
				if source == "group_lid" {
					info.Chat = types.NewJID("group", types.GroupServer)
					info.IsGroup = true
				}
				if source == "outgoing_echo" {
					info.IsFromMe = true
				}
				message := editTestProtocol("edited")
				switch shape {
				case "legacy_wrapper":
					message = &waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{Message: message}}
				case "ephemeral_wrapper":
					message = &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: message}}
				case "plain_text":
					message = &waE2E.Message{Conversation: proto.String("edited")}
				case "empty_text":
					message = &waE2E.Message{Conversation: proto.String("")}
				case "extended_text":
					message = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("edited"), ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("quoted")}}}
				}
				original, client := encryptedEditFixture(t, info, marshalEditFixture(t, message))
				wire := proto.Clone(original.Message)
				owned := *original
				failed, err := prepareIncomingMessageEdit(context.Background(), client, &owned)
				if err != nil || failed {
					t.Fatalf("decrypt failed=%v err=%v", failed, err)
				}
				pm := owned.Message.GetProtocolMessage()
				if !owned.IsEdit || pm.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT || pm.GetKey().GetID() != editTestTargetID {
					t.Fatalf("incomplete edit: %#v", owned)
				}
				if shape == "extended_text" {
					if pm.GetEditedMessage().GetExtendedTextMessage().GetContextInfo().GetStanzaID() != "quoted" {
						t.Fatal("quote context was lost")
					}
				} else if shape != "empty_text" && pm.GetEditedMessage().GetConversation() != "edited" {
					t.Fatal("edited text was lost")
				}
				if owned.IsEphemeral != (shape == "ephemeral_wrapper") {
					t.Fatal("wrapper flags were lost")
				}
				if !reflect.DeepEqual(original.Info, owned.Info) || owned.RawMessage != original.RawMessage || !proto.Equal(original.Message, wire) || original.IsEdit {
					t.Fatal("wire event or JIDs were modified")
				}
			})
		}
	}
}

func TestPrepareIncomingMessageEditFailureRetainsEnvelope(t *testing.T) {
	for _, mode := range []string{"no_client", "no_store", "no_secrets", "missing_secret", "store_error", "bad_nonce", "bad_mac", "missing_target", "invalid_protobuf", "empty_plaintext", "unknown_protocol", "wrong_target", "missing_edit_body"} {
		t.Run(mode, func(t *testing.T) {
			message := editTestProtocol("edited")
			if mode == "unknown_protocol" {
				message.ProtocolMessage.Type = waE2E.ProtocolMessage_HISTORY_SYNC_NOTIFICATION.Enum()
			}
			if mode == "wrong_target" {
				message.ProtocolMessage.Key.ID = proto.String("different-message")
			}
			if mode == "missing_edit_body" {
				message.ProtocolMessage.EditedMessage = nil
			}
			plaintext := marshalEditFixture(t, message)
			if mode == "invalid_protobuf" {
				plaintext = []byte{0xff}
			}
			if mode == "empty_plaintext" {
				plaintext = nil
			}
			event, client := encryptedEditFixture(t, editTestInfo(), plaintext)
			storeFailure := errors.New("store unavailable")
			switch mode {
			case "no_client":
				client = nil
			case "no_store":
				client.Store = nil
			case "no_secrets":
				client.Store.MsgSecrets = nil
			case "missing_secret":
				client.Store.MsgSecrets = &editTestSecretStore{get: func(context.Context, types.JID, types.JID, types.MessageID) ([]byte, types.JID, error) {
					return nil, types.EmptyJID, nil
				}}
			case "store_error":
				client.Store.MsgSecrets = &editTestSecretStore{get: func(context.Context, types.JID, types.JID, types.MessageID) ([]byte, types.JID, error) {
					return nil, types.EmptyJID, storeFailure
				}}
			case "bad_nonce":
				event.Message.SecretEncryptedMessage.EncIV = []byte{1}
			case "bad_mac":
				event.Message.SecretEncryptedMessage.EncPayload[0] ^= 0xff
			case "missing_target":
				event.Message.SecretEncryptedMessage.TargetMessageKey = nil
			}
			wire := proto.Clone(event.Message)
			failed, err := prepareIncomingMessageEdit(context.Background(), client, event)
			if err == nil || !failed || !event.IsEdit || !proto.Equal(event.Message, wire) {
				t.Fatalf("failure lost edit envelope: failed=%v err=%v", failed, err)
			}
			if mode == "store_error" && !errors.Is(err, storeFailure) {
				t.Fatal("store error cause was lost")
			}
			if mode == "missing_secret" && !errors.Is(err, whatsmeow.ErrOriginalMessageSecretNotFound) {
				t.Fatal("missing-secret cause was lost")
			}
		})
	}
}

func TestPrepareIncomingMessageEditContextIsBoundedAndCancelled(t *testing.T) {
	for _, mode := range []string{"success", "parent_cancel", "parent_deadline"} {
		t.Run(mode, func(t *testing.T) {
			event, client := encryptedEditFixture(t, editTestInfo(), marshalEditFixture(t, editTestProtocol("edited")))
			var lookupCtx context.Context
			secretStore := client.Store.MsgSecrets
			client.Store.MsgSecrets = &editTestSecretStore{get: func(ctx context.Context, chat, sender types.JID, id types.MessageID) ([]byte, types.JID, error) {
				lookupCtx = ctx
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > messageEditDecryptTimeout {
					t.Error("lookup has no finite edit deadline")
				}
				if mode != "success" {
					<-ctx.Done()
					return nil, types.EmptyJID, ctx.Err()
				}
				return secretStore.GetMessageSecret(ctx, chat, sender, id)
			}}
			ctx, cancel := context.WithCancel(context.Background())
			if mode == "parent_cancel" {
				cancel()
			}
			if mode == "parent_deadline" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
			}
			defer cancel()
			failed, err := prepareIncomingMessageEdit(ctx, client, event)
			if mode == "success" && (failed || err != nil) {
				t.Fatal(err)
			}
			if mode == "parent_cancel" && (!failed || !errors.Is(err, context.Canceled)) {
				t.Fatal(err)
			}
			if mode == "parent_deadline" && (!failed || !errors.Is(err, context.DeadlineExceeded)) {
				t.Fatal(err)
			}
			select {
			case <-lookupCtx.Done():
			default:
				t.Fatal("owned lookup context was not cancelled")
			}
		})
	}
}

func TestConcurrentMessageEditCopiesKeepWireEventUnchanged(t *testing.T) {
	original, client := encryptedEditFixture(t, editTestInfo(), marshalEditFixture(t, editTestProtocol("edited")))
	wire := proto.Clone(original.Message)
	var workers sync.WaitGroup
	for range 20 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			owned := *original
			if failed, err := prepareIncomingMessageEdit(context.Background(), client, &owned); failed || err != nil {
				t.Errorf("failed=%v err=%v", failed, err)
			}
		}()
	}
	workers.Wait()
	if original.IsEdit || !proto.Equal(original.Message, wire) {
		t.Fatal("shared wire event was modified")
	}
}

func TestOtherEncryptedMessageKindsAreUntouched(t *testing.T) {
	event, _ := encryptedEditFixture(t, editTestInfo(), nil)
	event.Message.SecretEncryptedMessage.SecretEncType = waE2E.SecretEncryptedMessage_EVENT_EDIT.Enum()
	wire := proto.Clone(event.Message)
	client := &whatsmeow.Client{Store: &store.Device{MsgSecrets: &editTestSecretStore{get: func(context.Context, types.JID, types.JID, types.MessageID) ([]byte, types.JID, error) {
		t.Fatal("non-message edit queried secret store")
		return nil, types.EmptyJID, nil
	}}}}
	failed, err := prepareIncomingMessageEdit(context.Background(), client, event)
	if failed || err != nil || event.IsEdit || !proto.Equal(wire, event.Message) {
		t.Fatal("unrelated encrypted message was changed")
	}
}
