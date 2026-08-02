package quota

import (
	"context"
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
)

// CheckResult holds the result of a quota check.
type CheckResult struct {
	Allowed   bool
	UsedCount int
	MaxLimit  int // -1 means unlimited
	Message   string
}

// Checker is the main quota enforcement engine.
type Checker struct {
	store     *store
	freeLimit int
	ownerJIDs map[string]bool
}

var globalChecker *Checker

// Init creates and stores the global Checker instance.
func Init(db *sqlx.DB, freeLimit int, owners []string) {
	m := make(map[string]bool, len(owners))
	for _, o := range owners {
		m[o] = true
	}
	globalChecker = &Checker{
		store:     newStore(db),
		freeLimit: freeLimit,
		ownerJIDs: m,
	}
}

// Global returns the global Checker instance.
func Global() *Checker {
	return globalChecker
}

// CheckCommand checks if a user is allowed to execute a quota-limited command.
// Returns (allowed bool, blockMessage string, err error).
// This signature is designed to be used with commands.SetQuotaCheck.
func (c *Checker) CheckCommand(ctx context.Context, jid string) (bool, string, error) {
	// 1. Owner bypass
	if c.ownerJIDs[jid] {
		return true, "", nil
	}

	// 2. Premium bypass
	isPremium, err := c.store.IsPremium(ctx, jid)
	if err != nil {
		log.Printf("quota: premium check error: %v", err)
		return true, "", nil // fail open
	}
	if isPremium {
		return true, "", nil
	}

	// 3. Free user: increment and check
	count, err := c.store.IncrementAndGet(ctx, jid)
	if err != nil {
		log.Printf("quota: increment error: %v", err)
		return true, "", nil // fail open
	}

	if count > c.freeLimit {
		msg := fmt.Sprintf(
			"🚫 *Limit Harian Tercapai!*\n\n"+
				"Kamu sudah menggunakan *%d/%d* kuota gratis hari ini.\n"+
				"Kuota akan reset otomatis besok pukul *00:00 WIB*.\n\n"+
				"⭐ Mau *unlimited*? Hubungi owner untuk upgrade ke *Premium*!\n"+
				"Ketik *.donasi* untuk info donasi.",
			c.freeLimit, c.freeLimit,
		)
		return false, msg, nil
	}

	return true, "", nil
}

// GetUsageInfo returns the current usage info for a JID (used by .quota command).
func (c *Checker) GetUsageInfo(ctx context.Context, jid string) (*UsageInfo, error) {
	// Owner
	if c.ownerJIDs[jid] {
		return &UsageInfo{MaxLimit: -1, IsPremium: true}, nil
	}

	// Premium
	isPremium, err := c.store.IsPremium(ctx, jid)
	if err != nil {
		return nil, err
	}
	if isPremium {
		return &UsageInfo{MaxLimit: -1, IsPremium: true}, nil
	}

	// Free
	count, resetDate, err := c.store.GetUsage(ctx, jid)
	if err != nil {
		return nil, err
	}
	return &UsageInfo{
		UsedCount: count,
		MaxLimit:  c.freeLimit,
		IsPremium: false,
		ResetDate: resetDate,
	}, nil
}

// IsPremium checks if a JID is premium (delegates to store).
func (c *Checker) IsPremium(ctx context.Context, jid string) (bool, error) {
	if c.ownerJIDs[jid] {
		return true, nil
	}
	return c.store.IsPremium(ctx, jid)
}

// AddPremium adds a premium user (delegates to store).
func (c *Checker) AddPremium(ctx context.Context, jid, addedBy string, days int) error {
	return c.store.AddPremium(ctx, jid, addedBy, days)
}

// RemovePremium removes a premium user (delegates to store).
func (c *Checker) RemovePremium(ctx context.Context, jid string) error {
	return c.store.RemovePremium(ctx, jid)
}

// ListPremium lists all active premium users (delegates to store).
func (c *Checker) ListPremium(ctx context.Context) ([]PremiumUser, error) {
	return c.store.ListPremium(ctx)
}
