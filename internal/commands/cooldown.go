package commands

import (
	"sync"
	"time"

	staticassets "github.com/MapIHS/kotonehara/internal/static"
)

const cooldownSweepLimit = 16

var cd = struct {
	mu sync.Mutex
	d  time.Duration
	m  map[string]time.Time
	n  map[string]time.Time
}{
	d: 3 * time.Second,
	m: map[string]time.Time{},
	n: map[string]time.Time{},
}

func SetCooldown(d time.Duration) {
	if d <= 0 {
		d = 0
	}
	cd.mu.Lock()
	cd.d = d
	// Configuration changes mark a new lifecycle; old deadlines must not leak
	// into a newly started client.
	cd.m = make(map[string]time.Time)
	cd.n = make(map[string]time.Time)
	cd.mu.Unlock()
}

func allowCooldown(key string) bool {
	return allowCooldownAliases([]string{key})
}

func allowCooldownAliases(keys []string) bool {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	if cd.d == 0 {
		return true
	}
	now := time.Now()
	sweepCooldownLocked(now, cooldownSweepLimit)
	for _, key := range keys {
		if until, ok := cd.m[key]; ok && now.Before(until) {
			return false
		}
	}
	until := now.Add(cd.d)
	for _, key := range keys {
		if key != "" {
			cd.m[key] = until
		}
	}
	return true
}

func shouldSendCooldownSticker(key string) bool {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	if cd.d == 0 {
		return false
	}
	now := time.Now()
	sweepCooldownLocked(now, cooldownSweepLimit)
	if until, ok := cd.n[key]; ok && now.Before(until) {
		return false
	}
	cd.n[key] = now.Add(cd.d)
	return true
}

func sweepCooldownLocked(now time.Time, limit int) {
	for _, deadlines := range []map[string]time.Time{cd.m, cd.n} {
		checked := 0
		for key, until := range deadlines {
			if checked >= limit {
				break
			}
			checked++
			if !now.Before(until) {
				delete(deadlines, key)
			}
		}
	}
}

func loadSpamSticker() ([]byte, error) {
	return staticassets.SpamSticker, nil
}

func loadOwnerSticker() ([]byte, error) {
	return staticassets.OwnerSticker, nil
}
