package pkg

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/openai"
)

const maxAIReplySize = 65536

var aiRouters sync.Map // map[string]*openai.Router; one router per loaded configuration

func aiCmd(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	_ = client

	if cfg.OpenAIProvidersError != "" {
		m.Reply(ctx, "Konfigurasi rotasi AI tidak valid: "+cfg.OpenAIProvidersError)
		return
	}
	if len(cfg.OpenAIProviders) == 0 && cfg.OpenAIAPIKey == "" {
		m.Reply(ctx, "OPENAI_API_KEY belum diatur di env, yaa.")
		return
	}
	if len(cfg.OpenAIProviders) == 0 && cfg.OpenAIModel == "" {
		m.Reply(ctx, "OPENAI_MODEL belum diatur di env, yaa.")
		return
	}

	prompt := strings.TrimSpace(m.Query)
	quoted := strings.TrimSpace(aiQuotedText(m))
	if quoted != "" {
		prompt = fmt.Sprintf("Konteks pesan yang direply:\n%s\n\nPertanyaan/perintah:\n%s", quoted, prompt)
	}

	providers := make([]openai.Provider, 0, len(cfg.OpenAIProviders))
	for _, provider := range cfg.OpenAIProviders {
		if !provider.Enabled {
			continue
		}
		for _, model := range provider.Models {
			providers = append(providers, openai.Provider{Name: provider.Name, BaseURL: provider.BaseURL, APIKey: provider.APIKey, Model: model})
		}
	}
	if len(providers) == 0 {
		providers = append(providers, openai.Provider{BaseURL: cfg.OpenAIBaseURL, APIKey: cfg.OpenAIAPIKey, Model: cfg.OpenAIModel})
	}

	ai := aiRouterFor(providers, cfg.OpenAITimeout)
	answer, err := ai.ChatCompletion(ctx, []openai.Message{
		{Role: "system", Content: cfg.OpenAISystemPrompt},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		m.Reply(ctx, "AI gagal menjawab: "+err.Error())
		return
	}

	if len(answer) > maxAIReplySize {
		answer = answer[:maxAIReplySize-5] + "\n\n..."
	}
	m.Reply(ctx, answer)
}

func aiRouterFor(providers []openai.Provider, timeout time.Duration) *openai.Router {
	var key strings.Builder
	for _, provider := range providers {
		// Delimiters make the key unambiguous while keeping API keys out of logs.
		fmt.Fprintf(&key, "%q|%q|%q|%q;", provider.Name, provider.BaseURL, provider.APIKey, provider.Model)
	}
	key.WriteString(timeout.String())
	if router, ok := aiRouters.Load(key.String()); ok {
		return router.(*openai.Router)
	}
	router := openai.NewRouter(providers, timeout)
	actual, _ := aiRouters.LoadOrStore(key.String(), router)
	return actual.(*openai.Router)
}

func aiQuotedText(m *message.Message) string {
	if m == nil || m.QuotedMsg == nil {
		return ""
	}
	if s := m.QuotedMsg.GetExtendedTextMessage().GetText(); s != "" {
		return s
	}
	if s := m.QuotedMsg.GetConversation(); s != "" {
		return s
	}
	if s := m.QuotedMsg.GetImageMessage().GetCaption(); s != "" {
		return s
	}
	if s := m.QuotedMsg.GetVideoMessage().GetCaption(); s != "" {
		return s
	}
	return ""
}

func init() {
	commands.Register(&commands.Command{
		Name:        "ai",
		As:          []string{"ask", "gpt"},
		Tags:        "ai",
		Description: "Tanya AI via OpenAI-compatible API",
		IsPrefix:    true,
		IsQuery:     true,
		ShowWait:    true,
		Exec:        aiCmd,
	})
}
