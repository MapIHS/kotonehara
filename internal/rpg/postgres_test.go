package rpg

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// The same game, HTTP, rollback and persistence suite runs on both drivers.
// PostgreSQL fixtures use separate schemas in a disposable local rpg_test DB.
// Never load DATABASE_URL or any application env file for these tests.
func openPostgresTest(t *testing.T, dsn, path string) (*sqlx.DB, *Service) {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path != "/rpg_test" ||
		(u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
		t.Fatal("RPG_TEST_POSTGRES_URL must target a disposable local database named rpg_test")
	}
	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "rpg_test_" + digest(path)[:24]
	if _, err = admin.Exec(`CREATE SCHEMA IF NOT EXISTS ` + schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sqlx.Connect("postgres", u.String())
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	db.SetMaxOpenConns(5)
	t.Cleanup(func() {
		_ = db.Close()
		_, err := admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
		_ = admin.Close()
		if err != nil {
			t.Errorf("cleanup test schema: %v", err)
		}
	})
	s, err := New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	s.random = repeatingByte(0x10)
	return db, s
}

func TestPostgresIndependentServices(t *testing.T) {
	if os.Getenv("RPG_TEST_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	path := filepath.Join(t.TempDir(), "shared")
	_, first := openTest(t, path)
	_, second := openTest(t, path)
	ctx := context.Background()
	// Different Service instances have different mutexes and connection pools.
	// Their concurrent first login must still create only one wallet.
	var wg sync.WaitGroup
	profiles := make(chan Profile, 2)
	for _, s := range []*Service{first, second} {
		wg.Go(func() {
			p, err := s.EnsurePlayer(ctx, []string{"12345@lid"}, "Concurrent")
			if err != nil {
				t.Error(err)
				return
			}
			profiles <- p
		})
	}
	wg.Wait()
	close(profiles)
	var player string
	for p := range profiles {
		if player != "" && player != p.ID {
			t.Fatal("duplicate identity wallet")
		}
		player = p.ID
	}
	if player == "" {
		t.Fatal("no player")
	}
	for _, s := range []*Service{first, second} {
		wg.Go(func() {
			out, err := s.Summon(ctx, player, "shared-request", BannerID, 10)
			if err != nil {
				t.Error(err)
				return
			}
			if out.Profile.Shards != 0 || len(out.Results) != 10 {
				t.Error("duplicate charge or missing receipt")
			}
		})
	}
	wg.Wait()
	var receipts int
	if err := first.db.Get(&receipts, `SELECT COUNT(*) FROM rpg_requests`); err != nil || receipts != 1 {
		t.Fatal("request not idempotent across services", receipts, err)
	}
	ticket, err := first.IssueTicket(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	successes := make(chan string, 2)
	for _, s := range []*Service{first, second} {
		wg.Go(func() {
			session, err := s.ExchangeTicket(ctx, ticket)
			if err == nil {
				successes <- session
			} else {
				requireCode(t, err, "ticket_invalid")
			}
		})
	}
	wg.Wait()
	close(successes)
	if len(successes) != 1 {
		t.Fatal("ticket must be consumed exactly once")
	}
}
