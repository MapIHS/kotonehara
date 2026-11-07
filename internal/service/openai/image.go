package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
)

var imageGenerationTool = toolDefinition{
	Type:     "function",
	Function: json.RawMessage(`{"name":"generate_image","description":"Generate an image requested by the user. Describe the desired image in prompt. The bot sends the generated image directly.","parameters":{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"],"additionalProperties":false}}`),
}

func (c *Client) generateImage(ctx context.Context, model, arguments string) (Reply, error) {
	var args struct {
		Prompt string `json:"prompt"`
	}
	if len(arguments) > 12000 || json.Unmarshal([]byte(arguments), &args) != nil || strings.TrimSpace(args.Prompt) == "" || len(args.Prompt) > 8000 {
		return Reply{}, fmt.Errorf("prompt gambar tidak valid")
	}
	payload, _ := json.Marshal(map[string]any{"model": model, "prompt": args.Prompt, "n": 1})
	endpoint := strings.TrimSuffix(chatCompletionsEndpoint(c.BaseURL), "/chat/completions") + "/images/generations?response_format=binary"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return Reply{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	client := http.DefaultClient
	if c.HTTP != nil {
		client = c.HTTP
	}
	transport := *client
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := transport.Do(req)
	if err != nil {
		return Reply{}, fmt.Errorf("gagal membuat gambar")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024+1))
	if err != nil || len(body) > 16*1024*1024 {
		return Reply{}, fmt.Errorf("respons gambar terlalu besar atau gagal dibaca")
	}
	if resp.StatusCode != http.StatusOK {
		return Reply{}, fmt.Errorf("provider gambar mengembalikan HTTP %d", resp.StatusCode)
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return Reply{}, fmt.Errorf("provider tidak mengembalikan PNG/JPEG yang valid")
	}
	mime := "image/png"
	if format == "jpeg" {
		mime = "image/jpeg"
	}
	return Reply{Images: []string{"data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(body)}}, nil
}
