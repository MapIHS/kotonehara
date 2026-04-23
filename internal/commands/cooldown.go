package commands

import (
	"os"
	"path/filepath"
	"runtime"
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

var spamSticker = struct {
	once sync.Once
	data []byte
	err  error
}{}

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

func loadSpamSticker() ([]byte, error) {
	spamSticker.once.Do(func() {
		path := spamStickerPath()
		spamSticker.data, spamSticker.err = os.ReadFile(path)
	})
	return spamSticker.data, spamSticker.err
}

func spamStickerPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("internal", "static", "spam.webp")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "static", "spam.webp"))
}
