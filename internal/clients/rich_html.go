package clients

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/proto/waAICommon"
	"go.mau.fi/whatsmeow/proto/waAICommonDeprecated"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

const maxRichHTMLPayloadBytes = 64 << 10

// Observed in current Baileys HTML builders, not a public WhatsApp API contract.
const richHTMLPrimitive = "GenAIaeacdsnwHtmlPrimitive"

// BuildRichHTML encodes the experimental unified-response payload. Rendering is
// handled by the receiving WhatsApp client and is not guaranteed by the schema.
// trustedSources supplies attribution labels; it does not grant network access.
func BuildRichHTML(payload string, trustedSources ...string) (*waE2E.Message, error) {
	if strings.TrimSpace(payload) == "" {
		return nil, errors.New("Payload HTML tidak boleh kosong.")
	}
	if len(payload) > maxRichHTMLPayloadBytes {
		return nil, errors.New("Payload HTML maksimal 64 KiB.")
	}
	if !utf8.ValidString(payload) {
		return nil, errors.New("Payload HTML harus berupa UTF-8 yang valid.")
	}
	// Encode an empty JSON array when no attribution was supplied. Do not add an
	// unrelated source, and do not mutate the caller's slice while normalizing.
	sources := make([]string, 0, len(trustedSources))
	seen := make(map[string]bool, len(trustedSources))
	for _, source := range trustedSources {
		if !utf8.ValidString(source) {
			return nil, errors.New("Sumber HTML harus berupa UTF-8 yang valid.")
		}
		source = strings.TrimSpace(source)
		if source != "" && !seen[source] {
			sources = append(sources, source)
			seen[source] = true
		}
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	// This JSON travels inside protobuf bytes, not an HTML script element. Avoid
	// expanding every HTML delimiter into a six-byte JSON Unicode escape.
	encoder.SetEscapeHTML(false)
	err = encoder.Encode(map[string]any{
		"__typename":  "GenAIUnifiedResponse",
		"response_id": id.String(),
		"sections": []any{
			map[string]any{
				"__typename": "GenAIUnifiedResponseSection",
				"view_model": map[string]any{
					"__typename": "GenAISingleLayoutViewModel",
					"primitive": map[string]any{
						"__typename":      richHTMLPrimitive,
						"payload":         payload,
						"trusted_sources": sources,
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{
			BotMetadata: &waAICommon.BotMetadata{BotResponseID: proto.String(id.String())},
		},
		BotForwardedMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				RichResponseMessage: &waE2E.AIRichResponseMessage{
					MessageType: waAICommonDeprecated.AIRichResponseMessageType_AI_RICH_RESPONSE_TYPE_STANDARD.Enum(),
					// Protobuf expects JSON bytes here, not the bytes of a base64 string.
					UnifiedResponse: &waAICommon.AIRichResponseUnifiedResponse{Data: bytes.TrimSuffix(data.Bytes(), []byte("\n"))},
					ContextInfo: &waE2E.ContextInfo{
						ForwardingScore: proto.Uint32(1),
						IsForwarded:     proto.Bool(true),
						ForwardOrigin:   waE2E.ContextInfo_META_AI.Enum(),
					},
				},
			},
		},
	}, nil
}
