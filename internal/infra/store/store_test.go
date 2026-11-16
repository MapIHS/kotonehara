package store

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func initTestDB(t *testing.T) {
	t.Helper()
	conn, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	previous := db
	InitDB(conn)
	t.Cleanup(func() { db = previous; _ = conn.Close() })
}

func TestNSFWToggle(t *testing.T) {
	initTestDB(t)
	const group = "120363000000000000@g.us"
	if IsNSFWEnabled(group) {
		t.Fatal("NSFW must default to disabled")
	}
	for _, want := range []bool{true, false, true} {
		got, err := ToggleNSFW(group)
		if err != nil {
			t.Fatal(err)
		}
		if got != want || IsNSFWEnabled(group) != want {
			t.Fatalf("toggle: got %t, stored %t, want %t", got, IsNSFWEnabled(group), want)
		}
	}
	if IsNSFWEnabled("other@g.us") {
		t.Fatal("setting leaked to another group")
	}
}

func TestAFKLifecycleAcrossAliases(t *testing.T) {
	initTestDB(t)
	ctx := context.Background()
	const pn, lid = "628123456789@s.whatsapp.net", "123456789012345@lid"

	if _, ok, err := GetAFK(ctx, pn, lid); ok || err != nil {
		t.Fatalf("unexpected AFK before set: ok=%t err=%v", ok, err)
	}
	// A stale row under the alias must be replaced by the canonical row.
	if err := SetAFK(ctx, lid, "lama"); err != nil {
		t.Fatal(err)
	}
	if err := SetAFK(ctx, pn, "makan", lid); err != nil {
		t.Fatal(err)
	}
	// Callers pass every known alias; the state is found whichever comes first.
	state, ok, err := GetAFK(ctx, lid, pn)
	if err != nil || !ok || state.Reason != "makan" || state.JID != pn {
		t.Fatalf("lookup by aliases: %+v ok=%t err=%v", state, ok, err)
	}
	if _, ok, _ := GetAFK(ctx, lid); ok {
		t.Fatal("stale alias row survived SetAFK")
	}
	if state.Time.IsZero() {
		t.Fatal("AFK time not stored")
	}
	cleared, ok, err := ClearAFK(ctx, lid, pn)
	if err != nil || !ok || cleared.Reason != "makan" {
		t.Fatalf("clear: %+v ok=%t err=%v", cleared, ok, err)
	}
	if _, ok, _ := GetAFK(ctx, pn, lid); ok {
		t.Fatal("AFK still present after clear")
	}
}

func TestNilDBIsSafe(t *testing.T) {
	previous := db
	db = nil
	t.Cleanup(func() { db = previous })
	if IsNSFWEnabled("x@g.us") {
		t.Fatal("nil DB reported enabled")
	}
	if _, err := ToggleNSFW("x@g.us"); err != nil {
		t.Fatal(err)
	}
}
