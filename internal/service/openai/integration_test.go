//go:build integration

package openai

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/subosito/gotenv"
)

// TestConfiguredProvidersIntegration verifies every enabled configured
// provider-model combination with a minimal real request. It is intentionally
// excluded from normal tests because it uses API credentials and quota.
func TestConfiguredProvidersIntegration(t *testing.T) {
	loadLocalEnv(t)
	cfg := config.Load()
	if cfg.OpenAIProvidersError != "" {
		t.Fatal(cfg.OpenAIProvidersError)
	}

	tested := 0
	for _, provider := range cfg.OpenAIProviders {
		if !provider.Enabled {
			continue
		}
		for _, model := range provider.Models {
			tested++
			t.Run(provider.Name+"/"+model, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), cfg.OpenAITimeout)
				defer cancel()
				answer, err := New(provider.BaseURL, provider.APIKey, model, cfg.OpenAITimeout).ChatCompletion(ctx, []Message{{Role: "user", Content: "Reply with exactly: OK"}})
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("provider responded (%d characters)", len(answer))
			})
		}
	}
	if tested == 0 {
		t.Fatal("tidak ada provider AI aktif di OPENAI_PROVIDERS")
	}
}

func loadLocalEnv(t *testing.T) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		path := filepath.Join(dir, ".env")
		if _, err := os.Stat(path); err == nil {
			if err := gotenv.Load(path); err != nil {
				t.Fatal(err)
			}
			if providersFile := os.Getenv("OPENAI_PROVIDERS_FILE"); providersFile != "" && !filepath.IsAbs(providersFile) {
				t.Setenv("OPENAI_PROVIDERS_FILE", filepath.Join(dir, providersFile))
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("file .env tidak ditemukan")
		}
		dir = parent
	}
}
