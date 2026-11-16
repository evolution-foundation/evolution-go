package user_service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

const (
	avatarRequestTimeout   = 8 * time.Second
	clientReadyWait        = 2 * time.Second
	userInfoRequestTimeout = 10 * time.Second
	pictureURLEnrichBudget = 5 * time.Second
)

// ErrInvalidUserNumber identifies missing or malformed query input.
var ErrInvalidUserNumber = errors.New("invalid phone number")

type profileQueryClient interface {
	GetUserInfo(context.Context, []types.JID) (map[types.JID]types.UserInfo, error)
	GetProfilePictureInfo(context.Context, types.JID, *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error)
}

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

func (u *userService) GetUser(ctx context.Context, data *CheckUserStruct, instance *instance_model.Instance) (*UserCollection, error) {
	if data == nil || instance == nil {
		return nil, ErrInvalidUserNumber
	}
	jids, err := parseUserQueryJIDs(data.Number)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(queryContext(ctx), userInfoRequestTimeout+pictureURLEnrichBudget)
	defer cancel()
	client, err := u.ensureClientConnectedCtx(ctx, instance.Id)
	if err != nil {
		return nil, err
	}
	if client.Store.LIDs == nil {
		// The pinned GetUserInfo implementation persists LID mappings even
		// when the response is empty, so a missing store would panic there.
		return nil, errors.New("user LID store unavailable")
	}
	return queryUserInfo(ctx, client, client.Store.LIDs, jids)
}

func queryUserInfo(ctx context.Context, client profileQueryClient, lids store.LIDStore, jids []types.JID) (*UserCollection, error) {
	usyncCtx, cancel := context.WithTimeout(ctx, userInfoRequestTimeout)
	defer cancel()
	resolved := make([]types.JID, 0, len(jids))
	seen := make(map[types.JID]bool, len(jids))
	for _, jid := range jids {
		jid, err := resolveQueryJID(usyncCtx, lids, jid)
		if err != nil {
			return nil, err
		}
		if !seen[jid] {
			seen[jid] = true
			resolved = append(resolved, jid)
		}
	}
	resp, err := client.GetUserInfo(usyncCtx, resolved)
	if err != nil {
		return nil, fmt.Errorf("query user info: %w", err)
	}
	uc := &UserCollection{Users: make(map[types.JID]UserInfo, len(resp))}
	keys := make([]types.JID, 0, len(resp))
	for jid, info := range resp {
		converted := UserInfo{VerifiedName: info.VerifiedName, Status: info.Status, PictureID: info.PictureID, Devices: info.Devices}
		if !info.LID.IsEmpty() {
			lid := info.LID.String()
			converted.LID = &lid
		}
		uc.Users[jid] = converted
		keys = append(keys, jid)
	}
	// Build every base entry first; optional work can stop without losing users.
	// Stable ordering avoids random selection of photos when the budget expires.
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	enrichCtx, enrichCancel := context.WithTimeout(ctx, pictureURLEnrichBudget)
	defer enrichCancel()
	for _, jid := range keys {
		if enrichCtx.Err() != nil {
			break
		}
		info := uc.Users[jid]
		if info.LID == nil && lids != nil && jid.Server == types.DefaultUserServer {
			if lid, lidErr := lids.GetLIDForPN(enrichCtx, jid); lidErr == nil && !lid.IsEmpty() {
				lidString := lid.String()
				info.LID = &lidString
			}
		}
		if info.PictureID != "" && enrichCtx.Err() == nil {
			pic, picErr := fetchProfilePicture(enrichCtx, client, jid, true)
			if picErr == nil && pic != nil {
				info.PictureURL = pic.URL
			}
			uc.Users[jid] = info
			if errors.Is(picErr, whatsmeow.ErrIQRateOverLimit) {
				break
			}
		} else {
			uc.Users[jid] = info
		}
	}
	return uc, nil
}

// ExistingID is deliberately empty: unchanged-picture responses otherwise omit
// the URL. The caller owns the deadline shared with lookup and enrichment.
func fetchProfilePicture(ctx context.Context, client profileQueryClient, jid types.JID, preview bool) (*types.ProfilePictureInfo, error) {
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
	jid, err := resolveQueryJID(ctx, client.Store.LIDs, jids[0])
	if err != nil {
		return nil, err
	}
	pic, err := fetchProfilePicture(ctx, client, jid, data.Preview)
	if err != nil {
		return nil, err
	}
	if pic == nil {
		return nil, errors.New("no profile picture found")
	}
	return pic, nil
}
