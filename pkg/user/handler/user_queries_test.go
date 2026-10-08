package user_handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	auth_middleware "github.com/evolution-foundation/evolution-go/pkg/middleware"
	user_service "github.com/evolution-foundation/evolution-go/pkg/user/service"
	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type queryServiceFake struct {
	user_service.UserService
	info   func(context.Context, *user_service.CheckUserStruct, *instance_model.Instance) (*user_service.UserCollection, error)
	avatar func(context.Context, *user_service.GetAvatarStruct, *instance_model.Instance) (*types.ProfilePictureInfo, error)
}

func (f queryServiceFake) GetUser(ctx context.Context, data *user_service.CheckUserStruct, instance *instance_model.Instance) (*user_service.UserCollection, error) {
	return f.info(ctx, data, instance)
}

func (f queryServiceFake) GetAvatar(ctx context.Context, data *user_service.GetAvatarStruct, instance *instance_model.Instance) (*types.ProfilePictureInfo, error) {
	return f.avatar(ctx, data, instance)
}

func userQueryRouter(service user_service.UserService, instance *instance_model.Instance) *gin.Engine {
	r := gin.New()
	r.Use(func(ctx *gin.Context) { ctx.Set("instance", instance) })
	validation := auth_middleware.NewJIDValidationMiddleware()
	h := &userHandler{userService: service}
	r.POST("/user/info", validation.ValidateNumberField(), h.GetUser)
	r.POST("/user/avatar", validation.ValidateNumberField(), h.GetAvatar)
	return r
}

func userQueryRequest(t *testing.T, r *gin.Engine, path, body string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestUserQueriesRejectInvalidBodies(t *testing.T) {
	service := queryServiceFake{
		info: func(context.Context, *user_service.CheckUserStruct, *instance_model.Instance) (*user_service.UserCollection, error) {
			t.Fatal("service called for invalid info body")
			return nil, nil
		},
		avatar: func(context.Context, *user_service.GetAvatarStruct, *instance_model.Instance) (*types.ProfilePictureInfo, error) {
			t.Fatal("service called for invalid avatar body")
			return nil, nil
		},
	}
	r := userQueryRouter(service, &instance_model.Instance{Id: "authenticated"})
	for _, path := range []string{"/user/info", "/user/avatar"} {
		for _, body := range []string{"null", "{}", "{", "[]", "", `{"number":null}`, `{"number":42}`} {
			t.Run(path+body, func(t *testing.T) {
				w := userQueryRequest(t, r, path, body, context.Background())
				if w.Code != http.StatusBadRequest {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestUserQueriesUseAuthenticatedInstanceAndRequestContext(t *testing.T) {
	instance := &instance_model.Instance{Id: "authenticated"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := queryServiceFake{
		info: func(gotCtx context.Context, data *user_service.CheckUserStruct, gotInstance *instance_model.Instance) (*user_service.UserCollection, error) {
			if gotInstance != instance || !errors.Is(gotCtx.Err(), context.Canceled) || len(data.Number) != 1 {
				t.Fatal("info lost authenticated instance or request context")
			}
			return nil, gotCtx.Err()
		},
		avatar: func(gotCtx context.Context, data *user_service.GetAvatarStruct, gotInstance *instance_model.Instance) (*types.ProfilePictureInfo, error) {
			if gotInstance != instance || !errors.Is(gotCtx.Err(), context.Canceled) || !data.Preview {
				t.Fatal("avatar lost authenticated instance or request context")
			}
			return nil, gotCtx.Err()
		},
	}
	r := userQueryRouter(service, instance)
	for _, tc := range []struct{ path, body string }{
		{"/user/info", `{"number":["14155550100"],"instanceId":"other"}`},
		{"/user/avatar", `{"number":"14155550100","preview":true,"instanceId":"other"}`},
	} {
		w := userQueryRequest(t, r, tc.path, tc.body, ctx)
		if w.Code != http.StatusGatewayTimeout {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}
}

func TestUserQueriesErrorResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"invalid number", user_service.ErrInvalidUserNumber, http.StatusBadRequest},
		{"rate limit", fmt.Errorf("private storage detail: %w", whatsmeow.ErrIQRateOverLimit), http.StatusTooManyRequests},
		{"timeout", fmt.Errorf("private storage detail: %w", context.DeadlineExceeded), http.StatusGatewayTimeout},
		{"internal", errors.New("postgres://username:private-secret@database"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := queryServiceFake{info: func(context.Context, *user_service.CheckUserStruct, *instance_model.Instance) (*user_service.UserCollection, error) {
				return nil, tc.err
			}, avatar: func(context.Context, *user_service.GetAvatarStruct, *instance_model.Instance) (*types.ProfilePictureInfo, error) {
				return nil, tc.err
			}}
			r := userQueryRouter(service, &instance_model.Instance{Id: "authenticated"})
			for _, req := range []struct{ path, body string }{{"/user/info", `{"number":["14155550100"]}`}, {"/user/avatar", `{"number":"14155550100"}`}} {
				w := userQueryRequest(t, r, req.path, req.body, context.Background())
				if w.Code != tc.status || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "postgres") {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestUserInfoPictureURLResponse(t *testing.T) {
	jid := types.NewJID("14155550100", types.DefaultUserServer)
	service := queryServiceFake{info: func(context.Context, *user_service.CheckUserStruct, *instance_model.Instance) (*user_service.UserCollection, error) {
		return &user_service.UserCollection{Users: map[types.JID]user_service.UserInfo{jid: {Status: "hello", PictureID: "photo", PictureURL: "https://example.test/photo"}}}, nil
	}}
	r := userQueryRouter(service, &instance_model.Instance{Id: "authenticated"})
	w := userQueryRequest(t, r, "/user/info", `{"number":["14155550100"]}`, context.Background())
	var body struct {
		Message string
		Data    struct {
			Users map[string]user_service.UserInfo
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	info := body.Data.Users[jid.String()]
	if w.Code != http.StatusOK || body.Message != "success" || info.PictureURL != "https://example.test/photo" || info.PictureID != "photo" || info.Status != "hello" {
		t.Fatalf("response=%s", w.Body.String())
	}
}

func TestUserQueriesMissingInstance(t *testing.T) {
	r := userQueryRouter(queryServiceFake{}, nil)
	for _, req := range []struct{ path, body string }{{"/user/info", `{"number":["14155550100"]}`}, {"/user/avatar", `{"number":"14155550100"}`}} {
		w := userQueryRequest(t, r, req.path, req.body, context.Background())
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", w.Code)
		}
	}
}
