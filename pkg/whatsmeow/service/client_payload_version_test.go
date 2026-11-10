package whatsmeow_service

import (
	"testing"

	"go.mau.fi/whatsmeow/store"
)

// VM4 (10/11/2026): a versão escolhida (variáveis ou web) tem de chegar ao ClientPayload,
// que é o que o WhatsApp confere no login. Antes só ia para DeviceProps e o login seguia
// com a versão fixa da biblioteca (405 "client outdated").
func TestApplyClientPayloadVersion(t *testing.T) {
	original := store.GetWAVersion()
	t.Cleanup(func() { store.SetWAVersion(original) })

	var logs []string
	logf := func(m string) { logs = append(logs, m) }

	if applyClientPayloadVersion(clientVersion{}, logf) {
		t.Fatal("versão zerada não pode ser aplicada")
	}
	if store.GetWAVersion() != original {
		t.Fatal("versão zerada mudou a versão da biblioteca")
	}

	if !applyClientPayloadVersion(clientVersion{Major: 2, Minor: 3000, Patch: 1053787253}, logf) {
		t.Fatal("versão válida não foi aplicada")
	}
	want := store.WAVersionContainer{2, 3000, 1053787253}
	if got := store.GetWAVersion(); got != want {
		t.Fatalf("GetWAVersion = %s, quer %s", got, want)
	}
	if got := store.BaseClientPayload.GetUserAgent().GetAppVersion().GetTertiary(); got != 1053787253 {
		t.Fatalf("ClientPayload.UserAgent.AppVersion.Tertiary = %d, quer 1053787253", got)
	}
	if len(logs) != 2 {
		t.Fatalf("esperava 2 linhas de log, veio %d: %v", len(logs), logs)
	}
}
