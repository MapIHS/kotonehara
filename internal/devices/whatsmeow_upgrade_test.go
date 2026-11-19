package devices

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"

	"go.mau.fi/whatsmeow/store/sqlstore"
)

// Exercises the library's real migration code, never the production database.
func TestWhatsmeowFreshAndV15Migration(t *testing.T) {
	for _, previous := range []string{"fresh", "v15"} {
		t.Run(previous, func(t *testing.T) {
			db := whatsmeowMigrationDB(t)
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

// Only disposable loopback PostgreSQL or a temporary SQLite database is allowed.
func whatsmeowMigrationDB(t *testing.T) *sqlx.DB {
	t.Helper()
	driver, dsn := "sqlite", filepath.Join(t.TempDir(), "whatsmeow.db")
	if raw := os.Getenv("WHATSMEOW_TEST_POSTGRES_URL"); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path != "/rpg_test" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") {
			t.Fatal("test URL must name a disposable loopback rpg_test database")
		}
		admin, err := sqlx.Connect("postgres", raw)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(dsn))
		schema := fmt.Sprintf("whatsmeow_test_%x", hash[:12])
		if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
				t.Error(err)
			}
			admin.Close()
		})
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		driver, dsn = "postgres", u.String()
	}
	db, err := sqlx.Connect(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}
