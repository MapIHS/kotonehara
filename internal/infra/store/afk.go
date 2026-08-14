package store

import (
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

func SetAFK(jid string, reason string) {
	if db == nil {
		return
	}
	
	// PostgreSQL uses $1, SQLite uses ?
	// To be safe and compatible with sqlx across multiple drivers, we can use NamedExec, or just DB-specific dialect if necessary.
	// We'll use NamedExec.
	query := `
	INSERT INTO afk_users (jid, reason, created_at)
	VALUES (:jid, :reason, :created_at)
	ON CONFLICT (jid) DO UPDATE SET
		reason = EXCLUDED.reason,
		created_at = EXCLUDED.created_at;
	`
	// Note: SQLite supports ON CONFLICT since 3.24.0. For standard compatibility:
	if db.DriverName() == "postgres" {
		query = `
		INSERT INTO afk_users (jid, reason, created_at)
		VALUES (:jid, :reason, :created_at)
		ON CONFLICT (jid) DO UPDATE SET
			reason = EXCLUDED.reason,
			created_at = EXCLUDED.created_at;
		`
	} else if db.DriverName() == "sqlite" || db.DriverName() == "sqlite3" {
		query = `
		INSERT INTO afk_users (jid, reason, created_at)
		VALUES (:jid, :reason, :created_at)
		ON CONFLICT(jid) DO UPDATE SET
			reason=excluded.reason,
			created_at=excluded.created_at;
		`
	} else {
		// Fallback for simple INSERT OR REPLACE if it's sqlite without ON CONFLICT (older versions)
		// But modernc.org/sqlite supports it.
		query = `
		INSERT OR REPLACE INTO afk_users (jid, reason, created_at)
		VALUES (:jid, :reason, :created_at);
		`
		if db.DriverName() == "postgres" {
			query = `
			INSERT INTO afk_users (jid, reason, created_at)
			VALUES (:jid, :reason, :created_at)
			ON CONFLICT (jid) DO UPDATE SET
				reason = EXCLUDED.reason,
				created_at = EXCLUDED.created_at;
			`
		}
	}

	_, err := db.NamedExec(query, map[string]interface{}{
		"jid":        jid,
		"reason":     reason,
		"created_at": time.Now(),
	})
	if err != nil {
		log.Printf("Gagal set AFK: %v", err)
	}
}

func ClearAFK(jid string) (AFKState, bool) {
	if db == nil {
		return AFKState{}, false
	}

	state, ok := GetAFK(jid)
	if !ok {
		return AFKState{}, false
	}

	// Use Rebind for driver-agnostic query (?)
	query := db.Rebind(`DELETE FROM afk_users WHERE jid = ?`)
	_, err := db.Exec(query, jid)
	if err != nil {
		log.Printf("Gagal clear AFK: %v", err)
		return AFKState{}, false
	}

	return state, true
}

func GetAFK(jid string) (AFKState, bool) {
	var state AFKState
	if db == nil {
		return state, false
	}

	query := db.Rebind(`SELECT jid, reason, created_at FROM afk_users WHERE jid = ? LIMIT 1`)
	err := db.Get(&state, query, jid)
	if err != nil {
		return state, false
	}

	return state, true
}
