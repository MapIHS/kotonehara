package commands

import (
	"testing"
	"time"
)

func TestCooldownAliasesShareWindow(t *testing.T) {
	SetCooldown(time.Minute)
	t.Cleanup(func() { SetCooldown(3 * time.Second) })

	aliases := []string{"628123@s.whatsapp.net|test", "123456789012345@lid|test"}
	if !allowCooldownAliases(aliases) {
		t.Fatal("first command should be allowed")
	}
	if allowCooldownAliases([]string{aliases[1]}) {
		t.Fatal("LID alias should share the PN cooldown window")
	}
}
