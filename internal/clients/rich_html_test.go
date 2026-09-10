package clients

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestRichHTMLWirePayload(t *testing.T) {
	payload := "  <p title=\"a & b\">Halo 日本語 👋</p>\n<script>const x = '</script>';</script>  "
	msg, err := BuildRichHTML(payload)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded waE2E.Message
	if err := proto.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	rich := decoded.GetBotForwardedMessage().GetMessage().GetRichResponseMessage()
	if rich.GetMessageType() != 1 || !rich.GetContextInfo().GetIsForwarded() || rich.GetContextInfo().GetForwardOrigin() != 4 || rich.GetContextInfo().GetForwardingScore() != 1 {
		t.Fatal("incorrect rich response type or forwarded context")
	}
	var data map[string]any
	if err := json.Unmarshal(rich.GetUnifiedResponse().GetData(), &data); err != nil {
		t.Fatalf("data must be raw JSON bytes: %v", err)
	}
	id, err := uuid.Parse(data["response_id"].(string))
	if err != nil || id.Version() != 4 {
		t.Fatalf("invalid response UUID: %v %v", id, err)
	}
	if decoded.GetMessageContextInfo().GetBotMetadata().GetBotResponseID() != id.String() {
		t.Fatal("bot metadata and unified response IDs must match")
	}
	expected := map[string]any{
		"__typename": "GenAIUnifiedResponse", "response_id": id.String(),
		"sections": []any{map[string]any{
			"__typename": "GenAIUnifiedResponseSection",
			"view_model": map[string]any{
				"__typename": "GenAISingleLayoutViewModel",
				"primitive": map[string]any{
					"__typename": "GenAIaeacdsnwHtmlPrimitive",
					"payload":    payload, "trusted_sources": []any{},
				},
			},
		}},
	}
	if !reflect.DeepEqual(data, expected) {
		t.Fatalf("wire payload changed: %#v", data)
	}
	if strings.Contains(string(rich.GetUnifiedResponse().GetData()), `\u003c`) {
		t.Fatal("HTML delimiters should not inflate the JSON payload")
	}
	// Base64 belongs to the protobuf JSON representation used by JS fromObject,
	// while the binary wire field must contain the original UTF-8 JSON bytes.
	jsonWire, err := protojson.Marshal(rich.GetUnifiedResponse())
	if err != nil {
		t.Fatal(err)
	}
	var jsonData struct{ Data string }
	if err := json.Unmarshal(jsonWire, &jsonData); err != nil {
		t.Fatal(err)
	}
	if jsonData.Data != base64.StdEncoding.EncodeToString(rich.GetUnifiedResponse().GetData()) {
		t.Fatal("unexpected protobuf JSON bytes encoding")
	}
	second, err := BuildRichHTML(payload)
	if err != nil {
		t.Fatal(err)
	}
	if string(second.GetBotForwardedMessage().GetMessage().GetRichResponseMessage().GetUnifiedResponse().GetData()) == string(rich.GetUnifiedResponse().GetData()) {
		t.Fatal("separate messages must have distinct response IDs")
	}
}

func TestRichHTMLPayloadValidation(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		valid         bool
	}{
		{"empty", "", false},
		{"whitespace", " \n\t", false},
		{"at limit", strings.Repeat("a", 64<<10), true},
		{"over limit", strings.Repeat("a", (64<<10)+1), false},
		{"multibyte over limit", strings.Repeat("é", (32<<10)+1), false},
		{"invalid UTF-8", "<p>\xff</p>", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := BuildRichHTML(tc.payload)
			if (err == nil) != tc.valid || (msg != nil) != tc.valid {
				t.Fatalf("valid=%t message=%t error=%v", tc.valid, msg != nil, err)
			}
		})
	}
}

func TestRichHTMLSources(t *testing.T) {
	sources := []string{" api.example.com ", "", "api.example.com", "youtu.be", " \t"}
	original := append([]string(nil), sources...)
	msg, err := BuildRichHTML("<p>player</p>", sources...)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(msg.GetBotForwardedMessage().GetMessage().GetRichResponseMessage().GetUnifiedResponse().GetData(), &data); err != nil {
		t.Fatal(err)
	}
	primitive := data["sections"].([]any)[0].(map[string]any)["view_model"].(map[string]any)["primitive"].(map[string]any)
	if !reflect.DeepEqual(primitive["trusted_sources"], []any{"api.example.com", "youtu.be"}) {
		t.Fatalf("unexpected sources: %#v", primitive["trusted_sources"])
	}
	if !reflect.DeepEqual(sources, original) {
		t.Fatal("builder mutated the caller's source list")
	}
	if msg, err := BuildRichHTML("<p>player</p>", "\xff"); err == nil || msg != nil {
		t.Fatal("invalid UTF-8 source must not be silently replaced")
	}
}
