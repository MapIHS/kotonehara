package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
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

func MigrateAFKIdentities(ctx context.Context) error {
	if db == nil {
		return errors.New("database belum diinisialisasi")
	}
	if err := normalizeAFKRows(ctx); err != nil {
		return err
	}
	rows, err := db.QueryxContext(ctx, `
		SELECT m.lid, m.pn FROM whatsmeow_lid_map m
		WHERE EXISTS (SELECT 1 FROM afk_users a WHERE a.jid = m.lid || '@lid')`)
	if err != nil {
		return fmt.Errorf("read LID mappings for AFK: %w", err)
	}
	type mapping struct{ lid, pn string }
	var mappings []mapping
	for rows.Next() {
		var lidUser, pnUser string
		if err := rows.Scan(&lidUser, &pnUser); err != nil {
			rows.Close()
			return fmt.Errorf("scan LID mapping for AFK: %w", err)
		}
		mappings = append(mappings, mapping{lid: lidUser, pn: pnUser})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, mapping := range mappings {
		if err := mergeAFKIdentity(ctx, mapping.pn+"@s.whatsapp.net", mapping.lid+"@lid"); err != nil {
			return err
		}
	}
	return nil
}

func normalizeAFKRows(ctx context.Context) error {
	var states []AFKState
	if err := db.SelectContext(ctx, &states, `SELECT jid, reason, created_at FROM afk_users`); err != nil {
		return fmt.Errorf("read AFK rows: %w", err)
	}
	for _, state := range states {
		at := strings.IndexByte(state.JID, '@')
		if at < 0 {
			continue
		}
		user, server := state.JID[:at], state.JID[at:]
		colon := strings.IndexByte(user, ':')
		if colon < 0 {
			continue
		}
		if err := mergeAFKIdentity(ctx, user[:colon]+server, state.JID); err != nil {
			return err
		}
	}
	return nil
}

func mergeAFKIdentity(ctx context.Context, canonical string, aliases ...string) error {
	all := uniqueNonEmpty(append(aliases, canonical))
	if len(all) == 0 {
		return nil
	}

	var latest AFKState
	found := false
	query := db.Rebind(`SELECT jid, reason, created_at FROM afk_users WHERE jid = ? LIMIT 1`)
	for _, jid := range all {
		var state AFKState
		err := db.GetContext(ctx, &state, query, jid)
		if err == nil && (!found || state.Time.After(latest.Time)) {
			latest, found = state, true
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read AFK alias: %w", err)
		}
	}
	if !found {
		return nil
	}
	return setAFK(ctx, canonical, latest.Reason, latest.Time, all...)
}

func SetAFK(ctx context.Context, jid string, reason string, aliases ...string) error {
	return setAFK(ctx, jid, reason, time.Now(), aliases...)
}

func setAFK(ctx context.Context, jid string, reason string, createdAt time.Time, aliases ...string) error {
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

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set AFK: %w", err)
	}
	defer tx.Rollback()

	deleteQuery := db.Rebind(`DELETE FROM afk_users WHERE jid = ?`)
	for _, alias := range uniqueNonEmpty(append(aliases, jid)) {
		if _, err := tx.ExecContext(ctx, deleteQuery, alias); err != nil {
			return fmt.Errorf("clean alias AFK: %w", err)
		}
	}

	_, err = tx.NamedExecContext(ctx, query, map[string]interface{}{
		"jid":        jid,
		"reason":     reason,
		"created_at": createdAt,
	})
	if err != nil {
		return fmt.Errorf("set AFK: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set AFK: %w", err)
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

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return AFKState{}, false, fmt.Errorf("begin clear AFK: %w", err)
	}
	defer tx.Rollback()

	query := db.Rebind(`DELETE FROM afk_users WHERE jid = ?`)
	for _, jid := range uniqueNonEmpty(jids) {
		if _, err := tx.ExecContext(ctx, query, jid); err != nil {
			return AFKState{}, false, fmt.Errorf("clear AFK: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return AFKState{}, false, fmt.Errorf("commit clear AFK: %w", err)
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
