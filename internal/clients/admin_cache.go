package clients

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	adminCacheMaxEntries    = 4096
	adminCacheSweepLimit    = 64
	adminCacheSweepInterval = 64
)

type adminCacheEntry struct {
	admins  []string
	expires time.Time
}

type adminCache struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[string]adminCacheEntry
	ops atomic.Uint64
}

func newAdminCache(ttl time.Duration) *adminCache {
	if ttl <= 0 {
		ttl = 45 * time.Second
	}
	return &adminCache{
		ttl: ttl,
		m:   make(map[string]adminCacheEntry),
	}
}

func (c *adminCache) get(key string) ([]string, bool) {
	now := time.Now()

	c.mu.RLock()
	ent, ok := c.m[key]
	if ok && now.Before(ent.expires) {
		out := append([]string(nil), ent.admins...)
		c.mu.RUnlock()
		c.maybeSweepExpired(now)
		return out, true
	}
	c.mu.RUnlock()

	if ok {
		c.mu.Lock()
		if current, exists := c.m[key]; exists && !now.Before(current.expires) {
			delete(c.m, key)
		}
		c.mu.Unlock()
	}
	c.maybeSweepExpired(now)
	return nil, false
}

func (c *adminCache) set(key string, admins []string) {
	now := time.Now()
	ent := adminCacheEntry{
		admins:  append([]string(nil), admins...),
		expires: now.Add(c.ttl),
	}
	c.mu.Lock()
	if _, exists := c.m[key]; !exists && len(c.m) >= adminCacheMaxEntries {
		c.sweepExpiredLocked(now, len(c.m))
		if len(c.m) >= adminCacheMaxEntries {
			for oldestKey := range c.m {
				delete(c.m, oldestKey)
				break
			}
		}
	}
	c.m[key] = ent
	c.mu.Unlock()
	c.maybeSweepExpired(now)
}

func (c *adminCache) maybeSweepExpired(now time.Time) {
	if c.ops.Add(1)%adminCacheSweepInterval != 0 {
		return
	}
	c.mu.Lock()
	c.sweepExpiredLocked(now, adminCacheSweepLimit)
	c.mu.Unlock()
}

func (c *adminCache) sweepExpiredLocked(now time.Time, limit int) {
	checked := 0
	for key, ent := range c.m {
		if checked >= limit {
			return
		}
		checked++
		if !now.Before(ent.expires) {
			delete(c.m, key)
		}
	}
}
