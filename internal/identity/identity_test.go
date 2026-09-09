package identity

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestIdentityNormalizesDevicesAndKeepsNamespacesSeparate(t *testing.T) {
	pnDevice := types.JID{User: "628123", Device: 7, Server: types.DefaultUserServer}
	lidSameDigits := types.NewJID("628123", types.HiddenUserServer)
	id := New(pnDevice, types.EmptyJID)

	if got := id.StateJID(); got != "628123@s.whatsapp.net" {
		t.Fatalf("StateJID() = %q", got)
	}
	if id.Matches(lidSameDigits) {
		t.Fatal("PN and LID with equal user components must not match")
	}
	if !id.Matches(types.NewJID("628123", types.DefaultUserServer)) {
		t.Fatal("device-qualified PN must match its non-device PN")
	}
}

func TestIdentityUsesVerifiedAlternate(t *testing.T) {
	lid := types.NewJID("123456789012345", types.HiddenUserServer)
	pn := types.NewJID("628123456789", types.DefaultUserServer)
	id := New(lid, pn)

	if id.Key != "pn:628123456789" {
		t.Fatalf("Key = %q", id.Key)
	}
	if !id.Matches(lid) || !id.Matches(pn) {
		t.Fatal("identity must match both verified aliases")
	}
}

func TestParseUserTreatsPlainNumberAsPN(t *testing.T) {
	jid, err := ParseUser("+628123456789")
	if err != nil {
		t.Fatal(err)
	}
	if jid.Server != types.DefaultUserServer || jid.User != "628123456789" {
		t.Fatalf("unexpected JID: %s", jid)
	}
	if _, err := ParseUser("owner-name"); err == nil {
		t.Fatal("non-numeric unqualified owner must be rejected")
	}
}
