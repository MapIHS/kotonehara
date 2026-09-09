package clients

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestGroupAdminAliasesIncludesPNLIDAndNormalizesDevice(t *testing.T) {
	pn := types.JID{User: "628123", Device: 2, Server: types.DefaultUserServer}
	lid := types.NewJID("123456789012345", types.HiddenUserServer)
	aliases := groupAdminAliases([]types.GroupParticipant{{
		JID:         lid,
		PhoneNumber: pn,
		LID:         lid,
		IsAdmin:     true,
	}})

	want := map[string]bool{
		"628123@s.whatsapp.net": true,
		"123456789012345@lid":   true,
	}
	if len(aliases) != len(want) {
		t.Fatalf("aliases = %v", aliases)
	}
	for _, alias := range aliases {
		if !want[alias] {
			t.Fatalf("unexpected admin alias %q", alias)
		}
	}
}

func TestGroupAdminAliasesExcludesRegularParticipants(t *testing.T) {
	aliases := groupAdminAliases([]types.GroupParticipant{{
		JID: types.NewJID("628123", types.DefaultUserServer),
	}})
	if len(aliases) != 0 {
		t.Fatalf("regular participant aliases = %v", aliases)
	}
}
