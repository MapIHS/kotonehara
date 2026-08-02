package quota

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// PremiumUser represents a premium user record.
type PremiumUser struct {
	JID       string     `db:"jid"`
	AddedBy   string     `db:"added_by"`
	ExpiresAt *time.Time `db:"expires_at"`
	CreatedAt time.Time  `db:"created_at"`
}

// UsageInfo holds the current usage state for a user.
type UsageInfo struct {
	UsedCount int
	MaxLimit  int  // -1 means unlimited
	IsPremium bool
	ResetDate string
}

type store struct {
	db     *sqlx.DB
	isPG   bool
}

func newStore(db *sqlx.DB) *store {
	driver := db.DriverName()
	return &store{
		db:   db,
		isPG: driver == "postgres" || driver == "pgx",
	}
}

// ph returns the correct placeholder for the given index (1-based).
// PostgreSQL uses $1, $2, etc. SQLite uses ?.
func (s *store) ph(n int) string {
	if s.isPG {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// nowExpr returns the SQL expression for "current timestamp".
func (s *store) nowExpr() string {
	if s.isPG {
		return "NOW()"
	}
	return "DATETIME('now')"
}

// todayWIBExpr returns the SQL expression for "today's date in WIB".
func (s *store) todayWIBExpr() string {
	if s.isPG {
		return "(NOW() AT TIME ZONE 'Asia/Jakarta')::DATE::TEXT"
	}
	return "DATE('now', '+7 hours')"
}

// IsPremium checks whether a JID has an active premium subscription.
func (s *store) IsPremium(ctx context.Context, jid string) (bool, error) {
	var count int
	q := fmt.Sprintf(
		`SELECT COUNT(*) FROM premium_users
		 WHERE jid = %s AND (expires_at IS NULL OR expires_at > %s)`,
		s.ph(1), s.nowExpr(),
	)
	err := s.db.QueryRowContext(ctx, q, jid).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check premium: %w", err)
	}
	return count > 0, nil
}

// IncrementAndGet atomically increments the daily usage counter for a JID
// using a lazy-reset UPSERT. Returns the new count after increment.
func (s *store) IncrementAndGet(ctx context.Context, jid string) (int, error) {
	today := s.todayWIBExpr()
	now := s.nowExpr()

	// PostgreSQL requires table-qualified columns in ON CONFLICT DO UPDATE
	usageCol := "usage_count"
	resetCol := "reset_date"
	if s.isPG {
		usageCol = "user_daily_usage.usage_count"
		resetCol = "user_daily_usage.reset_date"
	}

	q := fmt.Sprintf(
		`INSERT INTO user_daily_usage (jid, usage_count, reset_date, updated_at)
		 VALUES (%s, 1, %s, %s)
		 ON CONFLICT(jid) DO UPDATE SET
		   usage_count = CASE
		     WHEN %s < %s THEN 1
		     ELSE %s + 1
		   END,
		   reset_date = CASE
		     WHEN %s < %s THEN %s
		     ELSE %s
		   END,
		   updated_at = %s
		 RETURNING usage_count`,
		s.ph(1), today, now,
		resetCol, today,
		usageCol,
		resetCol, today, today,
		resetCol,
		now,
	)

	var count int
	err := s.db.QueryRowContext(ctx, q, jid).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("increment usage: %w", err)
	}
	return count, nil
}

// GetUsage returns the current usage count for a JID without incrementing.
func (s *store) GetUsage(ctx context.Context, jid string) (int, string, error) {
	var count int
	var resetDate string
	q := fmt.Sprintf(
		`SELECT usage_count, reset_date FROM user_daily_usage WHERE jid = %s`,
		s.ph(1),
	)
	err := s.db.QueryRowContext(ctx, q, jid).Scan(&count, &resetDate)
	if err == sql.ErrNoRows {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("get usage: %w", err)
	}
	// Check if reset_date has passed (lazy check without updating)
	today := time.Now().UTC().Add(7 * time.Hour).Format("2006-01-02")
	if resetDate < today {
		return 0, today, nil // Already expired, report as 0
	}
	return count, resetDate, nil
}

// AddPremium adds a JID as premium. If days <= 0, it's permanent (no expiry).
func (s *store) AddPremium(ctx context.Context, jid, addedBy string, days int) error {
	var expiresAt *time.Time
	if days > 0 {
		t := time.Now().UTC().AddDate(0, 0, days)
		expiresAt = &t
	}

	q := fmt.Sprintf(
		`INSERT INTO premium_users (jid, added_by, expires_at)
		 VALUES (%s, %s, %s)
		 ON CONFLICT(jid) DO UPDATE SET
		   added_by = excluded.added_by,
		   expires_at = excluded.expires_at`,
		s.ph(1), s.ph(2), s.ph(3),
	)

	_, err := s.db.ExecContext(ctx, q, jid, addedBy, expiresAt)
	if err != nil {
		return fmt.Errorf("add premium: %w", err)
	}
	return nil
}

// RemovePremium removes a JID from premium.
func (s *store) RemovePremium(ctx context.Context, jid string) error {
	q := fmt.Sprintf(`DELETE FROM premium_users WHERE jid = %s`, s.ph(1))
	_, err := s.db.ExecContext(ctx, q, jid)
	if err != nil {
		return fmt.Errorf("remove premium: %w", err)
	}
	return nil
}

// ListPremium returns all active premium users.
func (s *store) ListPremium(ctx context.Context) ([]PremiumUser, error) {
	var users []PremiumUser
	q := fmt.Sprintf(
		`SELECT jid, added_by, expires_at, created_at
		 FROM premium_users
		 WHERE expires_at IS NULL OR expires_at > %s
		 ORDER BY created_at DESC`,
		s.nowExpr(),
	)
	err := s.db.SelectContext(ctx, &users, q)
	if err != nil {
		return nil, fmt.Errorf("list premium: %w", err)
	}
	return users, nil
}
