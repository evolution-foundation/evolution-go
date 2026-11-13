package whatsmeow_service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestIncomingEditUsesOwningClientAndPreservesWireEvent(t *testing.T) {
	event, owner := encryptedEditFixture(t, editTestInfo(), marshalEditFixture(t, editTestProtocol("edited")))
	info, wire := event.Info, proto.Clone(event.Message)
	handler, capture := editHandlerFixture(t, owner)
	// A replacement session's map entry must not choose this event's secret store.
	handler.clientPointer = map[string]*whatsmeow.Client{editTestInstanceID: {}}
	handler.myEventHandler(event)
	data := capturedEditWebhook(t, capture)
	pm := data["Message"].(map[string]interface{})["protocolMessage"].(map[string]interface{})
	if data["IsEdit"] != true || data["messageType"] != "edit" || pm["typeName"] != "MESSAGE_EDIT" || pm["type"] != float64(14) {
		t.Fatal("incomplete edit classification", data)
	}
	if pm["key"].(map[string]interface{})["ID"] != editTestTargetID || pm["editedMessage"].(map[string]interface{})["conversation"] != "edited" {
		t.Fatal("missing target or plaintext", pm)
	}
	if _, exists := data["decryptFailed"]; exists {
		t.Fatal("successful decryption marked failed")
	}
	normalized := data["Info"].(map[string]interface{})
	if normalized["Sender"] != info.SenderAlt.ToNonAD().String() || normalized["Chat"] != info.SenderAlt.ToNonAD().String() {
		t.Fatal("webhook JID normalization changed", normalized)
	}
	if normalized["ID"] != info.ID {
		t.Fatal("edit event ID was replaced with target ID")
	}
	if !reflect.DeepEqual(info, event.Info) || event.IsEdit || !proto.Equal(wire, event.Message) || !proto.Equal(wire, event.RawMessage) {
		t.Fatal("shared whatsmeow event was modified")
	}
	if _, exists := data["RawMessage"]; exists {
		t.Fatal("raw encrypted envelope leaked into successful webhook")
	}
}

func TestIncomingLegacyEditFlags(t *testing.T) {
	for _, mode := range []string{"already_unwrapped", "legacy_wrapper", "newsletter_edit"} {
		t.Run(mode, func(t *testing.T) {
			text := &waE2E.Message{Conversation: proto.String("legacy edit")}
			event := &events.Message{Info: editTestInfo(), Message: text, IsEdit: mode == "already_unwrapped"}
			if mode == "legacy_wrapper" {
				event.Message = &waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{Message: text}}
			}
			if mode == "newsletter_edit" {
				event.Info.Chat = types.NewJID("channel", types.NewsletterServer)
				event.Info.ID = editTestTargetID
				event.NewsletterMeta = &events.NewsletterMessageMeta{EditTS: time.Unix(1700000001, 0)}
			}
			event.RawMessage = event.Message
			wire := proto.Clone(event.Message)
			handler, capture := editHandlerFixture(t, nil)
			handler.myEventHandler(event)
			data := capturedEditWebhook(t, capture)
			if data["IsEdit"] != true || data["messageType"] != "edit" || data["Message"].(map[string]interface{})["conversation"] != "legacy edit" {
				t.Fatal("legacy edit classification was lost", data)
			}
			if _, exists := data["decryptFailed"]; exists {
				t.Fatal("unencrypted edit was marked as decrypt failure")
			}
			if !proto.Equal(wire, event.Message) {
				t.Fatal("legacy source message was modified")
			}
		})
	}
}

