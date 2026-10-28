package identity

import (
	"reflect"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestIdentityUnifiesDevicePNAndLID(t *testing.T) {
	pn := types.NewJID("628123", types.DefaultUserServer)
	lid := types.NewJID("777", types.HiddenUserServer)
	device := pn
	device.Device = 5
	id := New(lid, device)
	if id.Key != "pn:628123" || id.StateJID() != pn.String() {
		t.Fatalf("incorrect canonical identity: %+v", id)
	}
	id.Add(pn)
	if got := id.AliasStrings(); !reflect.DeepEqual(got, []string{lid.String(), pn.String()}) {
		t.Fatalf("aliases = %v", got)
	}
	for _, jid := range []types.JID{device, pn, lid} {
		if !id.Matches(jid) {
			t.Errorf("missing alias %s", jid)
		}
	}
	if !id.MatchesString("+628123") || id.Matches(types.EmptyJID) || id.MatchesString("other@g.us") {
		t.Fatal("incorrect alias matching")
	}
}

func TestIdentityLIDAcquiresPhoneWithoutLosingAlias(t *testing.T) {
	lid := types.NewJID("777", types.HiddenUserServer)
	id := New(lid, types.EmptyJID)
	if id.Key != "lid:777" || id.StateJID() != lid.String() {
		t.Fatal(id)
	}
	id.Add(types.NewJID("628123", types.DefaultUserServer))
	if id.Key != "pn:628123" || !id.Matches(lid) {
		t.Fatal(id)
	}
	empty := New(types.EmptyJID, types.EmptyJID)
	if empty.Key != "" || len(empty.Aliases()) != 0 {
		t.Fatal(empty)
	}
}

func TestParseUser(t *testing.T) {
	for _, tc := range []struct {
		input, want string
		invalid     bool
	}{
		{" +628123 ", "628123@s.whatsapp.net", false},
		{"628123:5@s.whatsapp.net", "628123@s.whatsapp.net", false},
		{"777@lid", "777@lid", false},
		{"", "", true}, {"+", "", true}, {"62abc", "", true}, {"123@g.us", "", true}, {"123@broadcast", "", true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseUser(tc.input)
			if tc.invalid {
				if err == nil {
					t.Fatal("expected invalid user")
				}
				return
			}
			if err != nil || got.String() != tc.want {
				t.Fatalf("got %s, %v; want %s", got, err, tc.want)
			}
		})
	}
}
