package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

type RemoveBGResult struct {
	Message          string `json:"message"`
	OriginalFilename string `json:"original_filename"`
	ResultFilename   string `json:"result_filename"`
	ResultURL        string `json:"result_url"`
	ContentType      string `json:"content_type"`
	Quality          string `json:"quality"`
}

func (c *Client) RemoveBG(ctx context.Context, imageData []byte, filename, quality string) (*RemoveBGResult, error) {
	if quality == "" {
		quality = "fast"
	}

	mime := http.DetectContentType(imageData)
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/png"
	}

	if filename == "" {
		switch mime {
		case "image/jpeg":
			filename = "image.jpg"
		case "image/webp":
			filename = "image.webp"
		default:
			filename = "image.png"
		}
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, filename))
	h.Set("Content-Type", mime)

	fw, err := w.CreatePart(h)
	if err != nil {
		return nil, fmt.Errorf("removebg: create form file: %w", err)
	}
	if _, err := fw.Write(imageData); err != nil {
		return nil, fmt.Errorf("removebg: write image data: %w", err)
	}

	if err := w.WriteField("quality", quality); err != nil {
		return nil, fmt.Errorf("removebg: write quality field: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("removebg: close multipart: %w", err)
	}

	endpoint := c.BaseURL + "/image/remove-bg"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("removebg: create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("removebg: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp, maxAPIResponseSize)
	if err != nil {
		return nil, fmt.Errorf("removebg: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiHTTPStatusError("removebg", resp.StatusCode, body)
	}

	var result RemoveBGResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("removebg: decode response: %w", err)
	}

	return &result, nil
}