func TestIncomingEditFailuresAreForwardedWithoutPanic(t *testing.T) {
	for _, mode := range []string{"no_client", "no_store", "no_secrets", "bad_nonce", "bad_mac", "unknown_protocol", "empty_plaintext", "store_failure", "group_no_client", "group_no_store", "group_bad_mac"} {
		t.Run(mode, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("incoming edit panicked: %v", recovered)
				}
			}()
			message := editTestProtocol("edited")
			if mode == "unknown_protocol" {
				message.ProtocolMessage.Type = waE2E.ProtocolMessage_HISTORY_SYNC_NOTIFICATION.Enum()
			}
			plaintext := marshalEditFixture(t, message)
			if mode == "empty_plaintext" {
				plaintext = nil
			}
			info := editTestInfo()
			if strings.HasPrefix(mode, "group_") {
				info.Chat = types.NewJID("group", types.GroupServer)
				info.IsGroup = true
			}
			event, client := encryptedEditFixture(t, info, plaintext)
			switch mode {
			case "no_client", "group_no_client":
				client = nil
			case "no_store", "group_no_store":
				client.Store = nil
			case "no_secrets":
				client.Store.MsgSecrets = nil
			case "bad_nonce":
				event.Message.SecretEncryptedMessage.EncIV = []byte{1}
			case "bad_mac", "group_bad_mac":
				event.Message.SecretEncryptedMessage.EncPayload[0] ^= 0xff
			case "store_failure":
				client.Store.MsgSecrets = &editTestSecretStore{get: func(context.Context, types.JID, types.JID, types.MessageID) ([]byte, types.JID, error) {
					return nil, types.EmptyJID, errors.New("private-connection-string-do-not-log")
				}}
			}
			wire := proto.Clone(event.Message)
			handler, capture := editHandlerFixture(t, client)
			handler.myEventHandler(event)
			data := capturedEditWebhook(t, capture)
			if data["IsEdit"] != true || data["messageType"] != "edit" || data["decryptFailed"] != true {
				t.Fatal("edit failure was not forwarded", data)
			}
			encrypted := data["Message"].(map[string]interface{})["secretEncryptedMessage"].(map[string]interface{})
			if encrypted["targetMessageKey"].(map[string]interface{})["ID"] != editTestTargetID {
				t.Fatal("failed edit lost original target", encrypted)
			}
			if event.IsEdit || !proto.Equal(wire, event.Message) {
				t.Fatal("failure modified original event")
			}
			if mode == "store_failure" {
				logData, err := os.ReadFile(filepath.Join(handler.config.LogDirectory, editTestInstanceID, "instance.log"))
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(logData), "private-connection-string-do-not-log") {
					t.Fatal("private store error was logged")
				}
			}
		})
	}
}

func TestIncomingRevokeAndOrdinaryMessageClassification(t *testing.T) {
	for _, mode := range []string{"revoke", "missing_protocol_type", "ordinary_text", "newsletter_message", "outbound_edit"} {
		t.Run(mode, func(t *testing.T) {
			message := &waE2E.Message{Conversation: proto.String("ordinary")}
			if mode == "revoke" || mode == "missing_protocol_type" {
				message = &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: proto.String(editTestTargetID)}}}
				if mode == "missing_protocol_type" {
					message.ProtocolMessage.Type = nil
				}
			}
			if mode == "outbound_edit" {
				message = editTestProtocol("outbound edit")
			}
			event := &events.Message{Info: editTestInfo(), Message: message, RawMessage: message}
			if mode == "newsletter_message" {
				event.NewsletterMeta = &events.NewsletterMessageMeta{OriginalTS: time.Unix(1700000000, 0)}
			}
			event.Info.IsFromMe = mode == "outbound_edit"
			wire := proto.Clone(message)
			handler, capture := editHandlerFixture(t, nil)
			handler.myEventHandler(event)
			data := capturedEditWebhook(t, capture)
			if mode == "revoke" {
				pm := data["Message"].(map[string]interface{})["protocolMessage"].(map[string]interface{})
				if data["IsRevoke"] != true || data["messageType"] != "revoke" || pm["type"] != float64(0) || pm["typeName"] != "REVOKE" {
					t.Fatal(data)
				}
			} else if mode == "outbound_edit" {
				if data["IsEdit"] != true || data["messageType"] != "edit" || data["Info"].(map[string]interface{})["IsFromMe"] != true {
					t.Fatal(data)
				}
			} else {
				if _, exists := data["messageType"]; exists {
					t.Fatal("ordinary or ambiguous message classified as edit/revoke", data)
				}
				if _, exists := data["IsRevoke"]; exists {
					t.Fatal("missing enum type interpreted as REVOKE")
				}
			}
			if !proto.Equal(wire, event.Message) {
				t.Fatal("original plaintext was changed")
			}
		})
	}
}
