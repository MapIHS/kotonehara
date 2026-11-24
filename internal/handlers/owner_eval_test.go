package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
)

func TestOwnerEvalYaegiExpression(t *testing.T) {
	replies := []string{}
	m := &message.Message{
		Query: "1 + 1",
		Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			replies = append(replies, text)
			return whatsmeow.SendResponse{}, nil
		},
	}

	ownerEval(context.Background(), nil, m, config.Config{})

	if len(replies) == 0 {
		t.Fatal("expected reply, got none")
	}
	if replies[0] != "2" {
		t.Fatalf("expected reply '2', got: %q", replies[0])
	}
}

func TestOwnerEvalYaegiPrettyJSONString(t *testing.T) {
	replies := []string{}
	m := &message.Message{
		Query: `{"name":"kotone","status":"online"}`,
		Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			replies = append(replies, text)
			return whatsmeow.SendResponse{}, nil
		},
	}

	ownerEval(context.Background(), nil, m, config.Config{})

	if len(replies) == 0 {
		t.Fatal("expected reply, got none")
	}
	expected := "```\n{\n  \"name\": \"kotone\",\n  \"status\": \"online\"\n}\n```"
	if replies[0] != expected {
		t.Fatalf("expected pretty JSON:\n%s\ngot:\n%s", expected, replies[0])
	}
}

func TestOwnerEvalYaegiPrettyMap(t *testing.T) {
	replies := []string{}
	m := &message.Message{
		Query: `map[string]any{"status": "ok", "code": 200}`,
		Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			replies = append(replies, text)
			return whatsmeow.SendResponse{}, nil
		},
	}

	ownerEval(context.Background(), nil, m, config.Config{})

	if len(replies) == 0 {
		t.Fatal("expected reply, got none")
	}
	if !strings.Contains(replies[0], "code: 200") || !strings.Contains(replies[0], "status: \"ok\"") {
		t.Fatalf("expected formatted map with 2 spaces indent, got:\n%s", replies[0])
	}
	if !strings.HasPrefix(replies[0], "```") {
		t.Fatalf("expected code block wrapper ```, got:\n%s", replies[0])
	}
}

func TestOwnerEvalYaegiPrettyStdoutJSON(t *testing.T) {
	replies := []string{}
	m := &message.Message{
		Query: `fmt.Println("{\"user\":\"dika\",\"active\":true}")`,
		Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			replies = append(replies, text)
			return whatsmeow.SendResponse{}, nil
		},
	}

	ownerEval(context.Background(), nil, m, config.Config{})

	if len(replies) == 0 {
		t.Fatal("expected reply, got none")
	}
	if !strings.Contains(replies[0], "  \"user\": \"dika\"") {
		t.Fatalf("expected stdout json to be pretty-printed with indent, got:\n%s", replies[0])
	}
}
