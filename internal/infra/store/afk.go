package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
)

type AFKState struct {
	JID    string    `db:"jid"`
	Reason string    `db:"reason"`
	Time   time.Time `db:"created_at"`
}

var db *sqlx.DB

func InitDB(database *sqlx.DB) {
	db = database
	createTable()
	createGroupSettingsTable()
}

func createTable() {
	query := `
	CREATE TABLE IF NOT EXISTS afk_users (
		jid VARCHAR(255) PRIMARY KEY,
		reason TEXT,
		created_at TIMESTAMP
	);
	`
	_, err := db.Exec(query)
	if err != nil {
		log.Printf("Gagal membuat tabel afk_users: %v", err)
	}
}

func SetAFK(ctx context.Context, jid string, reason string) error {
	if db == nil {
		return errors.New("database belum diinisialisasi")
	}
	if jid == "" {
		return errors.New("jid kosong")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	query := `
	INSERT INTO afk_users (jid, reason, created_at)
	VALUES (:jid, :reason, :created_at)
	ON CONFLICT (jid) DO UPDATE SET
		reason = EXCLUDED.reason,
		created_at = EXCLUDED.created_at;
	`
	if db.DriverName() == "sqlite" || db.DriverName() == "sqlite3" {
		query = `
		INSERT INTO afk_users (jid, reason, created_at)
		VALUES (:jid, :reason, :created_at)
		ON CONFLICT(jid) DO UPDATE SET
			reason = excluded.reason,
			created_at = excluded.created_at;
		`
	}

	_, err := db.NamedExecContext(ctx, query, map[string]interface{}{
		"jid":        jid,
		"reason":     reason,
		"created_at": time.Now(),
	})
	if err != nil {
		return fmt.Errorf("set AFK: %w", err)
	}
	return nil
}

func ClearAFK(ctx context.Context, jids ...string) (AFKState, bool, error) {
	if db == nil {
		return AFKState{}, false, errors.New("database belum diinisialisasi")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	state, ok, err := GetAFK(ctx, jids...)
	if err != nil || !ok {
		return AFKState{}, false, err
	}

	query := db.Rebind(`DELETE FROM afk_users WHERE jid = ?`)
	for _, jid := range uniqueNonEmpty(jids) {
		if _, err := db.ExecContext(ctx, query, jid); err != nil {
			return AFKState{}, false, fmt.Errorf("clear AFK: %w", err)
		}
	}

	return state, true, nil
}

func GetAFK(ctx context.Context, jids ...string) (AFKState, bool, error) {
	var state AFKState
	if db == nil {
		return state, false, errors.New("database belum diinisialisasi")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	query := db.Rebind(`SELECT jid, reason, created_at FROM afk_users WHERE jid = ? LIMIT 1`)
	for _, jid := range uniqueNonEmpty(jids) {
		err := db.GetContext(ctx, &state, query, jid)
		if err == nil {
			return state, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return state, false, fmt.Errorf("get AFK: %w", err)
		}
	}
	return state, false, nil
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
