package quota

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

func getMigrations(driver string) []string {
	if driver == "sqlite3" || driver == "sqlite" {
		return []string{
			`CREATE TABLE IF NOT EXISTS premium_users (
				jid        TEXT PRIMARY KEY,
				added_by   TEXT NOT NULL,
				expires_at DATETIME,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE TABLE IF NOT EXISTS user_daily_usage (
				jid         TEXT PRIMARY KEY,
				usage_count INTEGER NOT NULL DEFAULT 0,
				reset_date  TEXT NOT NULL,
				updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
		}
	}
	// PostgreSQL
	return []string{
		`CREATE TABLE IF NOT EXISTS premium_users (
			jid        TEXT PRIMARY KEY,
			added_by   TEXT NOT NULL,
			expires_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS user_daily_usage (
			jid         TEXT PRIMARY KEY,
			usage_count INTEGER NOT NULL DEFAULT 0,
			reset_date  TEXT NOT NULL,
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}
}

type usageMigrationRow struct {
	JID        string    `db:"jid"`
	UsageCount int       `db:"usage_count"`
	ResetDate  string    `db:"reset_date"`
	UpdatedAt  time.Time `db:"updated_at"`
}

func MigrateIdentityAliases(ctx context.Context, db *sqlx.DB) error {
	if err := normalizeQuotaRows(ctx, db); err != nil {
		return err
	}
	rows, err := db.QueryxContext(ctx, `
		SELECT m.lid, m.pn FROM whatsmeow_lid_map m
		WHERE EXISTS (SELECT 1 FROM user_daily_usage u WHERE u.jid = m.lid || '@lid')
		   OR EXISTS (SELECT 1 FROM premium_users p WHERE p.jid = m.lid || '@lid')
		   OR EXISTS (SELECT 1 FROM premium_users p WHERE p.added_by = m.lid || '@lid')`)
	if err != nil {
		return fmt.Errorf("read identity mappings: %w", err)
	}
	type mapping struct{ lid, pn string }
	var mappings []mapping
	for rows.Next() {
		var lidUser, pnUser string
		if err := rows.Scan(&lidUser, &pnUser); err != nil {
			rows.Close()
			return fmt.Errorf("scan identity mapping: %w", err)
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
		if err := mergeQuotaIdentity(ctx, db, mapping.pn+"@s.whatsapp.net", mapping.lid+"@lid"); err != nil {
			return err
		}
	}
	return nil
}

func normalizeQuotaRows(ctx context.Context, db *sqlx.DB) error {
	var jids []string
	if err := db.SelectContext(ctx, &jids, `
		SELECT jid FROM user_daily_usage
		UNION SELECT jid FROM premium_users
		UNION SELECT added_by FROM premium_users`); err != nil {
		return fmt.Errorf("read legacy quota identities: %w", err)
	}
	for _, jid := range jids {
		canonical := normalizeJID(jid)
		if canonical == jid {
			continue
		}
		if err := mergeQuotaIdentity(ctx, db, canonical, jid); err != nil {
			return err
		}
	}
	return nil
}

func mergeQuotaIdentity(ctx context.Context, db *sqlx.DB, canonical, alias string) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin identity migration: %w", err)
	}
	defer tx.Rollback()

	query, args, err := sqlx.In(`SELECT jid, usage_count, reset_date, updated_at FROM user_daily_usage WHERE jid IN (?, ?)`, canonical, alias)
	if err != nil {
		return err
	}
	var usages []usageMigrationRow
	if err := tx.SelectContext(ctx, &usages, db.Rebind(query), args...); err != nil {
		return fmt.Errorf("read usage aliases: %w", err)
	}
	hasUsageAlias := false
	for _, usage := range usages {
		if usage.JID == alias {
			hasUsageAlias = true
			break
		}
	}
	if hasUsageAlias {
		latestDate := ""
		latestUpdate := time.Time{}
		count := 0
		for _, usage := range usages {
			if usage.ResetDate > latestDate {
				latestDate, count = usage.ResetDate, usage.UsageCount
			} else if usage.ResetDate == latestDate {
				count += usage.UsageCount
			}
			if usage.UpdatedAt.After(latestUpdate) {
				latestUpdate = usage.UpdatedAt
			}
		}
		if _, err := tx.ExecContext(ctx, db.Rebind(`DELETE FROM user_daily_usage WHERE jid IN (?, ?)`), canonical, alias); err != nil {
			return fmt.Errorf("delete usage aliases: %w", err)
		}
		if _, err := tx.ExecContext(ctx, db.Rebind(`INSERT INTO user_daily_usage (jid, usage_count, reset_date, updated_at) VALUES (?, ?, ?, ?)`), canonical, count, latestDate, latestUpdate); err != nil {
			return fmt.Errorf("write canonical usage: %w", err)
		}
	}

	type premiumMigrationRow struct {
		JID       string     `db:"jid"`
		AddedBy   string     `db:"added_by"`
		ExpiresAt *time.Time `db:"expires_at"`
		CreatedAt time.Time  `db:"created_at"`
	}
	query, args, err = sqlx.In(`SELECT jid, added_by, expires_at, created_at FROM premium_users WHERE jid IN (?, ?)`, canonical, alias)
	if err != nil {
		return err
	}
	var premiums []premiumMigrationRow
	if err := tx.SelectContext(ctx, &premiums, db.Rebind(query), args...); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read premium aliases: %w", err)
	}
	hasPremiumAlias := false
	for _, premium := range premiums {
		if premium.JID == alias {
			hasPremiumAlias = true
			break
		}
	}
	if hasPremiumAlias {
		selected := premiums[0]
		permanent := selected.ExpiresAt == nil
		for _, premium := range premiums[1:] {
			if premium.CreatedAt.Before(selected.CreatedAt) {
				selected.CreatedAt = premium.CreatedAt
			}
			if permanent {
				continue
			}
			if premium.ExpiresAt == nil {
				selected.AddedBy, selected.ExpiresAt, permanent = premium.AddedBy, nil, true
			} else if selected.ExpiresAt == nil || premium.ExpiresAt.After(*selected.ExpiresAt) {
				selected.AddedBy, selected.ExpiresAt = premium.AddedBy, premium.ExpiresAt
			}
		}
		if _, err := tx.ExecContext(ctx, db.Rebind(`DELETE FROM premium_users WHERE jid IN (?, ?)`), canonical, alias); err != nil {
			return fmt.Errorf("delete premium aliases: %w", err)
		}
		if _, err := tx.ExecContext(ctx, db.Rebind(`INSERT INTO premium_users (jid, added_by, expires_at, created_at) VALUES (?, ?, ?, ?)`), canonical, selected.AddedBy, selected.ExpiresAt, selected.CreatedAt); err != nil {
			return fmt.Errorf("write canonical premium: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`UPDATE premium_users SET added_by = ? WHERE added_by = ?`), canonical, alias); err != nil {
		return fmt.Errorf("canonicalize premium audit identity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit identity migration: %w", err)
	}
	return nil
}

// Migrate creates the quota tables if they don't exist.
func Migrate(ctx context.Context, db *sqlx.DB) error {
	for _, ddl := range getMigrations(db.DriverName()) {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("quota migrate: %w", err)
		}
	}
	return nil
}
