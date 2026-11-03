package server_handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/store"
)

func TestServerInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	startedAt := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	handler := NewServerHandler(ServerInfo{Version: "1.2.3", StartedAt: startedAt})

	engine := gin.New()
	engine.GET("/instance/server-info", handler.ServerInfo)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/instance/server-info", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["version"] != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", body["version"])
	}
	if body["startedAt"] != startedAt.Format(time.RFC3339) {
		t.Errorf("startedAt = %q, want %q", body["startedAt"], startedAt.Format(time.RFC3339))
	}
	if body["waVersion"] != store.GetWAVersion().String() {
		t.Errorf("waVersion = %q, want %q", body["waVersion"], store.GetWAVersion().String())
	}
}

func TestServerOk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/server/ok", NewServerHandler(ServerInfo{}).ServerOk)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/server/ok", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":"ok"}` {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}
