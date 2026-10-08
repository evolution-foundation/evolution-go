package whatsmeow_service

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"go.mau.fi/whatsmeow/util/gcmutil"
	"go.mau.fi/whatsmeow/util/hkdfutil"
	"google.golang.org/protobuf/proto"
)

const editTestInstanceID = "edit-instance"
const editTestTargetID = "original-message"

type editTestSecretStore struct {
	store.MsgSecretStore
	get func(context.Context, types.JID, types.JID, types.MessageID) ([]byte, types.JID, error)
}

func (s *editTestSecretStore) GetMessageSecret(ctx context.Context, chat, sender types.JID, id types.MessageID) ([]byte, types.JID, error) {
	return s.get(ctx, chat, sender, id)
}

func editTestInfo() types.MessageInfo {
	return types.MessageInfo{
		ID: "edit-event", Type: "text", Edit: "1", Timestamp: time.Unix(1700000000, 0),
		MessageSource: types.MessageSource{
			Chat:      types.NewJID("100", types.HiddenUserServer),
			Sender:    types.JID{User: "100", Server: types.HiddenUserServer, Device: 3},
			SenderAlt: types.JID{User: "5511999999999", Server: types.DefaultUserServer, Device: 3},
		},
	}
}

func editTestProtocol(text string) *waE2E.Message {
	return &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		Key:           &waCommon.MessageKey{ID: proto.String(editTestTargetID)},
		EditedMessage: &waE2E.Message{Conversation: proto.String(text)},
	}}
}

// Construct a real authenticated payload using the pinned whatsmeow wire format,
// then let its public decryption API validate the fixture in production code.
func encryptedEditFixture(t *testing.T, info types.MessageInfo, plaintext []byte) (*events.Message, *whatsmeow.Client) {
	t.Helper()
	secret := bytes.Repeat([]byte{0x42}, 32)
	sender := info.Sender.ToNonAD().String()
	key := hkdfutil.SHA256(secret, nil, []byte(editTestTargetID+sender+sender+string(whatsmeow.EncSecretMessageEdit)), 32)
	iv := bytes.Repeat([]byte{0x24}, 12)
	ciphertext, err := gcmutil.Encrypt(key, iv, plaintext, nil)
	if err != nil {
		t.Fatal(err)
	}
	message := &waE2E.Message{SecretEncryptedMessage: &waE2E.SecretEncryptedMessage{
		SecretEncType: waE2E.SecretEncryptedMessage_MESSAGE_EDIT.Enum(),
		EncIV:         iv, EncPayload: ciphertext,
		TargetMessageKey: &waCommon.MessageKey{
			ID: proto.String(editTestTargetID), RemoteJID: proto.String(info.Chat.String()), FromMe: proto.Bool(true),
		},
	}}
	secrets := &editTestSecretStore{get: func(ctx context.Context, chat, originalSender types.JID, id types.MessageID) ([]byte, types.JID, error) {
		if chat != info.Chat || originalSender != info.Sender || id != editTestTargetID {
			t.Errorf("secret queried with changed wire identity: chat=%s sender=%s id=%s", chat, originalSender, id)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("secret lookup has no deadline")
		}
		return secret, info.Sender, nil
	}}
	return &events.Message{Info: info, Message: message, RawMessage: message}, &whatsmeow.Client{Store: &store.Device{MsgSecrets: secrets}}
}

func marshalEditFixture(t *testing.T, message *waE2E.Message) []byte {
	t.Helper()
	data, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type editWebhookCapture struct {
	WhatsmeowService
	data  chan []byte
	done  chan struct{}
	queue string
}

func (c *editWebhookCapture) CallWebhook(instance *instance_model.Instance, queue string, payload []byte) {
	defer close(c.done)
	c.queue = queue
	c.data <- append([]byte(nil), payload...)
}

func editHandlerFixture(t *testing.T, client *whatsmeow.Client) (*MyClient, *editWebhookCapture) {
	t.Helper()
	cfg := &config.Config{LogDirectory: t.TempDir(), LogMaxSize: 1}
	manager := logger_wrapper.NewLoggerManager(cfg)
	t.Cleanup(func() {
		if err := manager.GetLogger(editTestInstanceID).Close(); err != nil {
			t.Error(err)
		}
	})
	capture := &editWebhookCapture{data: make(chan []byte, 1), done: make(chan struct{})}
	return &MyClient{
		WAClient: client, Instance: &instance_model.Instance{Id: editTestInstanceID, Name: "edit-test"},
		userID: editTestInstanceID, token: "test-token", config: cfg,
		service: capture, loggerWrapper: manager,
	}, capture
}

func capturedEditWebhook(t *testing.T, capture *editWebhookCapture) map[string]interface{} {
	t.Helper()
	select {
	case <-capture.done:
	case <-time.After(3 * time.Second):
		t.Fatal("webhook worker did not complete")
	}
	var post map[string]interface{}
	if err := json.Unmarshal(<-capture.data, &post); err != nil {
		t.Fatal(err)
	}
	if post["event"] != "Message" || post["instanceId"] != editTestInstanceID {
		t.Fatal(post)
	}
	if capture.queue != editTestInstanceID+".message" {
		t.Fatalf("changed message queue: %q", capture.queue)
	}
	return post["data"].(map[string]interface{})
}
