package handlers

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestParseProfileTargetPreservesLID(t *testing.T) {
	jid, err := parseProfileTarget("123456789012345:4@lid")
	if err != nil {
		t.Fatal(err)
	}
	if jid.String() != "123456789012345@lid" {
		t.Fatalf("profile target = %s", jid)
	}
}

func TestNormalizeCallTargetSupportsPNAndLID(t *testing.T) {
	tests := map[string]string{
		"+628123456789":         "628123456789@s.whatsapp.net",
		"123456789012345:2@lid": "123456789012345@lid",
	}
	for input, want := range tests {
		got, err := normalizeCallTarget(input)
		if err != nil {
			t.Fatalf("normalizeCallTarget(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeCallTarget(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := normalizeCallTarget(types.NewJID("123", types.GroupServer).String()); err == nil {
		t.Fatal("group JID must not be accepted as a call target")
	}
}

func TestParseLIDTargetRejectsUnsupportedNamespace(t *testing.T) {
	if _, _, err := parseLIDTarget(types.NewJID("123", types.GroupServer).String()); err == nil {
		t.Fatal("group JID must not be converted to a PN")
	}
}
