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
	db *sqlx.DB
}

func newStore(db *sqlx.DB) *store {
	return &store{db: db}
}

// IsPremium checks whether a JID has an active premium subscription.
func (s *store) IsPremium(ctx context.Context, jid string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM premium_users
		 WHERE jid = $1 AND (expires_at IS NULL OR expires_at > NOW())`,
		jid,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check premium: %w", err)
	}
	return count > 0, nil
}

// IncrementAndGet atomically increments the daily usage counter for a JID
// using a lazy-reset UPSERT. Returns the new count after increment.
func (s *store) IncrementAndGet(ctx context.Context, jid string) (int, error) {
	// Convert UTC to WIB (Asia/Jakarta) for date comparison
	var count int
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO user_daily_usage (jid, usage_count, reset_date, updated_at)
		 VALUES ($1, 1, (NOW() AT TIME ZONE 'Asia/Jakarta')::DATE::TEXT, NOW())
		 ON CONFLICT(jid) DO UPDATE SET
		   usage_count = CASE
		     WHEN user_daily_usage.reset_date < (NOW() AT TIME ZONE 'Asia/Jakarta')::DATE::TEXT THEN 1
		     ELSE user_daily_usage.usage_count + 1
		   END,
		   reset_date = CASE
		     WHEN user_daily_usage.reset_date < (NOW() AT TIME ZONE 'Asia/Jakarta')::DATE::TEXT THEN (NOW() AT TIME ZONE 'Asia/Jakarta')::DATE::TEXT
		     ELSE user_daily_usage.reset_date
		   END,
		   updated_at = NOW()
		 RETURNING usage_count`,
		jid,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("increment usage: %w", err)
	}
	return count, nil
}

// GetUsage returns the current usage count for a JID without incrementing.
func (s *store) GetUsage(ctx context.Context, jid string) (int, string, error) {
	var count int
	var resetDate string
	err := s.db.QueryRowContext(ctx,
		`SELECT usage_count, reset_date FROM user_daily_usage WHERE jid = $1`,
		jid,
	).Scan(&count, &resetDate)
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

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO premium_users (jid, added_by, expires_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT(jid) DO UPDATE SET
		   added_by = excluded.added_by,
		   expires_at = excluded.expires_at`,
		jid, addedBy, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("add premium: %w", err)
	}
	return nil
}

// RemovePremium removes a JID from premium.
func (s *store) RemovePremium(ctx context.Context, jid string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM premium_users WHERE jid = $1`, jid,
	)
	if err != nil {
		return fmt.Errorf("remove premium: %w", err)
	}
	return nil
}

// ListPremium returns all active premium users.
func (s *store) ListPremium(ctx context.Context) ([]PremiumUser, error) {
	var users []PremiumUser
	err := s.db.SelectContext(ctx, &users,
		`SELECT jid, added_by, expires_at, created_at
		 FROM premium_users
		 WHERE expires_at IS NULL OR expires_at > NOW()
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list premium: %w", err)
	}
	return users, nil
}
