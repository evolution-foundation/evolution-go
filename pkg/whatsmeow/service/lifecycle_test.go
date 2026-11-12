package whatsmeow_service

import (
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/runtimestate"
)

func TestStartClientRejectsStoppedGenerationBeforeExternalWork(t *testing.T) {
	signal := runtimestate.NewStopSignal()
	signal.Stop()

	var service whatsmeowService
	service.StartClient(&ClientData{StopSignal: signal})
}
