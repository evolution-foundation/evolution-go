package instance_service

import (
	"errors"
	"testing"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	"github.com/evolution-foundation/evolution-go/pkg/syncmap"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
	"gorm.io/gorm"
)

type deleteRepo struct {
	instance_repository.InstanceRepository
	instance    instance_model.Instance
	getErr      error
	deleteCalls int
}

func (r *deleteRepo) GetInstanceByID(string) (*instance_model.Instance, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	copied := r.instance
	return &copied, nil
}

func (r *deleteRepo) Delete(string) error {
	r.deleteCalls++
	return nil
}

type deleteWhatsmeow struct {
	whatsmeow_service.WhatsmeowService
	storeJids []string
	storeErr  error
}

func (w *deleteWhatsmeow) ClearInstanceCache(string, string) error { return nil }

func (w *deleteWhatsmeow) DeleteDeviceStore(jid string) error {
	w.storeJids = append(w.storeJids, jid)
	return w.storeErr
}

func newDeleteSubject(repo *deleteRepo, wm *deleteWhatsmeow) instances {
	return instances{
		instanceRepository: repo,
		whatsmeowService:   wm,
		clientPointer:      syncmap.New[string, *whatsmeow.Client](),
		killChannel:        syncmap.New[string, chan bool](),
	}
}

func TestDeleteWithoutClientRemovesDeviceStore(t *testing.T) {
	jid := "5511999999999@s.whatsapp.net"
	repo := &deleteRepo{instance: instance_model.Instance{Id: "inst-1", Jid: jid}}
	wm := &deleteWhatsmeow{}

	if err := newDeleteSubject(repo, wm).Delete("inst-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wm.storeJids) != 1 || wm.storeJids[0] != jid || repo.deleteCalls != 1 {
		t.Fatalf("store=%v repoDelete=%d, want [%s] and 1", wm.storeJids, repo.deleteCalls, jid)
	}
}

func TestDeleteWithoutJidSkipsDeviceStore(t *testing.T) {
	repo := &deleteRepo{instance: instance_model.Instance{Id: "inst-1"}}
	wm := &deleteWhatsmeow{}

	if err := newDeleteSubject(repo, wm).Delete("inst-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wm.storeJids) != 0 || repo.deleteCalls != 1 {
		t.Fatalf("store=%v repoDelete=%d, want none and 1", wm.storeJids, repo.deleteCalls)
	}
}

func TestDeleteDeviceStoreFailureAborts(t *testing.T) {
	repo := &deleteRepo{instance: instance_model.Instance{Id: "inst-1", Jid: "5511999999999@s.whatsapp.net"}}
	wm := &deleteWhatsmeow{storeErr: errors.New("db down")}

	if err := newDeleteSubject(repo, wm).Delete("inst-1"); err == nil {
		t.Fatal("expected error")
	}
	if repo.deleteCalls != 0 {
		t.Fatalf("repoDelete=%d, want 0", repo.deleteCalls)
	}
}

func TestDeleteMissingInstanceReturnsNotFound(t *testing.T) {
	for name, getErr := range map[string]error{
		"record not found": gorm.ErrRecordNotFound,
		"invalid uuid":     errors.New("invalid UUID format: bad"),
	} {
		t.Run(name, func(t *testing.T) {
			repo := &deleteRepo{getErr: getErr}
			err := newDeleteSubject(repo, &deleteWhatsmeow{}).Delete("x")
			if !errors.Is(err, ErrInstanceNotFound) {
				t.Fatalf("err=%v, want ErrInstanceNotFound", err)
			}
		})
	}
}
