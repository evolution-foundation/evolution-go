package user_service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

type queryClientFake struct {
	info    func(context.Context, []types.JID) (map[types.JID]types.UserInfo, error)
	picture func(context.Context, types.JID, *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error)
}

func (f queryClientFake) GetUserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
	return f.info(ctx, jids)
}

func (f queryClientFake) GetProfilePictureInfo(ctx context.Context, jid types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
	return f.picture(ctx, jid, params)
}

type queryLIDsFake struct {
	store.LIDStore
	pn  func(context.Context, types.JID) (types.JID, error)
	lid func(context.Context, types.JID) (types.JID, error)
}

func (f queryLIDsFake) GetPNForLID(ctx context.Context, jid types.JID) (types.JID, error) {
	return f.pn(ctx, jid)
}

func (f queryLIDsFake) GetLIDForPN(ctx context.Context, jid types.JID) (types.JID, error) {
	return f.lid(ctx, jid)
}

func requireQueryDeadline(t *testing.T, ctx context.Context, maximum time.Duration) time.Time {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > maximum {
		t.Fatalf("missing or excessive query deadline: %v %v", deadline, ok)
	}
	return deadline
}

func TestUserQueryCanonicalizationAndLID(t *testing.T) {
	input := []string{"+14155550100@s.whatsapp.net", "14155550100:4@s.whatsapp.net", "123@lid"}
	jids, err := parseUserQueryJIDs(input)
	if err != nil {
		t.Fatal(err)
	}
	pn := types.NewJID("14155550100", types.DefaultUserServer)
	lid := types.NewJID("123", types.HiddenUserServer)
	var lookupDeadline time.Time
	lids := queryLIDsFake{pn: func(ctx context.Context, got types.JID) (types.JID, error) {
		lookupDeadline = requireQueryDeadline(t, ctx, userInfoRequestTimeout)
		if got != lid {
			t.Fatalf("LID lookup = %s", got)
		}
		return pn, nil
	}, lid: func(context.Context, types.JID) (types.JID, error) {
		t.Fatal("response LID should be preserved without an extra lookup")
		return types.EmptyJID, nil
	}}
	var pictureDeadline time.Time
	client := queryClientFake{info: func(ctx context.Context, got []types.JID) (map[types.JID]types.UserInfo, error) {
		if !reflect.DeepEqual(got, []types.JID{pn}) {
			t.Fatalf("usync JIDs = %v", got)
		}
		if deadline := requireQueryDeadline(t, ctx, userInfoRequestTimeout); !deadline.Equal(lookupDeadline) {
			t.Fatal("LID lookup did not share usync deadline")
		}
		return map[types.JID]types.UserInfo{pn: {PictureID: "photo", Status: "hello", LID: lid, Devices: []types.JID{pn}}}, nil
	}, picture: func(ctx context.Context, got types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
		pictureDeadline = requireQueryDeadline(t, ctx, pictureURLEnrichBudget)
		if got != pn || !params.Preview || params.ExistingID != "" {
			t.Fatalf("picture = %s %+v", got, params)
		}
		return &types.ProfilePictureInfo{URL: "https://example.test/photo", ID: "photo"}, nil
	}}
	uc, err := queryUserInfo(context.Background(), client, lids, jids)
	if err != nil {
		t.Fatal(err)
	}
	info := uc.Users[pn]
	if info.PictureURL != "https://example.test/photo" || info.PictureID != "photo" || info.Status != "hello" || info.LID == nil || *info.LID != lid.String() || len(info.Devices) != 1 {
		t.Fatalf("info = %+v", info)
	}
	if pictureDeadline.IsZero() || !reflect.DeepEqual(input, []string{"+14155550100@s.whatsapp.net", "14155550100:4@s.whatsapp.net", "123@lid"}) {
		t.Fatal("missing picture call or changed caller input")
	}
}

func TestUserInfoPreservesBaseEntriesOnOptionalFailures(t *testing.T) {
	first := types.NewJID("100", types.DefaultUserServer)
	second := types.NewJID("200", types.DefaultUserServer)
	third := types.NewJID("300", types.DefaultUserServer)
	lid := types.NewJID("123", types.HiddenUserServer)
	for _, tc := range []struct {
		name       string
		pictureErr error
		wantCalls  int
	}{
		{"rate limit", fmt.Errorf("picture: %w", whatsmeow.ErrIQRateOverLimit), 1},
		{"private photo", whatsmeow.ErrProfilePictureUnauthorized, 2},
		{"missing photo", whatsmeow.ErrProfilePictureNotSet, 2},
		{"nil photo", nil, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var sharedDeadline time.Time
			client := queryClientFake{info: func(context.Context, []types.JID) (map[types.JID]types.UserInfo, error) {
				return map[types.JID]types.UserInfo{first: {PictureID: "a", Status: "first", LID: lid}, second: {PictureID: "b", Status: "second", LID: lid}, third: {Status: "third", LID: lid}}, nil
			}, picture: func(ctx context.Context, jid types.JID, _ *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
				deadline := requireQueryDeadline(t, ctx, pictureURLEnrichBudget)
				if calls == 0 {
					sharedDeadline = deadline
					if jid != first {
						t.Fatalf("first picture = %s", jid)
					}
				} else if !sharedDeadline.Equal(deadline) {
					t.Fatal("picture budget was reset")
				}
				calls++
				return nil, tc.pictureErr
			}}
			uc, err := queryUserInfo(context.Background(), client, nil, []types.JID{first, second, third})
			if err != nil || len(uc.Users) != 3 || calls != tc.wantCalls {
				t.Fatalf("result=%+v err=%v calls=%d", uc, err, calls)
			}
			for jid, info := range uc.Users {
				if info.PictureURL != "" || info.Status == "" || info.LID == nil {
					t.Fatalf("lost base info for %s: %+v", jid, info)
				}
			}
		})
	}
}

