package user_service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

const (
	avatarRequestTimeout = 8 * time.Second
	clientReadyWait      = 2 * time.Second
)

// ErrInvalidUserNumber identifies missing or malformed query input.
var ErrInvalidUserNumber = errors.New("invalid phone number")

func queryContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (u *userService) ensureClientConnectedCtx(ctx context.Context, instanceID string) (*whatsmeow.Client, error) {
	ctx = queryContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client := u.whatsmeowService.GetClient(instanceID)
	if client == nil {
		if err := u.whatsmeowService.StartInstanceContext(ctx, instanceID); err != nil {
			return nil, fmt.Errorf("start user session: %w", err)
		}
		var err error
		client, err = u.waitForClientReady(ctx, instanceID, clientReadyWait)
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !client.IsConnected() {
		return nil, errors.New("client disconnected")
	}
	if client.Store == nil || !client.IsLoggedIn() {
		return nil, errors.New("client is not logged in to WhatsApp")
	}
	return client, nil
}

func (u *userService) waitForClientReady(ctx context.Context, instanceID string, maxWait time.Duration) (*whatsmeow.Client, error) {
	ctx, cancel := context.WithTimeout(queryContext(ctx), maxWait)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("waiting for client: %w", err)
		}
		client := u.whatsmeowService.GetClient(instanceID)
		if client != nil && client.Store != nil && client.IsConnected() && client.IsLoggedIn() {
			return client, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for client: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func parseUserQueryJIDs(numbers []string) ([]types.JID, error) {
	if len(numbers) == 0 {
		return nil, ErrInvalidUserNumber
	}
	jids := make([]types.JID, 0, len(numbers))
	for _, number := range numbers {
		jid, ok := utils.ParseJID(number)
		if !ok {
			return nil, ErrInvalidUserNumber
		}
		jid = utils.CanonicalJID(jid).ToNonAD()
		if (jid.Server == types.DefaultUserServer || jid.Server == types.HiddenUserServer) &&
			(jid.User == "" || strings.Trim(jid.User, "0123456789") != "") {
			return nil, ErrInvalidUserNumber
		}
		jids = append(jids, jid)
	}
	return jids, nil
}

func resolveQueryJID(ctx context.Context, lids store.LIDStore, jid types.JID) (types.JID, error) {
	if err := ctx.Err(); err != nil {
		return types.EmptyJID, err
	}
	if jid.Server == types.HiddenUserServer && lids != nil {
		pn, err := lids.GetPNForLID(ctx, jid)
		if ctx.Err() != nil {
			return types.EmptyJID, ctx.Err()
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return types.EmptyJID, err
		}
		// An absent/failed mapping keeps the original LID. Never query a
		// different session store or replace it with an empty JID.
		if err == nil && !pn.IsEmpty() && pn.Server == types.DefaultUserServer {
			jid = utils.CanonicalJID(pn).ToNonAD()
		}
	}
	return jid, nil
}

// ExistingID is deliberately empty: unchanged-picture responses otherwise omit
// the URL. The caller owns the deadline shared with lookup and enrichment.
func fetchProfilePicture(ctx context.Context, client profilePictureClient, jid types.JID, preview bool) (*types.ProfilePictureInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pic, err := client.GetProfilePictureInfo(ctx, utils.CanonicalJID(jid).ToNonAD(), &whatsmeow.GetProfilePictureParams{Preview: preview})
	if err != nil {
		return nil, fmt.Errorf("get profile picture: %w", err)
	}
	return pic, nil
}

func (u *userService) GetAvatar(ctx context.Context, data *GetAvatarStruct, instance *instance_model.Instance) (*types.ProfilePictureInfo, error) {
	if data == nil || instance == nil {
		return nil, ErrInvalidUserNumber
	}
	jids, err := parseUserQueryJIDs([]string{data.Number})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(queryContext(ctx), avatarRequestTimeout)
	defer cancel()
	client, err := u.ensureClientConnectedCtx(ctx, instance.Id)
	if err != nil {
		return nil, err
	}
	return queryAvatar(ctx, client, client.Store.LIDs, jids[0], data.Preview)
}
