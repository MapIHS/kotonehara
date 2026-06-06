package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/service/httpclient"
)

const maxResponseBytes = 8 * 1024 * 1024

type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error json.RawMessage `json:"error,omitempty"`
}

func New(baseURL, apiKey, model string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}

	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  strings.TrimSpace(apiKey),
		Model:   strings.TrimSpace(model),
		HTTP:    httpclient.New("", timeout).HTTP,
	}
}

func (c *Client) ChatCompletion(ctx context.Context, messages []Message) (string, error) {
	if c == nil {
		return "", fmt.Errorf("openai client nil")
	}
	if c.APIKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY belum diatur")
	}
	if c.BaseURL == "" {
		return "", fmt.Errorf("OPENAI_BASE_URL belum diatur")
	}
	if c.Model == "" {
		return "", fmt.Errorf("OPENAI_MODEL belum diatur")
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("pesan kosong")
	}

	endpoint := chatCompletionsEndpoint(c.BaseURL)
	payload, err := json.Marshal(chatCompletionRequest{
		Model:    c.Model,
		Messages: messages,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxResponseBytes {
		return "", fmt.Errorf("respons AI terlalu besar")
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", openAIHTTPStatusError(resp.StatusCode, body)
	}

	var out chatCompletionResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		if len(out.Error) > 0 {
			return "", fmt.Errorf("openai api: %s", decodeOpenAIError(out.Error))
		}
		return "", fmt.Errorf("openai api tidak mengembalikan pilihan jawaban")
	}

	content := strings.TrimSpace(decodeOpenAIContent(out.Choices[0].Message.Content))
	if content == "" {
		return "", fmt.Errorf("openai api mengembalikan jawaban kosong")
	}
	return content, nil
}

func chatCompletionsEndpoint(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

func decodeOpenAIContent(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, part := range parts {
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(part.Text)
		}
		return b.String()
	}

	return strings.TrimSpace(string(raw))
}

func openAIHTTPStatusError(statusCode int, body []byte) error {
	message := ""

	var payload struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		message = strings.TrimSpace(payload.Message)
		if message == "" && len(payload.Error) > 0 {
			message = decodeOpenAIError(payload.Error)
		}
	}

	if message == "" {
		message = strings.Join(strings.Fields(string(body)), " ")
		if len(message) > 220 {
			message = message[:220]
		}
	}

	if message == "" {
		return fmt.Errorf("openai api http %d", statusCode)
	}
	return fmt.Errorf("openai api http %d: %s", statusCode, message)
}

func decodeOpenAIError(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}

	var obj struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		parts := make([]string, 0, 3)
		if obj.Message != "" {
			parts = append(parts, obj.Message)
		}
		if obj.Type != "" {
			parts = append(parts, "type="+obj.Type)
		}
		if obj.Code != "" {
			parts = append(parts, "code="+obj.Code)
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}

	return strings.TrimSpace(string(raw))
}
