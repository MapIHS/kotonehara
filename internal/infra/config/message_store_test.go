package config

import (
	"testing"
	"time"
)

func TestMessageStoreConfig(t *testing.T) {
	for _, key := range []string{"MESSAGE_STORE_ENABLED", "MESSAGE_STORE_RETENTION", "MESSAGE_STORE_CACHE_BYTES", "MESSAGE_STORE_QUEUE_BYTES"} {
		t.Setenv(key, "")
	}
	cfg := Load()
	if !cfg.MessageStoreEnabled || cfg.MessageStoreRetention != 24*time.Hour || cfg.MessageStoreCacheBytes != 4<<20 || cfg.MessageStoreQueueBytes != 4<<20 {
		t.Fatal("unexpected defaults")
	}
	t.Setenv("MESSAGE_STORE_ENABLED", "false")
	t.Setenv("MESSAGE_STORE_RETENTION", "6h")
	t.Setenv("MESSAGE_STORE_CACHE_BYTES", "1024")
	t.Setenv("MESSAGE_STORE_QUEUE_BYTES", "-1")
	cfg = Load()
	if cfg.MessageStoreEnabled || cfg.MessageStoreRetention != 6*time.Hour || cfg.MessageStoreCacheBytes != 1024 || cfg.MessageStoreQueueBytes != 4<<20 {
		t.Fatal("config overrides/fallbacks failed")
	}
}
