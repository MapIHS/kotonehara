package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
)

type ocrResponse struct {
	Text string `json:"text"`
}

func (c *Client) ExtractOCR(ctx context.Context, imageBytes []byte) (string, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return "", err
	}
	u.Path = "/api/ocr"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(imageBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "image/jpeg")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("OCR api http %d", resp.StatusCode)
	}

	var out APIResponse[ocrResponse]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return "", err
	}
	return out.Data.Text, nil
}
