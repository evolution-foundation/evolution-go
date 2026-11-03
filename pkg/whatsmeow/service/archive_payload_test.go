package whatsmeow_service

import (
	"encoding/json"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestArchivePayload(t *testing.T) {
	jid := types.NewJID("5511999999999", types.DefaultUserServer)
	ts := time.Unix(1_700_000_000, 0)
	evt := &events.Archive{
		JID:          jid,
		Timestamp:    ts,
		Action:       &waSyncAction.ArchiveChatAction{Archived: proto.Bool(true)},
		FromFullSync: true,
	}

	payload := archivePayload(evt)
	if payload["JID"] != jid || payload["Timestamp"] != ts || payload["FromFullSync"] != true {
		t.Fatalf("payload = %+v", payload)
	}
	if _, err := json.Marshal(payload); err != nil {
		t.Fatalf("payload must be JSON-serializable: %v", err)
	}
}