func TestUserQueryLIDFallbacks(t *testing.T) {
	lid := types.NewJID("123", types.HiddenUserServer)
	for _, tc := range []struct {
		name string
		pn   types.JID
		err  error
	}{
		{"no mapping", types.EmptyJID, nil},
		{"store error", types.EmptyJID, errors.New("private storage details")},
		{"invalid mapping", types.NewJID("999", types.GroupServer), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lids := queryLIDsFake{pn: func(ctx context.Context, got types.JID) (types.JID, error) {
				requireQueryDeadline(t, ctx, userInfoRequestTimeout)
				return tc.pn, tc.err
			}}
			client := queryClientFake{info: func(ctx context.Context, got []types.JID) (map[types.JID]types.UserInfo, error) {
				if !reflect.DeepEqual(got, []types.JID{lid}) {
					t.Fatalf("fallback = %v", got)
				}
				return nil, nil
			}}
			uc, err := queryUserInfo(context.Background(), client, lids, []types.JID{lid})
			if err != nil || uc.Users == nil || len(uc.Users) != 0 {
				t.Fatalf("empty result = %+v, %v", uc, err)
			}
		})
	}
}

func TestUserQueryCancellationAndBudgets(t *testing.T) {
	jid := types.NewJID("123", types.HiddenUserServer)
	t.Run("lookup cancellation prevents usync", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		lids := queryLIDsFake{pn: func(lookupCtx context.Context, _ types.JID) (types.JID, error) {
			requireQueryDeadline(t, lookupCtx, userInfoRequestTimeout)
			cancel()
			<-lookupCtx.Done()
			return types.EmptyJID, lookupCtx.Err()
		}}
		client := queryClientFake{info: func(context.Context, []types.JID) (map[types.JID]types.UserInfo, error) {
			t.Fatal("usync after cancellation")
			return nil, nil
		}}
		_, err := queryUserInfo(ctx, client, lids, []types.JID{jid})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cause = %v", err)
		}
	})
	t.Run("lookup deadline prevents usync", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		lids := queryLIDsFake{pn: func(lookupCtx context.Context, _ types.JID) (types.JID, error) {
			requireQueryDeadline(t, lookupCtx, 20*time.Millisecond)
			<-lookupCtx.Done()
			return types.EmptyJID, lookupCtx.Err()
		}}
		client := queryClientFake{info: func(context.Context, []types.JID) (map[types.JID]types.UserInfo, error) {
			t.Fatal("usync after deadline")
			return nil, nil
		}}
		_, err := queryUserInfo(ctx, client, lids, []types.JID{jid})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cause = %v", err)
		}
	})
	for _, optionalStage := range []string{"LID", "picture"} {
		t.Run("optional "+optionalStage+" cancellation keeps users", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pn := types.NewJID("100", types.DefaultUserServer)
			other := types.NewJID("200", types.DefaultUserServer)
			calls := 0
			lids := queryLIDsFake{lid: func(lookupCtx context.Context, _ types.JID) (types.JID, error) {
				requireQueryDeadline(t, lookupCtx, pictureURLEnrichBudget)
				cancel()
				<-lookupCtx.Done()
				return types.EmptyJID, lookupCtx.Err()
			}}
			client := queryClientFake{info: func(context.Context, []types.JID) (map[types.JID]types.UserInfo, error) {
				return map[types.JID]types.UserInfo{pn: {PictureID: "a", Status: "first"}, other: {PictureID: "b", Status: "second"}}, nil
			}, picture: func(picCtx context.Context, _ types.JID, _ *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
				calls++
				requireQueryDeadline(t, picCtx, pictureURLEnrichBudget)
				cancel()
				<-picCtx.Done()
				return nil, picCtx.Err()
			}}
			var storeToUse store.LIDStore = lids
			wantCalls := 0
			if optionalStage == "picture" {
				storeToUse = nil
				wantCalls = 1
			}
			uc, err := queryUserInfo(ctx, client, storeToUse, []types.JID{pn, other})
			if err != nil || len(uc.Users) != 2 || calls != wantCalls || uc.Users[other].Status != "second" {
				t.Fatalf("optional result=%+v err=%v calls=%d", uc, err, calls)
			}
		})
	}
	t.Run("usync error cause", func(t *testing.T) {
		client := queryClientFake{info: func(context.Context, []types.JID) (map[types.JID]types.UserInfo, error) {
			return nil, whatsmeow.ErrIQRateOverLimit
		}}
		_, err := queryUserInfo(context.Background(), client, nil, []types.JID{jid})
		if !errors.Is(err, whatsmeow.ErrIQRateOverLimit) {
			t.Fatalf("cause = %v", err)
		}
	})
}

