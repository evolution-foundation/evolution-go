package whatsmeow_service

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
)

func TestKeepAliveExpired(t *testing.T) {
	now := time.Now()
	if keepAliveExpired(now.Add(-30*time.Second), now) {
		t.Fatal("30s sem keepalive não deve forçar reconexão")
	}
	if !keepAliveExpired(now.Add(-whatsmeow.KeepAliveMaxFailTime-time.Second), now) {
		t.Fatal("acima de KeepAliveMaxFailTime deve forçar reconexão")
	}
}
