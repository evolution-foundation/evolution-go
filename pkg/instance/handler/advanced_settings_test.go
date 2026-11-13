package instance_handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
	auth_middleware "github.com/evolution-foundation/evolution-go/pkg/middleware"
	"github.com/gin-gonic/gin"
)

const settingsInstanceID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

type settingsServiceStub struct {
	instance_service.InstanceService
	getErr, updateErr, connectErr       error
	getCalls, updateCalls, connectCalls int
	updated                             *instance_model.AdvancedSettings
}

func (s *settingsServiceStub) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	if token != "instance-token" {
		return nil, errors.New("invalid token")
	}
	return &instance_model.Instance{Id: settingsInstanceID}, nil
}

func (s *settingsServiceStub) GetAdvancedSettings(id string) (*instance_model.AdvancedSettings, error) {
	s.getCalls++
	if id != settingsInstanceID {
		return nil, errors.New("unexpected instance")
	}
	message := "busy"
	return &instance_model.AdvancedSettings{
		AlwaysOnline: instance_model.BoolPtr(true), RejectCall: instance_model.BoolPtr(true),
		ReadMessages: instance_model.BoolPtr(false), IgnoreGroups: instance_model.BoolPtr(true),
		IgnoreStatus: instance_model.BoolPtr(false), MsgRejectCall: &message,
	}, s.getErr
}

func (s *settingsServiceStub) UpdateAdvancedSettings(id string, settings *instance_model.AdvancedSettings) error {
	s.updateCalls++
	if id != settingsInstanceID {
		return errors.New("unexpected instance")
	}
	s.updated = settings
	return s.updateErr
}

func (s *settingsServiceStub) Connect(*instance_service.ConnectStruct, *instance_model.Instance) (*instance_model.Instance, string, string, error) {
	s.connectCalls++
	return &instance_model.Instance{Id: settingsInstanceID}, "", "MESSAGE", s.connectErr
}

func settingsTestRouter(svc *settingsServiceStub) *gin.Engine {
	router := gin.New()
	router.Use(auth_middleware.NewMiddleware(nil, svc).Auth)
	handler := NewInstanceHandler(svc, nil)
	router.GET("/instance/:instanceId/advanced-settings", handler.GetAdvancedSettings)
	router.PUT("/instance/:instanceId/advanced-settings", handler.UpdateAdvancedSettings)
	router.POST("/instance/connect", handler.Connect)
	return router
}

func TestAdvancedSettingsAuthorization(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		for _, tt := range []struct {
			name, token, id string
			status          int
		}{
			{"missing_token", "", settingsInstanceID, http.StatusUnauthorized},
			{"invalid_token", "other-token", settingsInstanceID, http.StatusUnauthorized},
			{"other_instance", "instance-token", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", http.StatusForbidden},
			{"invalid_id", "instance-token", "not-a-uuid", http.StatusBadRequest},
		} {
			t.Run(method+"/"+tt.name, func(t *testing.T) {
				svc := &settingsServiceStub{}
				req := httptest.NewRequest(method, "/instance/"+tt.id+"/advanced-settings", strings.NewReader("{}"))
				req.Header.Set("apikey", tt.token)
				req.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				settingsTestRouter(svc).ServeHTTP(response, req)
				if response.Code != tt.status || svc.getCalls != 0 || svc.updateCalls != 0 {
					t.Fatalf("status=%d get=%d update=%d, want %d and no service access", response.Code, svc.getCalls, svc.updateCalls, tt.status)
				}
			})
		}
	}
}

func TestAdvancedSettingsUpdateResponses(t *testing.T) {
	for _, tt := range []struct {
		name, body            string
		getErr, updateErr     error
		status, gets, updates int
	}{
		{"partial_success", `{"alwaysOnline":false,"msgRejectCall":""}`, nil, nil, 200, 1, 1},
		{"empty_object", "{}", nil, nil, 200, 1, 1},
		{"null_fields", `{"alwaysOnline":null,"msgRejectCall":null}`, nil, nil, 200, 1, 1},
		{"null_body", "null", nil, nil, 400, 0, 0},
		{"array_body", "[]", nil, nil, 400, 0, 0},
		{"invalid_type", `{"alwaysOnline":"false"}`, nil, nil, 400, 0, 0},
		{"invalid_json", "{", nil, nil, 400, 0, 0},
		{"read_failure", "{}", errors.New("private database details"), nil, 500, 1, 1},
		{"write_failure", "{}", nil, errors.New("private database details"), 500, 0, 1},
		{"instance_deleted", "{}", nil, instance_model.ErrInstanceNotFound, 404, 0, 1},
		{"deleted_before_read", "{}", instance_model.ErrInstanceNotFound, nil, 404, 1, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &settingsServiceStub{getErr: tt.getErr, updateErr: tt.updateErr}
			req := httptest.NewRequest(http.MethodPut, "/instance/"+strings.ToUpper(settingsInstanceID)+"/advanced-settings", strings.NewReader(tt.body))
			req.Header.Set("apikey", "instance-token")
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			settingsTestRouter(svc).ServeHTTP(response, req)
			if response.Code != tt.status || svc.getCalls != tt.gets || svc.updateCalls != tt.updates {
				t.Fatalf("status=%d get=%d update=%d body=%s", response.Code, svc.getCalls, svc.updateCalls, response.Body)
			}
			if strings.Contains(response.Body.String(), "private database") {
				t.Fatal("exposed internal error")
			}
			if tt.status == 200 {
				var body struct {
					Settings instance_model.AdvancedSettings `json:"settings"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Settings.RejectCall == nil || !*body.Settings.RejectCall || body.Settings.MsgRejectCall == nil || *body.Settings.MsgRejectCall != "busy" {
					t.Fatalf("response did not contain persisted settings: %s", response.Body)
				}
			}
			if tt.name == "partial_success" && (svc.updated.AlwaysOnline == nil || *svc.updated.AlwaysOnline || svc.updated.MsgRejectCall == nil || *svc.updated.MsgRejectCall != "" || svc.updated.RejectCall != nil) {
				t.Fatalf("lost request field presence: %#v", svc.updated)
			}
		})
	}
}

func TestConnectInvalidRequests(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		err        error
		calls      int
	}{
		{"null", "null", nil, 0},
		{"invalid_events", `{"subscribe":["invalid"]}`, instance_service.ErrInvalidConnectSettings, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &settingsServiceStub{connectErr: tt.err}
			req := httptest.NewRequest(http.MethodPost, "/instance/connect", strings.NewReader(tt.body))
			req.Header.Set("apikey", "instance-token")
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			settingsTestRouter(svc).ServeHTTP(response, req)
			if response.Code != 400 || svc.connectCalls != tt.calls {
				t.Fatalf("status=%d connectCalls=%d", response.Code, svc.connectCalls)
			}
		})
	}
}

func TestGetAdvancedSettingsResponses(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{
		{"success", nil, http.StatusOK},
		{"deleted", instance_model.ErrInstanceNotFound, http.StatusNotFound},
		{"read_failure", errors.New("private database details"), http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &settingsServiceStub{getErr: tt.err}
			req := httptest.NewRequest(http.MethodGet, "/instance/"+settingsInstanceID+"/advanced-settings", nil)
			req.Header.Set("apikey", "instance-token")
			response := httptest.NewRecorder()
			settingsTestRouter(svc).ServeHTTP(response, req)
			if response.Code != tt.status || svc.getCalls != 1 || svc.updateCalls != 0 || strings.Contains(response.Body.String(), "private database") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
		})
	}
}
