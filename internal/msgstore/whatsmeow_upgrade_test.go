package msgstore

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/store/sqlstore"
)

// Uses the same isolated SQLite/PostgreSQL fixture as the message-store suite.
// Exercises the library's real migration code, never the production database.
func TestWhatsmeowFreshAndV15Migration(t *testing.T) {
	for _, previous := range []string{"fresh", "v15"} {
		t.Run(previous, func(t *testing.T) {
			db, _ := fixture(t, Options{})
			if db.DriverName() == "sqlite" {
				if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
					t.Fatal(err)
				}
			}
			if previous == "v15" {
				for _, q := range []string{
					`CREATE TABLE whatsmeow_version(version INTEGER,compat INTEGER)`,
					`INSERT INTO whatsmeow_version(version,compat) VALUES(15,8)`,
					`CREATE TABLE whatsmeow_chat_settings(our_jid TEXT,chat_jid TEXT,muted_until BIGINT NOT NULL DEFAULT 0,pinned BOOLEAN NOT NULL DEFAULT false,archived BOOLEAN NOT NULL DEFAULT false,PRIMARY KEY(our_jid,chat_jid))`,
					`INSERT INTO whatsmeow_chat_settings(our_jid,chat_jid,muted_until,pinned,archived) VALUES('bot@s.whatsapp.net','group@g.us',12345,true,false)`,
				} {
					if _, err := db.Exec(q); err != nil {
						t.Fatal(err)
					}
				}
			}
			container := sqlstore.NewWithDB(db.DB, db.DriverName(), nil)
			for i := 0; i < 2; i++ {
				if err := container.Upgrade(context.Background()); err != nil {
					t.Fatalf("upgrade %d: %v", i, err)
				}
			}
			var version int
			if err := db.Get(&version, "SELECT version FROM whatsmeow_version"); err != nil || version != 16 {
				t.Fatalf("version=%d err=%v", version, err)
			}
			if previous == "v15" {
				var muted int64
				var pinned bool
				var root string
				if err := db.QueryRow(`SELECT muted_until,pinned,wasa_root_secret_id FROM whatsmeow_chat_settings WHERE our_jid='bot@s.whatsapp.net' AND chat_jid='group@g.us'`).Scan(&muted, &pinned, &root); err != nil {
					t.Fatal(err)
				}
				if muted != 12345 || !pinned || root != "" {
					t.Fatal("existing chat settings changed during migration")
				}
			} else {
				if _, err := db.Exec(`SELECT wasa_root_secret_id FROM whatsmeow_chat_settings LIMIT 0`); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
