package user_service

import (
	"context"
	"errors"
	"testing"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

type avatarPictureFake struct {
	query func(context.Context, types.JID, *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error)
}

func (f avatarPictureFake) GetProfilePictureInfo(ctx context.Context, jid types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
	return f.query(ctx, jid, params)
}

type avatarLIDsFake struct {
	store.LIDStore
	lookup func(context.Context, types.JID) (types.JID, error)
}

func (f avatarLIDsFake) GetPNForLID(ctx context.Context, jid types.JID) (types.JID, error) {
	return f.lookup(ctx, jid)
}

func TestAvatarQueryCanonicalJIDsAndParams(t *testing.T) {
	for _, tc := range []struct {
		name, input, mapped, want string
		mappingErr                error
	}{
		{"phone", "14155550100", "", "14155550100@s.whatsapp.net", nil},
		{"plus and device", "+14155550100:4@s.whatsapp.net", "", "14155550100@s.whatsapp.net", nil},
		{"LID mapping", "123:7@lid", "+14155550100:4@s.whatsapp.net", "14155550100@s.whatsapp.net", nil},
		{"unknown LID", "123@lid", "", "123@lid", nil},
		{"LID store failure", "123@lid", "", "123@lid", errors.New("storage failure")},
		{"invalid mapping", "123@lid", "999@g.us", "123@lid", nil},
		{"group", "120363000000000001@g.us", "", "120363000000000001@g.us", nil},
	} {
		for _, preview := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/full", true: "/preview"}[preview], func(t *testing.T) {
				jids, err := parseUserQueryJIDs([]string{tc.input})
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), avatarRequestTimeout)
				defer cancel()
				deadline, _ := ctx.Deadline()
				lookups := 0
				lids := avatarLIDsFake{lookup: func(gotCtx context.Context, jid types.JID) (types.JID, error) {
					lookups++
					if gotDeadline, ok := gotCtx.Deadline(); !ok || !gotDeadline.Equal(deadline) || jid.String() != "123@lid" {
						t.Fatal("LID lookup lost canonical identity or shared deadline")
					}
					if tc.mapped == "" {
						return types.EmptyJID, tc.mappingErr
					}
					return types.ParseJID(tc.mapped)
				}}
				calls := 0
				wantPicture := &types.ProfilePictureInfo{URL: "https://example.test/photo", ID: "photo", Type: "image", DirectPath: "/photo", Hash: []byte{1, 2, 3}}
				client := avatarPictureFake{query: func(gotCtx context.Context, jid types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
					calls++
					if gotDeadline, ok := gotCtx.Deadline(); !ok || !gotDeadline.Equal(deadline) {
						t.Fatal("photo IQ reset request deadline")
					}
					if jid.String() != tc.want || params.Preview != preview || params.ExistingID != "" {
						t.Fatalf("IQ = %s %+v", jid, params)
					}
					return wantPicture, nil
				}}
				pic, err := queryAvatar(ctx, client, lids, jids[0], preview)
				wantLookups := 0
				if jids[0].Server == types.HiddenUserServer {
					wantLookups = 1
				}
				if err != nil || pic != wantPicture || calls != 1 || lookups != wantLookups {
					t.Fatalf("result = %+v %v calls=%d lookups=%d", pic, err, calls, lookups)
				}
			})
		}
	}
}

func TestAvatarQueryMissingStoreAndPictures(t *testing.T) {
	lid := types.NewJID("123", types.HiddenUserServer)
	for _, tc := range []struct {
		name         string
		picture      *types.ProfilePictureInfo
		err, wantErr error
	}{
		{"missing store", &types.ProfilePictureInfo{URL: "https://example.test/photo"}, nil, nil},
		{"nil photo", nil, nil, whatsmeow.ErrProfilePictureNotSet},
		{"empty URL", &types.ProfilePictureInfo{ID: "photo"}, nil, whatsmeow.ErrProfilePictureNotSet},
		{"rate limit", nil, whatsmeow.ErrIQRateOverLimit, whatsmeow.ErrIQRateOverLimit},
		{"private photo", nil, whatsmeow.ErrProfilePictureUnauthorized, whatsmeow.ErrProfilePictureUnauthorized},
		{"missing photo", nil, whatsmeow.ErrProfilePictureNotSet, whatsmeow.ErrProfilePictureNotSet},
		{"IQ timeout", nil, whatsmeow.ErrIQTimedOut, whatsmeow.ErrIQTimedOut},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := avatarPictureFake{query: func(context.Context, types.JID, *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
				calls++
				return tc.picture, tc.err
			}}
			ctx, cancel := context.WithTimeout(context.Background(), avatarRequestTimeout)
			defer cancel()
			pic, err := queryAvatar(ctx, client, nil, lid, false)
			if !errors.Is(err, tc.wantErr) || calls != 1 {
				t.Fatalf("cause = %v, want %v, calls=%d", err, tc.wantErr, calls)
			}
			if tc.wantErr != nil && pic != nil {
				t.Fatal("failed query returned success data")
			}
		})
	}
}

