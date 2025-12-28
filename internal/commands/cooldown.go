package commands

import (
	"sync"
	"time"
)

var cd = struct {
	mu sync.Mutex
	d  time.Duration
	m  map[string]time.Time
}{
	d: 3 * time.Second,
	m: map[string]time.Time{},
}

func SetCooldown(d time.Duration) {
	if d <= 0 {
		d = 0
	}
	cd.mu.Lock()
	cd.d = d
	cd.mu.Unlock()
}

func allowCooldown(key string) bool {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	if cd.d == 0 {
		return true
	}
	now := time.Now()
	if until, ok := cd.m[key]; ok && now.Before(until) {
		return false
	}
	cd.m[key] = now.Add(cd.d)
	return true
}
