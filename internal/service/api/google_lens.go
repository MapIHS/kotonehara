package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxGoogleLensImageBytes = 10 << 20

type GoogleLensMatch struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
}

type GoogleLensResult struct {
	Mode          string            `json:"mode"`
	LensPage      string            `json:"lensPage"`
	AIResponse    string            `json:"aiResponse"`
	VisualMatches []GoogleLensMatch `json:"visualMatches"`
}

// GoogleLens sends an image directly to the authenticated Lens worker.
func (c *Client) GoogleLens(ctx context.Context, workerURL, token, accessClientID, accessClientSecret, mode string, image []byte) (*GoogleLensResult, error) {
	if len(image) == 0 || len(image) > maxGoogleLensImageBytes {
		return nil, fmt.Errorf("gambar harus berukuran antara 1 byte dan 10 MiB")
	}
	if mode != "visual" && mode != "ai" {
		return nil, fmt.Errorf("mode Google Lens tidak valid")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("GOOGLE_LENS_WORKER_TOKEN belum diatur")
	}
	contentType := http.DetectContentType(image)
	switch contentType {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return nil, fmt.Errorf("Google Lens hanya mendukung JPEG, PNG, atau WebP")
	}

	u, err := url.Parse(strings.TrimSpace(workerURL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return nil, fmt.Errorf("GOOGLE_LENS_WORKER_URL tidak valid")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/google-lens"
	query := u.Query()
	query.Set("mode", mode)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(image))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("Content-Type", contentType)
	if accessClientID != "" && accessClientSecret != "" {
		req.Header.Set("CF-Access-Client-Id", accessClientID)
		req.Header.Set("CF-Access-Client-Secret", accessClientSecret)
	}
	client := &http.Client{Timeout: 100 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 2<<20 {
		return nil, fmt.Errorf("respons Google Lens melebihi batas")
	}
	var payload struct {
		Status  string            `json:"status"`
		Data    *GoogleLensResult `json:"data"`
		Detail  string            `json:"detail"`
		Message string            `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("respons Google Lens tidak valid")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(payload.Detail)
		if message == "" {
			message = strings.TrimSpace(payload.Message)
		}
		if message == "" {
			message = fmt.Sprintf("worker HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("Google Lens: %s", message)
	}
	if payload.Status != "success" || payload.Data == nil || payload.Data.Mode != mode {
		return nil, fmt.Errorf("respons Google Lens tidak lengkap")
	}
	return payload.Data, nil
}
