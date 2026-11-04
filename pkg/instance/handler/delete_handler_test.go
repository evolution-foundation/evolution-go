package instance_handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
)

type deleteService struct {
	instance_service.InstanceService
	err error
}

func (s *deleteService) Delete(string) error { return s.err }

func TestDeleteStatusByError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", instance_service.ErrInstanceNotFound, http.StatusNotFound},
		{"other", errors.New("boom"), http.StatusInternalServerError},
		{"ok", nil, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &instanceHandler{instanceService: &deleteService{err: tc.err}}
			router := gin.New()
			router.DELETE("/instance/delete/:instanceId", h.Delete)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/instance/delete/abc", nil))
			if rec.Code != tc.want {
				t.Fatalf("status=%d, want %d", rec.Code, tc.want)
			}
		})
	}
}
