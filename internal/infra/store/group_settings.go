package store

import (
	"log"
)

func createGroupSettingsTable() {
	query := `
	CREATE TABLE IF NOT EXISTS group_settings (
		jid VARCHAR(255) PRIMARY KEY,
		nsfw_enabled BOOLEAN DEFAULT false
	);
	`
	_, err := db.Exec(query)
	if err != nil {
		log.Printf("Gagal membuat tabel group_settings: %v", err)
	}
}

func IsNSFWEnabled(jid string) bool {
	if db == nil {
		return false
	}
	var enabled bool
	query := db.Rebind(`SELECT nsfw_enabled FROM group_settings WHERE jid = ?`)
	err := db.Get(&enabled, query, jid)
	if err != nil {
		return false // Default is false
	}
	return enabled
}

func ToggleNSFW(jid string) (bool, error) {
	if db == nil {
		return false, nil
	}

	current := IsNSFWEnabled(jid)
	newStatus := !current

	query := ""
	if db.DriverName() == "postgres" {
		query = `
		INSERT INTO group_settings (jid, nsfw_enabled)
		VALUES (:jid, :nsfw_enabled)
		ON CONFLICT (jid) DO UPDATE SET
			nsfw_enabled = EXCLUDED.nsfw_enabled;
		`
	} else if db.DriverName() == "sqlite" || db.DriverName() == "sqlite3" {
		query = `
		INSERT INTO group_settings (jid, nsfw_enabled)
		VALUES (:jid, :nsfw_enabled)
		ON CONFLICT(jid) DO UPDATE SET
			nsfw_enabled=excluded.nsfw_enabled;
		`
	} else {
		query = `
		INSERT OR REPLACE INTO group_settings (jid, nsfw_enabled)
		VALUES (:jid, :nsfw_enabled);
		`
	}

	_, err := db.NamedExec(query, map[string]interface{}{
		"jid":          jid,
		"nsfw_enabled": newStatus,
	})
	if err != nil {
		return false, err
	}

	return newStatus, nil
}
