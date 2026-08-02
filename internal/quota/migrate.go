package quota

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

var migrations = []string{
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

// Migrate creates the quota tables if they don't exist.
func Migrate(ctx context.Context, db *sqlx.DB) error {
	for _, ddl := range migrations {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("quota migrate: %w", err)
		}
	}
	return nil
}
