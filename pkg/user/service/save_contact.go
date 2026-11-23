package user_service

// Save a contact to the device addressbook.
//
// WhatsApp syncs the contact list across devices via APP STATE (the same
// channel as mute/pin/archive): a "contact"-index mutation on the
// critical_unblock_low patch — which whatsmeow itself applies back into
// Store.Contacts on receipt (appstate.go, IndexContact → PutContactName).
// The API currently exposes only the READ side (GET /user/contacts); this
// adds the WRITE side: build the ContactAction (FullName/FirstName +
// SaveOnPrimaryAddressbook, which asks the primary phone to also store it in
// the SYSTEM addressbook) and send it with the official primitive
// Client.SendAppState. After the resync SendAppState triggers, the contact
// shows up in GET /user/contacts.
//
// Own file + minimal call-site lines (interface/handler/route) to keep the
// change easy to review and rebase.

import (
	"context"
	"errors"
	"strings"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"go.mau.fi/whatsmeow/appstate"
	waSyncAction "go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type SaveContactStruct struct {
	Number    string `json:"phone"`
	FullName  string `json:"fullName"`
	FirstName string `json:"firstName"`
}

// contactMutationVersion is the static version of the "contact" index
// mutation (each index has its own — mute=2, pin=5, …; reference
// implementations use 2 for contact). A wrong value fails CLEANLY in
// SendAppState (the server rejects the patch), with no side effects.
const contactMutationVersion = 2

func (u *userService) SaveContact(data *SaveContactStruct, instance *instance_model.Instance) error {
	client, err := u.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}
	number := strings.TrimSpace(data.Number)
	fullName := strings.TrimSpace(data.FullName)
	if number == "" || fullName == "" {
		return errors.New("phone and fullName are required")
	}
	firstName := strings.TrimSpace(data.FirstName)
	if firstName == "" {
		firstName = strings.Fields(fullName)[0]
	}
	jid := types.NewJID(number, types.DefaultUserServer)

	patch := appstate.PatchInfo{
		Type: appstate.WAPatchCriticalUnblockLow, // the patch that carries the contact list
		Mutations: []appstate.MutationInfo{{
			Index:   []string{appstate.IndexContact, jid.String()},
			Version: contactMutationVersion,
			Value: &waSyncAction.SyncActionValue{
				ContactAction: &waSyncAction.ContactAction{
					FullName:                 proto.String(fullName),
					FirstName:                proto.String(firstName),
					SaveOnPrimaryAddressbook: proto.Bool(true),
				},
			},
		}},
	}
	if err := client.SendAppState(context.Background(), patch); err != nil {
		u.loggerWrapper.GetLogger(instance.Id).LogError("[%s] SaveContact %s: %v", instance.Id, jid.String(), err)
		return err
	}
	u.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Contact saved to addressbook: %s (%s)", instance.Id, fullName, jid.String())
	return nil
}
