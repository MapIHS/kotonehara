package quota

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T, migrate bool) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if migrate {
		if err := Migrate(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

const user = "628123456789@s.whatsapp.net"

func TestFreeLimitEnforced(t *testing.T) {
	Init(newTestDB(t, true), 2, nil, true)
	ctx := context.Background()
	for i := 1; i <= 2; i++ {
		if ok, _, err := Global().CheckIdentity(ctx, user, nil); !ok || err != nil {
			t.Fatalf("call %d blocked: %v", i, err)
		}
	}
	ok, msg, err := Global().CheckIdentity(ctx, user, nil)
	if ok || msg == "" || err != nil {
		t.Fatalf("third call allowed: ok=%t msg=%q err=%v", ok, msg, err)
	}
}

func TestZeroLimitIsUnlimited(t *testing.T) {
	Init(newTestDB(t, true), 0, nil, true)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if ok, _, err := Global().CheckIdentity(ctx, user, nil); !ok || err != nil {
			t.Fatalf("FREE_DAILY_LIMIT=0 blocked call %d", i)
		}
	}
	info, err := Global().GetIdentityUsageInfo(ctx, user, nil)
	if err != nil || info.MaxLimit != -1 {
		t.Fatalf("usage info should report unlimited: %+v %v", info, err)
	}
}

func TestStoreFailurePolicy(t *testing.T) {
	ctx := context.Background()
	// Without migrations every quota query fails.
	Init(newTestDB(t, false), 10, nil, true)
	if ok, _, err := Global().CheckIdentity(ctx, user, nil); !ok || err != nil {
		t.Fatalf("fail-open blocked on store error: ok=%t err=%v", ok, err)
	}
	Init(newTestDB(t, false), 10, nil, false)
	ok, msg, err := Global().CheckIdentity(ctx, user, nil)
	if ok || msg == "" || err != nil {
		t.Fatalf("fail-closed allowed on store error: ok=%t msg=%q err=%v", ok, msg, err)
	}
}

func TestOwnerBypassesEvenWhenStoreFails(t *testing.T) {
	Init(newTestDB(t, false), 1, []string{user}, false)
	if ok, _, _ := Global().CheckIdentity(context.Background(), user, nil); !ok {
		t.Fatal("owner blocked")
	}
}
