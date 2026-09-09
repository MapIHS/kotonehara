package store

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestMigrateAFKIdentitiesKeepsLatestState(t *testing.T) {
	database, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	db = database
	createTable()
	if _, err := db.Exec(`CREATE TABLE whatsmeow_lid_map (lid TEXT PRIMARY KEY, pn TEXT UNIQUE NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO whatsmeow_lid_map (lid, pn) VALUES ('123456789012345', '628123456789')`); err != nil {
		t.Fatal(err)
	}
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	if _, err := db.Exec(`INSERT INTO afk_users (jid, reason, created_at) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)`,
		"628123456789@s.whatsapp.net", "old", older,
		"123456789012345@lid", "new", newer,
		"628999:4@s.whatsapp.net", "device", newer,
	); err != nil {
		t.Fatal(err)
	}

	if err := MigrateAFKIdentities(context.Background()); err != nil {
		t.Fatal(err)
	}

	state, ok, err := GetAFK(context.Background(), "628123456789@s.whatsapp.net")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || state.Reason != "new" {
		t.Fatalf("canonical AFK = %#v, found=%t", state, ok)
	}
	if state.Time.Before(newer.Add(-time.Second)) || state.Time.After(newer.Add(time.Second)) {
		t.Fatalf("AFK timestamp changed during migration: got %v, want around %v", state.Time, newer)
	}
	var rows int
	if err := db.Get(&rows, `SELECT COUNT(*) FROM afk_users WHERE jid = '123456789012345@lid'`); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("LID AFK rows = %d, want 0", rows)
	}
	state, ok, err = GetAFK(context.Background(), "628999@s.whatsapp.net")
	if err != nil || !ok || state.Reason != "device" {
		t.Fatalf("normalized device AFK = %#v, found=%t, err=%v", state, ok, err)
	}
}
