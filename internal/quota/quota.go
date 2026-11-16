package quota

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/MapIHS/kotonehara/internal/identity"
	"github.com/jmoiron/sqlx"
)

// normalizeJID strips the device suffix from a WhatsApp JID so that
// "628xxx:0@s.whatsapp.net" becomes "628xxx@s.whatsapp.net".
// This ensures consistent JID storage and lookup.
func normalizeJID(jid string) string {
	if parsed, err := identity.ParseUser(jid); err == nil {
		return parsed.String()
	}
	at := strings.IndexByte(jid, '@')
	if at < 0 {
		return jid
	}
	user := jid[:at]
	server := jid[at:]
	if colon := strings.IndexByte(user, ':'); colon >= 0 {
		user = user[:colon]
	}
	return user + server
}

// CheckResult holds the result of a quota check.
type CheckResult struct {
	Allowed   bool
	UsedCount int
	MaxLimit  int // -1 means unlimited
	Message   string
}

// Checker is the main quota enforcement engine.
type Checker struct {
	store      *store
	freeLimit  int
	ownerJIDs  map[string]bool
	identityMu sync.Mutex
	reconciled map[string]bool
	// failOpen decides what happens when the quota store errors: allow the
	// command (true) or block it with an explanatory message (false).
	failOpen bool
}

const unavailableMessage = "⚠️ Sistem kuota sedang bermasalah, jadi command belum bisa dipakai. Coba lagi nanti, yaa."

var globalChecker *Checker

// Init creates and stores the global Checker instance.
// A freeLimit of 0 or less means free users are unlimited.
func Init(db *sqlx.DB, freeLimit int, owners []string, failOpen bool) {
	m := make(map[string]bool, len(owners))
	for _, o := range owners {
		m[normalizeJID(o)] = true
	}
	globalChecker = &Checker{
		store:      newStore(db),
		freeLimit:  freeLimit,
		ownerJIDs:  m,
		reconciled: make(map[string]bool),
		failOpen:   failOpen,
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
	return c.CheckIdentity(ctx, jid, nil)
}

func (c *Checker) CheckIdentity(ctx context.Context, jid string, aliases []string) (bool, string, error) {
	jid = normalizeJID(jid)
	if err := c.reconcileAliases(ctx, jid, aliases); err != nil {
		return c.storeFailure("identity reconcile", err)
	}

	// 1. Owner bypass
	if c.ownerJIDs[jid] {
		return true, "", nil
	}

	// 2. Premium bypass
	isPremium, err := c.store.IsPremium(ctx, jid)
	if err != nil {
		return c.storeFailure("premium check", err)
	}
	if isPremium {
		return true, "", nil
	}

	// 3. Free users are unlimited when no positive limit is configured.
	if c.freeLimit <= 0 {
		return true, "", nil
	}

	// 4. Free user: increment and check
	count, err := c.store.IncrementAndGet(ctx, jid)
	if err != nil {
		return c.storeFailure("increment", err)
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

// storeFailure applies the configured policy to a quota store error. It never
// returns the error itself so callers get a definite allow/deny decision.
func (c *Checker) storeFailure(stage string, err error) (bool, string, error) {
	if c.failOpen {
		log.Printf("quota: %s error (fail-open, allowing): %v", stage, err)
		return true, "", nil
	}
	log.Printf("quota: %s error (fail-closed, blocking): %v", stage, err)
	return false, unavailableMessage, nil
}

func (c *Checker) reconcileAliases(ctx context.Context, canonical string, aliases []string) error {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	for _, alias := range aliases {
		alias = normalizeJID(alias)
		if alias == "" || alias == canonical {
			continue
		}
		pair := canonical + "|" + alias
		if c.reconciled[pair] {
			continue
		}
		if err := mergeQuotaIdentity(ctx, c.store.db, canonical, alias); err != nil {
			return err
		}
		c.store.invalidatePremium(canonical, alias)
		c.reconciled[pair] = true
	}
	return nil
}

// GetUsageInfo returns the current usage info for a JID (used by .quota command).
func (c *Checker) GetUsageInfo(ctx context.Context, jid string) (*UsageInfo, error) {
	return c.GetIdentityUsageInfo(ctx, jid, nil)
}

func (c *Checker) GetIdentityUsageInfo(ctx context.Context, jid string, aliases []string) (*UsageInfo, error) {
	jid = normalizeJID(jid)
	if err := c.reconcileAliases(ctx, jid, aliases); err != nil {
		return nil, err
	}

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

	// Free, but no positive limit configured.
	if c.freeLimit <= 0 {
		return &UsageInfo{MaxLimit: -1}, nil
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
	jid = normalizeJID(jid)
	if c.ownerJIDs[jid] {
		return true, nil
	}
	return c.store.IsPremium(ctx, jid)
}

// AddPremium adds a premium user (delegates to store).
func (c *Checker) AddPremium(ctx context.Context, jid, addedBy string, days int) error {
	return c.store.AddPremium(ctx, normalizeJID(jid), normalizeJID(addedBy), days)
}

// RemovePremium removes a premium user (delegates to store).
func (c *Checker) RemovePremium(ctx context.Context, jid string) error {
	return c.store.RemovePremium(ctx, normalizeJID(jid))
}

// ListPremium lists all active premium users (delegates to store).
func (c *Checker) ListPremium(ctx context.Context) ([]PremiumUser, error) {
	return c.store.ListPremium(ctx)
}
