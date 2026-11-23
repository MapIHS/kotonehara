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
	"time"
)

// Shared across clients so simultaneous commands cannot flood the backend.
var removeBGRequests = &removeBGLimiter{slot: make(chan struct{}, 1), delay: 2 * time.Second}

type removeBGLimiter struct {
	slot  chan struct{}
	delay time.Duration
	next  time.Time // accessed only while holding slot
}

func (l *removeBGLimiter) acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case l.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if wait := time.Until(l.next); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			<-l.slot
			return nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		<-l.slot
		return nil, err
	}
	return func() { l.next = time.Now().Add(l.delay); <-l.slot }, nil
}

type RemoveBGResult struct {
	Message          string `json:"message"`
	OriginalFilename string `json:"original_filename"`
	ResultFilename   string `json:"result_filename"`
	ResultURL        string `json:"result_url"`
	ContentType      string `json:"content_type"`
	Quality          string `json:"quality"`
}

func (c *Client) RemoveBG(ctx context.Context, imageData []byte, filename, quality string) (*RemoveBGResult, error) {
	release, err := removeBGRequests.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
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
