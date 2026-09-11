package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type PlayerSession struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Artist    string    `json:"artist"`
	Duration  float64   `json:"duration"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mimeType"`
	ExpiresAt time.Time `json:"expiresAt"`
	WSURL     string    `json:"wsUrl"`
	PlayerURL string    `json:"playerUrl"`
	HTML      string    `json:"html"`
}

func (c *Client) CreatePlayer(ctx context.Context, query string) (*PlayerSession, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("BASEAPI_URL Hararest belum valid")
	}
	body, err := json.Marshal(map[string]string{"query": query, "baseUrl": u.Scheme + "://" + u.Host})
	if err != nil {
		return nil, err
	}
	u.Path, u.RawPath, u.RawQuery, u.Fragment = "/api/player/sessions", "", "", ""
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, readAPIError("player", resp)
	}
	var result APIResponse[PlayerSession]
	if err := decodeAPIResponse(resp, &result); err != nil {
		return nil, err
	}
	session := &result.Data
	ws, wsErr := url.Parse(session.WSURL)
	page, pageErr := url.Parse(session.PlayerURL)
	if wsErr != nil || pageErr != nil || ws.Host == "" || page.Host != ws.Host ||
		(ws.Scheme != "ws" && ws.Scheme != "wss") || (page.Scheme != "http" && page.Scheme != "https") ||
		ws.User != nil || page.User != nil || strings.TrimSpace(session.HTML) == "" ||
		session.ExpiresAt.IsZero() || session.Size <= 0 || session.Size > 24<<20 {
		return nil, fmt.Errorf("Respons sesi player Hararest tidak valid")
	}
	return session, nil
}