func TestFetchProfilePictureCanonicalParams(t *testing.T) {
	for _, preview := range []bool{true, false} {
		t.Run(fmt.Sprint(preview), func(t *testing.T) {
			client := queryClientFake{picture: func(ctx context.Context, got types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
				if got.String() != "14155550100@s.whatsapp.net" || params.Preview != preview || params.ExistingID != "" {
					t.Fatalf("params = %s %+v", got, params)
				}
				return nil, whatsmeow.ErrProfilePictureNotSet
			}}
			jid, _ := types.ParseJID("+14155550100:4@s.whatsapp.net")
			_, err := fetchProfilePicture(context.Background(), client, jid, preview)
			if !errors.Is(err, whatsmeow.ErrProfilePictureNotSet) {
				t.Fatalf("cause = %v", err)
			}
		})
	}
}

type querySessionFake struct {
	whatsmeow_service.WhatsmeowService
	clients      sync.Map
	start        func(string) error
	startContext func(context.Context, string) error
}

func (f *querySessionFake) GetClient(id string) *whatsmeow.Client {
	client, ok := f.clients.Load(id)
	if !ok {
		return nil
	}
	return client.(*whatsmeow.Client)
}

func (f *querySessionFake) StartInstance(id string) error { return f.start(id) }

func (f *querySessionFake) StartInstanceContext(ctx context.Context, id string) error {
	if f.startContext != nil {
		return f.startContext(ctx, id)
	}
	return f.start(id)
}

func TestUserRequestDeadlineReachesStartup(t *testing.T) {
	for _, name := range []string{"info", "avatar"} {
		t.Run(name, func(t *testing.T) {
			wantBudget := avatarRequestTimeout
			if name == "info" {
				wantBudget = userInfoRequestTimeout + pictureURLEnrichBudget
			}
			sessions := &querySessionFake{startContext: func(ctx context.Context, id string) error {
				if id != "one" {
					t.Fatalf("startup instance = %s", id)
				}
				requireQueryDeadline(t, ctx, wantBudget)
				return context.DeadlineExceeded
			}}
			u := &userService{whatsmeowService: sessions}
			instance := &instance_model.Instance{Id: "one"}
			var err error
			if name == "info" {
				_, err = u.GetUser(context.Background(), &CheckUserStruct{Number: []string{"14155550100"}}, instance)
			} else {
				_, err = u.GetAvatar(context.Background(), &GetAvatarStruct{Number: "14155550100"}, instance)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("startup cause = %v", err)
			}
		})
	}
}

func TestUserSessionErrorsAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sessions := &querySessionFake{start: func(string) error { t.Fatal("startup on canceled request"); return nil }}
	u := &userService{whatsmeowService: sessions}
	if _, err := u.ensureClientConnectedCtx(ctx, "one"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cause = %v", err)
	}
	sessions.start = func(id string) error {
		if id != "one" {
			t.Fatalf("wrong instance: %s", id)
		}
		return fmt.Errorf("startup: %w", context.DeadlineExceeded)
	}
	if _, err := u.ensureClientConnectedCtx(context.Background(), "one"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("startup cause = %v", err)
	}
	sessions.start = func(string) error { cancel(); return nil }
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	if _, err := u.ensureClientConnectedCtx(ctx, "one"); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait cause = %v", err)
	}
	if _, err := u.waitForClientReady(context.Background(), "one", time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("readiness cause = %v", err)
	}
	instance := &instance_model.Instance{Id: "one"}
	for _, data := range []*CheckUserStruct{nil, {}, {Number: []string{""}}, {Number: []string{"not a phone"}}, {Number: []string{"abc@s.whatsapp.net"}}, {Number: []string{"abc@lid"}}} {
		if _, err := u.GetUser(context.Background(), data, instance); !errors.Is(err, ErrInvalidUserNumber) {
			t.Fatalf("invalid input = %v", err)
		}
	}
	for _, data := range []*GetAvatarStruct{nil, {}, {Number: "not a phone"}, {Number: "abc@s.whatsapp.net"}} {
		if _, err := u.GetAvatar(context.Background(), data, instance); !errors.Is(err, ErrInvalidUserNumber) {
			t.Fatalf("invalid avatar = %v", err)
		}
	}
	// A disconnected client from another instance must never satisfy readiness.
	sessions.clients.Store("two", &whatsmeow.Client{})
	if client, err := u.waitForClientReady(ctx, "one", time.Millisecond); client != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("other session = %v %v", client, err)
	}
}
