package clients

import (
	"sync"
	"time"
)

type adminCacheEntry struct {
	admins  []string
	expires time.Time
}

type adminCache struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[string]adminCacheEntry
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
	c.mu.RUnlock()

	if !ok || now.After(ent.expires) {
		return nil, false
	}

	out := make([]string, len(ent.admins))
	copy(out, ent.admins)
	return out, true
}

func (c *adminCache) set(key string, admins []string) {
	ent := adminCacheEntry{
		admins:  admins,
		expires: time.Now().Add(c.ttl),
	}
	c.mu.Lock()
	c.m[key] = ent
	c.mu.Unlock()
}

func (c *adminCache) invalidate(key string) {
	c.mu.Lock()
	delete(c.m, key)
	c.mu.Unlock()
}
