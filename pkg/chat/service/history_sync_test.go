package chat_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

type recordingPeerSender struct {
	sent []*waE2E.Message
	err  error
}

func (r *recordingPeerSender) BuildHistorySyncRequest(info *types.MessageInfo, count int) *waE2E.Message {
	return (&whatsmeow.Client{}).BuildHistorySyncRequest(info, count)
}

func (r *recordingPeerSender) SendPeerMessage(_ context.Context, message *waE2E.Message) (whatsmeow.SendResponse, error) {
	r.sent = append(r.sent, message)
	return whatsmeow.SendResponse{ID: "PEER1"}, r.err
}

func TestRequestHistorySyncSendsToOwnPrimaryDevice(t *testing.T) {
	sender := &recordingPeerSender{}
	chat := types.NewJID("95336127295713", types.HiddenUserServer)
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, IsFromMe: false},
		ID:            "3A26EBAB519B7497FCC1",
		Timestamp:     time.Unix(1760144000, 0),
	}

	res, err := requestHistorySync(context.Background(), sender, info, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ID != "PEER1" {
		t.Fatalf("expected peer response id, got %q", res.ID)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected one peer message, got %d", len(sender.sent))
	}

	request := sender.sent[0].GetProtocolMessage().GetPeerDataOperationRequestMessage().GetHistorySyncOnDemandRequest()
	if request.GetChatJID() != chat.String() {
		t.Errorf("expected chat %q, got %q", chat.String(), request.GetChatJID())
	}
	if request.GetOldestMsgID() != info.ID {
		t.Errorf("expected oldest message %q, got %q", info.ID, request.GetOldestMsgID())
	}
	if request.GetOnDemandMsgCount() != 30 {
		t.Errorf("expected count 30, got %d", request.GetOnDemandMsgCount())
	}
	if request.GetOldestMsgTimestampMS() != info.Timestamp.Unix() {
		t.Errorf("expected timestamp %d, got %d", info.Timestamp.Unix(), request.GetOldestMsgTimestampMS())
	}
}

func TestRequestHistorySyncPropagatesSendError(t *testing.T) {
	sender := &recordingPeerSender{err: errors.New("not logged in")}
	info := &types.MessageInfo{ID: "X", Timestamp: time.Unix(1, 0)}

	if _, err := requestHistorySync(context.Background(), sender, info, 10); err == nil {
		t.Fatal("expected error to be propagated")
	}
}
