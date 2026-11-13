package user_service

import (
	"context"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

type profilePictureClient interface {
	GetProfilePictureInfo(context.Context, types.JID, *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error)
}

// queryAvatar shares the caller's request budget across LID lookup and the IQ.
// It returns only usable URLs and never downloads the image or retries the query.
func queryAvatar(ctx context.Context, client profilePictureClient, lids store.LIDStore, jid types.JID, preview bool) (*types.ProfilePictureInfo, error) {
	jid, err := resolveQueryJID(ctx, lids, jid)
	if err != nil {
		return nil, err
	}
	pic, err := fetchProfilePicture(ctx, client, jid, preview)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if pic == nil || pic.URL == "" {
		return nil, whatsmeow.ErrProfilePictureNotSet
	}
	return pic, nil
}
