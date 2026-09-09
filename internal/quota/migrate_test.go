package quota

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestMigrateIdentityAliasesMergesUsageAndPremium(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE whatsmeow_lid_map (lid TEXT PRIMARY KEY, pn TEXT UNIQUE NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO whatsmeow_lid_map (lid, pn) VALUES ('123456789012345', '628123456789')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO user_daily_usage (jid, usage_count, reset_date) VALUES
		('628123456789@s.whatsapp.net', 2, '2026-09-09'),
		('123456789012345@lid', 3, '2026-09-09')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO premium_users (jid, added_by, expires_at) VALUES
		('628123456789@s.whatsapp.net', 'owner@s.whatsapp.net', DATETIME('now', '+1 day')),
		('123456789012345@lid', 'owner@lid', NULL)`); err != nil {
		t.Fatal(err)
	}

	if err := MigrateIdentityAliases(ctx, db); err != nil {
		t.Fatal(err)
	}

	var count, rows int
	if err := db.Get(&count, `SELECT usage_count FROM user_daily_usage WHERE jid = '628123456789@s.whatsapp.net'`); err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("merged usage = %d, want 5", count)
	}
	if err := db.Get(&rows, `SELECT COUNT(*) FROM user_daily_usage WHERE jid = '123456789012345@lid'`); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("LID usage rows = %d, want 0", rows)
	}
	if err := db.Get(&rows, `SELECT COUNT(*) FROM premium_users WHERE jid = '628123456789@s.whatsapp.net' AND expires_at IS NULL`); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("permanent canonical premium rows = %d, want 1", rows)
	}
}