func TestAvatarQueryCancellation(t *testing.T) {
	lid := types.NewJID("123", types.HiddenUserServer)
	t.Run("lookup timeout prevents IQ", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		lids := avatarLIDsFake{lookup: func(gotCtx context.Context, _ types.JID) (types.JID, error) {
			if _, ok := gotCtx.Deadline(); !ok {
				t.Fatal("unbounded LID lookup")
			}
			<-gotCtx.Done()
			return types.EmptyJID, gotCtx.Err()
		}}
		client := avatarPictureFake{query: func(context.Context, types.JID, *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
			t.Fatal("photo requested after lookup timeout")
			return nil, nil
		}}
		_, err := queryAvatar(ctx, client, lids, lid, false)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cause = %v", err)
		}
	})
	t.Run("IQ timeout has no retry", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		calls := 0
		client := avatarPictureFake{query: func(gotCtx context.Context, _ types.JID, _ *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
			calls++
			<-gotCtx.Done()
			return nil, gotCtx.Err()
		}}
		_, err := queryAvatar(ctx, client, nil, lid, false)
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("cause = %v calls=%d", err, calls)
		}
	})
	t.Run("canceled request does no lookup", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		lids := avatarLIDsFake{lookup: func(context.Context, types.JID) (types.JID, error) {
			t.Fatal("lookup after cancellation")
			return types.EmptyJID, nil
		}}
		_, err := queryAvatar(ctx, avatarPictureFake{}, lids, lid, false)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cause = %v", err)
		}
	})
	t.Run("late success is rejected", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		client := avatarPictureFake{query: func(context.Context, types.JID, *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
			cancel()
			return &types.ProfilePictureInfo{URL: "https://example.test/photo"}, nil
		}}
		pic, err := queryAvatar(ctx, client, nil, lid, false)
		if !errors.Is(err, context.Canceled) || pic != nil {
			t.Fatalf("late success = %+v %v", pic, err)
		}
	})
}

type avatarSessionsFake struct {
	whatsmeow_service.WhatsmeowService
	start func(context.Context, string) error
}

func (f avatarSessionsFake) GetClient(string) *whatsmeow.Client { return nil }

func (f avatarSessionsFake) StartInstanceContext(ctx context.Context, id string) error {
	return f.start(ctx, id)
}

func TestAvatarRequestBudgetAndValidation(t *testing.T) {
	instance := &instance_model.Instance{Id: "one"}
	sessions := avatarSessionsFake{start: func(ctx context.Context, id string) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > avatarRequestTimeout || id != instance.Id {
			t.Fatal("startup lost instance or avatar deadline")
		}
		return context.DeadlineExceeded
	}}
	u := &userService{whatsmeowService: sessions}
	if _, err := u.GetAvatar(context.Background(), &GetAvatarStruct{Number: "14155550100"}, instance); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("startup cause = %v", err)
	}
	sessions.start = func(context.Context, string) error { t.Fatal("startup for invalid or canceled request"); return nil }
	u.whatsmeowService = sessions
	for _, data := range []*GetAvatarStruct{nil, {}, {Number: "not a phone"}, {Number: "abc@s.whatsapp.net"}, {Number: "abc@lid"}} {
		if _, err := u.GetAvatar(context.Background(), data, instance); !errors.Is(err, ErrInvalidUserNumber) {
			t.Fatalf("invalid input = %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := u.GetAvatar(ctx, &GetAvatarStruct{Number: "14155550100"}, instance); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel cause = %v", err)
	}
	if _, err := u.waitForClientReady(context.Background(), "one", time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("readiness cause = %v", err)
	}
}
