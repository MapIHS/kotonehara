package handlers

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/MapIHS/kotonehara/internal/commands"
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
	expected := "```json\n{\n  \"name\": \"kotone\",\n  \"status\": \"online\"\n}\n```"
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

func TestOwnerEvalPreservesMultilineSource(t *testing.T) {
	replies := ownerEvalTestReplies(t, &message.Message{
		Body: "> a := 2\nb := 3\na + b",
	})
	if len(replies) != 1 || replies[0] != "5" {
		t.Fatalf("multiline eval replies = %q, want [5]", replies)
	}
}

func TestOwnerEvalRejectsOversizedInput(t *testing.T) {
	replies := ownerEvalTestReplies(t, &message.Message{Query: strings.Repeat("x", ownerEvalMaxQuerySize+1)})
	if len(replies) != 1 || !strings.Contains(replies[0], "Maksimum 64 KiB") {
		t.Fatalf("oversized input replies = %q", replies)
	}
}

func TestOwnerEvalTimeout(t *testing.T) {
	replies := ownerEvalTestReplies(t, &message.Message{Query: `(func() { for {} })()`})
	if len(replies) != 1 || !strings.Contains(replies[0], "timeout (10 detik)") {
		t.Fatalf("timeout replies = %q", replies)
	}
}

func TestOwnerEvalBoundsOutputBeforeReply(t *testing.T) {
	replies := ownerEvalTestReplies(t, &message.Message{
		Query: `(func() { for i := 0; i < 10000; i++ { fmt.Print(strings.Repeat("x", 100)) } })()`,
	})
	if len(replies) != 1 || len(replies[0]) > ownerEvalMaxOutputSize+64 {
		t.Fatalf("bounded output length = %d, replies = %d", len(replies[0]), len(replies))
	}
	if !strings.Contains(replies[0], "[output dipotong]") {
		t.Fatalf("output truncation was not reported: len=%d tail=%q", len(replies[0]), replies[0][len(replies[0])-100:])
	}
}

func TestOwnerEvalOnlyRunsOneAtATime(t *testing.T) {
	ownerEvalSlot <- struct{}{}
	defer func() { <-ownerEvalSlot }()
	replies := ownerEvalTestReplies(t, &message.Message{Query: "1 + 1"})
	if len(replies) != 1 || !strings.Contains(replies[0], "Eval lain masih berjalan") {
		t.Fatalf("busy eval replies = %q", replies)
	}
}

func TestTruncateOwnerEvalReplyPreservesUTF8(t *testing.T) {
	reply := truncateOwnerEvalReply(strings.Repeat("😀", ownerEvalMaxReplySize))
	if len(reply) > ownerEvalMaxReplySize || !strings.HasSuffix(reply, "\n\n...") {
		t.Fatalf("truncated reply length/suffix = %d %q", len(reply), reply[len(reply)-5:])
	}
	if !utf8.ValidString(reply) {
		t.Fatal("truncation produced invalid UTF-8")
	}
}

func TestOwnerEvalIsOwnerOnlyInPrivateAndGroupChats(t *testing.T) {
	for _, cmd := range commands.Commands() {
		if cmd.Name != ">" {
			continue
		}
		if !cmd.IsOwner || cmd.IsPrivate || !cmd.IsQuery || cmd.IsPrefix {
			t.Fatalf("unexpected owner eval command policy: %+v", cmd)
		}
		return
	}
	t.Fatal("owner eval command is not registered")
}

func ownerEvalTestReplies(t *testing.T, m *message.Message) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var replies []string
	m.Reply = func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		replies = append(replies, text)
		return whatsmeow.SendResponse{}, nil
	}
	ownerEval(ctx, nil, m, config.Config{})
	return replies
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
