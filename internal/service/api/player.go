package api

import (
	"context"
	"fmt"
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
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	u, err := url.Parse(c.BaseURL)
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("BASEAPI_URL Hararest belum valid")
	}
	job, err := c.runMediaJob(ctx, "/api/player/jobs", map[string]string{"query": query, "baseUrl": u.Scheme + "://" + u.Host}, jobPollInterval)
	if err != nil {
		return nil, err
	}
	if job.Result.Kind != "player" || job.Result.Player == nil {
		return nil, fmt.Errorf("Respons sesi player Hararest tidak valid")
	}
	session := job.Result.Player
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
