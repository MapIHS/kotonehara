package pkg

import (
	"context"
	"fmt"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/openai"
)

const maxAIReplySize = 65536

func aiCmd(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	_ = client

	if cfg.OpenAIAPIKey == "" {
		m.Reply(ctx, "OPENAI_API_KEY belum diatur di env, yaa.")
		return
	}
	if cfg.OpenAIModel == "" {
		m.Reply(ctx, "OPENAI_MODEL belum diatur di env, yaa.")
		return
	}

	prompt := strings.TrimSpace(m.Query)
	quoted := strings.TrimSpace(aiQuotedText(m))
	if quoted != "" {
		prompt = fmt.Sprintf("Konteks pesan yang direply:\n%s\n\nPertanyaan/perintah:\n%s", quoted, prompt)
	}

	ai := openai.New(cfg.OpenAIBaseURL, cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAITimeout)
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
