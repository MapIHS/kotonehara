package quote

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const maxResponseSize = 4 << 20 // 4 MiB

// Generate sends a quote payload to the API and returns the resulting image as
// raw PNG bytes. The caller is responsible for further conversion (e.g. to a
// WebP sticker).
func (c *Client) Generate(ctx context.Context, payload Payload) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("quote: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("quote: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("quote: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("quote: read response: %w", err)
	}
	if len(body) > maxResponseSize {
		return nil, fmt.Errorf("quote: response body melebihi batas %d byte", maxResponseSize)
	}

	if resp.StatusCode != http.StatusOK {
		snippet := string(body)
		if len(snippet) > 180 {
			snippet = snippet[:180]
		}
		return nil, fmt.Errorf("quote: api http %d: %s", resp.StatusCode, snippet)
	}

	var res Response
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("quote: decode response: %w", err)
	}
	if !res.Ok {
		return nil, fmt.Errorf("quote: api mengembalikan ok=false")
	}

	imgBytes, err := base64.StdEncoding.DecodeString(res.Result.Image)
	if err != nil {
		return nil, fmt.Errorf("quote: decode base64 image: %w", err)
	}

	return imgBytes, nil
}
