package instance_service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	"github.com/evolution-foundation/evolution-go/pkg/syncmap"
	"go.mau.fi/whatsmeow"
)

type fakeInstanceRepository struct {
	instance_repository.InstanceRepository
	instance instance_model.Instance
}

func (f *fakeInstanceRepository) GetInstanceByID(string) (*instance_model.Instance, error) {
	copied := f.instance
	return &copied, nil
}

func infoSubject(rawProxy, globalHost string) instances {
	return instances{
		instanceRepository: &fakeInstanceRepository{instance: instance_model.Instance{Id: "inst-1", Proxy: rawProxy}},
		config:             &config.Config{ProxyHost: globalHost},
		clientPointer:      syncmap.New[string, *whatsmeow.Client](),
	}
}

func TestInfoReportsHasProxyWithoutExposingCredentials(t *testing.T) {
	cases := []struct {
		name       string
		rawProxy   string
		globalHost string
		want       bool
	}{
		{"no proxy", "", "", false},
		{"empty proxy object", `{"host":"","port":"","username":"","password":""}`, "", false},
		{"instance proxy", `{"host":"10.0.0.1","port":"3128","username":"u","password":"secret-pass"}`, "", true},
		{"global proxy", "", "proxy.local", true},
		{"invalid json without global proxy", "not-json", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := infoSubject(tc.rawProxy, tc.globalHost).Info("inst-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.HasProxy != tc.want {
				t.Fatalf("HasProxy = %v, want %v", got.HasProxy, tc.want)
			}
			if got.Proxy != "" {
				t.Fatalf("Proxy must be blanked, got %q", got.Proxy)
			}
			body, _ := json.Marshal(got)
			if strings.Contains(string(body), "secret-pass") {
				t.Fatalf("response leaks proxy credentials: %s", body)
			}
			if !strings.Contains(string(body), `"hasProxy":`) {
				t.Fatalf("response lacks hasProxy: %s", body)
			}
		})
	}
}
