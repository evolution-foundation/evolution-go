package whatsmeow_service

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types/events"
)

// Recuperação de app state (arquivados, fixados, silenciados, etiquetas, ...).
//
// O whatsmeow só emite events.AppStateSyncError quando um patch não confere
// (ex.: "mismatching LTHash") e não tenta se recuperar; sem tratamento a
// coleção fica quebrada até um novo pareamento. Sequência aplicada, a mesma
// documentada pelo whatsmeow (ref. upstream PR #144):
//  1. falha no sync incremental -> refaz um sync completo da coleção;
//  2. falha no sync completo -> pede ao celular um snapshot de recuperação
//     (BuildAppStateRecoveryRequest). Nunca usa a notificação de recuperação
//     fatal, que desvincula todos os aparelhos e exige QR novo.
// Cada etapa tem cooldown por coleção para não entrar em laço.

const appStateRecoveryCooldown = 15 * time.Minute

type appStateRecoveryAttempt struct {
	fullSyncAt        time.Time
	recoveryRequestAt time.Time
}

type appStateRecoveryState struct {
	mu       sync.Mutex
	attempts map[appstate.WAPatchName]appStateRecoveryAttempt
}

// reserve marca a etapa como tentada agora e retorna false se ela já foi
// tentada para essa coleção dentro do cooldown.
func (s *appStateRecoveryState) reserve(name appstate.WAPatchName, recoveryRequest bool, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.attempts == nil {
		s.attempts = make(map[appstate.WAPatchName]appStateRecoveryAttempt)
	}
	attempt := s.attempts[name]
	last := attempt.fullSyncAt
	if recoveryRequest {
		last = attempt.recoveryRequestAt
	}
	if !last.IsZero() && now.Sub(last) < appStateRecoveryCooldown {
		return false
	}
	if recoveryRequest {
		attempt.recoveryRequestAt = now
	} else {
		attempt.fullSyncAt = now
	}
	s.attempts[name] = attempt
	return true
}

// isTransientAppStateError indica erros do servidor/rede (IQ, ex.: 429), que
// não significam estado corrompido e não justificam pedir snapshot ao celular.
func isTransientAppStateError(err error) bool {
	var iqErr *whatsmeow.IQError
	return errors.As(err, &iqErr) || errors.Is(err, whatsmeow.ErrAppStateRateLimited)
}

func (mycli *MyClient) handleAppStateSyncError(evt *events.AppStateSyncError) {
	if evt == nil || mycli.WAClient == nil {
		return
	}
	logger := mycli.loggerWrapper.GetLogger(mycli.userID)

	if evt.FullSync && isTransientAppStateError(evt.Error) {
		logger.LogWarn("[%s] App-state full sync for %s failed with a server error, not requesting recovery: %v",
			mycli.userID, evt.Name, evt.Error)
		return
	}

	recoveryRequest := evt.FullSync
	if !mycli.appStateRecovery.reserve(evt.Name, recoveryRequest, time.Now()) {
		logger.LogInfo("[%s] App-state recovery already attempted recently for %s (fullSync=%t)",
			mycli.userID, evt.Name, evt.FullSync)
		return
	}

	client := mycli.WAClient
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if !recoveryRequest {
			logger.LogWarn("[%s] App-state sync failed for %s (%v); starting full sync",
				mycli.userID, evt.Name, evt.Error)
			if err := client.FetchAppState(ctx, evt.Name, true, false); err != nil {
				// Se o sync completo também falhar na verificação, o whatsmeow
				// emite um novo AppStateSyncError com FullSync=true (etapa 2).
				logger.LogWarn("[%s] App-state full sync for %s did not complete: %v", mycli.userID, evt.Name, err)
			}
			return
		}

		logger.LogWarn("[%s] App-state full sync failed for %s (%v); requesting recovery snapshot from the phone",
			mycli.userID, evt.Name, evt.Error)
		if _, err := client.SendPeerMessage(ctx, whatsmeow.BuildAppStateRecoveryRequest(evt.Name)); err != nil {
			logger.LogError("[%s] Failed to send app-state recovery request for %s: %v", mycli.userID, evt.Name, err)
			return
		}
		logger.LogInfo("[%s] App-state recovery request sent for %s", mycli.userID, evt.Name)
	}()
}
