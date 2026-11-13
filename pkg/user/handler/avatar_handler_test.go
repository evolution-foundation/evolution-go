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

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
	auth_middleware "github.com/evolution-foundation/evolution-go/pkg/middleware"
	user_service "github.com/evolution-foundation/evolution-go/pkg/user/service"
	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type avatarUserServiceFake struct {
	user_service.UserService
	query func(context.Context, *user_service.GetAvatarStruct, *instance_model.Instance) (*types.ProfilePictureInfo, error)
}

func (f avatarUserServiceFake) GetAvatar(ctx context.Context, data *user_service.GetAvatarStruct, instance *instance_model.Instance) (*types.ProfilePictureInfo, error) {
	return f.query(ctx, data, instance)
}

type avatarInstancesFake struct {
	instance_service.InstanceService
}

func (avatarInstancesFake) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	if token == "token-one" {
		return &instance_model.Instance{Id: "one"}, nil
	}
	if token == "token-two" {
		return &instance_model.Instance{Id: "two"}, nil
	}
	if token == "nil-instance" {
		return nil, nil
	}
	return nil, errors.New("unknown token")
}

func avatarRouter(service user_service.UserService) *gin.Engine {
	r := gin.New()
	middleware := auth_middleware.NewMiddleware(&config.Config{}, avatarInstancesFake{})
	validation := auth_middleware.NewJIDValidationMiddleware()
	h := &userHandler{userService: service}
	r.POST("/user/avatar", middleware.Auth, validation.ValidateNumberField(), h.GetAvatar)
	return r
}

func avatarHTTPRequest(r *gin.Engine, token, body string, ctx context.Context) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/user/avatar", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("apikey", token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAvatarHTTPRejectsInvalidBodiesAndMissingAuth(t *testing.T) {
	service := avatarUserServiceFake{query: func(context.Context, *user_service.GetAvatarStruct, *instance_model.Instance) (*types.ProfilePictureInfo, error) {
		t.Fatal("service called for rejected request")
		return nil, nil
	}}
	r := avatarRouter(service)
	for _, body := range []string{"null", "{}", "{", "", "[]", `{"number":null}`, `{"number":42}`, `{"number":[]}`, `{"number":"14155550100","preview":"yes"}`} {
		t.Run(body, func(t *testing.T) {
			w := avatarHTTPRequest(r, "token-one", body, context.Background())
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
	for _, token := range []string{"", "invalid-token"} {
		w := avatarHTTPRequest(r, token, `{"number":"14155550100"}`, context.Background())
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("auth status=%d", w.Code)
		}
	}
	w := avatarHTTPRequest(r, "nil-instance", `{"number":"14155550100"}`, context.Background())
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("nil instance status=%d", w.Code)
	}
}

func TestAvatarHTTPSessionScopeAndSuccessContract(t *testing.T) {
	for _, tc := range []struct {
		token, id string
		preview   bool
	}{{"token-one", "one", false}, {"token-two", "two", true}} {
		t.Run(tc.id, func(t *testing.T) {
			calls := 0
			var hash []byte
			if tc.preview {
				hash = []byte{1, 2, 3}
			}
			picture := &types.ProfilePictureInfo{URL: "https://example.test/photo", ID: "photo", Type: "image", DirectPath: "/photo", Hash: hash}
			service := avatarUserServiceFake{query: func(ctx context.Context, data *user_service.GetAvatarStruct, instance *instance_model.Instance) (*types.ProfilePictureInfo, error) {
				calls++
				if instance.Id != tc.id || data.Preview != tc.preview || data.Number == "" {
					t.Fatal("request switched session or lost preview")
				}
				return picture, nil
			}}
			body := fmt.Sprintf(`{"number":"14155550100","preview":%t,"instanceId":"other"}`, tc.preview)
			w := avatarHTTPRequest(avatarRouter(service), tc.token, body, context.Background())
			want, err := json.Marshal(gin.H{"message": "success", "data": picture})
			if err != nil {
				t.Fatal(err)
			}
			var gotJSON, wantJSON interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &gotJSON); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(want, &wantJSON); err != nil {
				t.Fatal(err)
			}
			gotNormalized, _ := json.Marshal(gotJSON)
			wantNormalized, _ := json.Marshal(wantJSON)
			if w.Code != http.StatusOK || calls != 1 || string(gotNormalized) != string(wantNormalized) {
				t.Fatalf("status=%d got=%s want=%s calls=%d", w.Code, gotNormalized, wantNormalized, calls)
			}
		})
	}
}

func TestAvatarHTTPContextAndErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"invalid number", user_service.ErrInvalidUserNumber, http.StatusBadRequest},
		{"rate limit", fmt.Errorf("private details: %w", whatsmeow.ErrIQRateOverLimit), http.StatusTooManyRequests},
		{"IQ timeout", whatsmeow.ErrIQTimedOut, http.StatusGatewayTimeout},
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"cancellation", context.Canceled, http.StatusGatewayTimeout},
		{"private photo", whatsmeow.ErrProfilePictureUnauthorized, http.StatusInternalServerError},
		{"missing photo", whatsmeow.ErrProfilePictureNotSet, http.StatusInternalServerError},
		{"internal error", errors.New("postgres://user:private-password@database"), http.StatusInternalServerError},
		{"nil result", nil, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			service := avatarUserServiceFake{query: func(gotCtx context.Context, _ *user_service.GetAvatarStruct, instance *instance_model.Instance) (*types.ProfilePictureInfo, error) {
				if !errors.Is(gotCtx.Err(), context.Canceled) || instance.Id != "one" {
					t.Fatal("lost request context or instance")
				}
				return nil, tc.err
			}}
			w := avatarHTTPRequest(avatarRouter(service), "token-one", `{"number":"14155550100"}`, ctx)
			if w.Code != tc.status || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "postgres") {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
