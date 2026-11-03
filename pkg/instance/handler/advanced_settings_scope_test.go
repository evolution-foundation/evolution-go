package instance_handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

func TestAuthorizedPathInstance(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		pathID   string
		authID   string
		wantOK   bool
		wantCode int
	}{
		{"mesma instância", "a", "a", true, http.StatusOK},
		{"outra instância", "b", "a", false, http.StatusForbidden},
		{"sem id", "", "a", false, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "instanceId", Value: tc.pathID}}
			c.Set("instance", &instance_model.Instance{Id: tc.authID})

			id, ok := authorizedPathInstance(c)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && id != tc.pathID {
				t.Fatalf("id = %q, want %q", id, tc.pathID)
			}
			if !ok && w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantCode)
			}
		})
	}
}
