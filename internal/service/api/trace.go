package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type TraceMoeResponse struct {
	Error  string           `json:"error"`
	Result []TraceMoeResult `json:"result"`
}

type TraceMoeResult struct {
	Anilist struct {
		ID    int `json:"id"`
		IDMal int `json:"idMal"`
		Title struct {
			Native  string `json:"native"`
			Romaji  string `json:"romaji"`
			English string `json:"english"`
		} `json:"title"`
		IsAdult bool `json:"isAdult"`
	} `json:"anilist"`
	Filename   string      `json:"filename"`
	Episode    interface{} `json:"episode"`
	From       float64     `json:"from"`
	To         float64     `json:"to"`
	Similarity float64     `json:"similarity"`
	Video      string      `json:"video"`
}

type AnilistInfo struct {
	ID          int    `json:"id"`
	Title       struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
		Native  string `json:"native"`
	} `json:"title"`
	Description string `json:"description"`
	BannerImage string `json:"bannerImage"`
	CoverImage  struct {
		ExtraLarge string `json:"extraLarge"`
		Large      string `json:"large"`
		Medium     string `json:"medium"`
	} `json:"coverImage"`
	Genres     []string `json:"genres"`
	Status     string   `json:"status"`
	Season     string   `json:"season"`
	SeasonYear int      `json:"seasonYear"`
	MeanScore  int      `json:"meanScore"`
	Episodes   int      `json:"episodes"`
	Duration   int      `json:"duration"`
}

type anilistGraphQLResponse struct {
	Data struct {
		Media AnilistInfo `json:"Media"`
	} `json:"data"`
}

// SearchTraceMoe calls the trace.moe API with the provided image bytes.
func (c *Client) SearchTraceMoe(ctx context.Context, imageBytes []byte) (*TraceMoeResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.trace.moe/search?anilistInfo", bytes.NewReader(imageBytes))
	if err != nil {
		return nil, fmt.Errorf("trace.moe create request: %w", err)
	}

	req.Header.Set("Content-Type", "image/jpeg")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trace.moe request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("trace.moe error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var traceRes TraceMoeResponse
	if err := json.NewDecoder(resp.Body).Decode(&traceRes); err != nil {
		return nil, fmt.Errorf("trace.moe decode response: %w", err)
	}

	if traceRes.Error != "" {
		return nil, fmt.Errorf("trace.moe returned error: %s", traceRes.Error)
	}

	return &traceRes, nil
}

// FetchAnilistInfo queries the AniList GraphQL API for anime metadata.
func (c *Client) FetchAnilistInfo(ctx context.Context, id int) (*AnilistInfo, error) {
	query := `query ($id: Int) { Media (id: $id, type: ANIME) { id title { romaji english native } description bannerImage coverImage { extraLarge large medium } genres status season seasonYear meanScore episodes duration } }`
	payload := map[string]interface{}{
		"query": query,
		"variables": map[string]int{
			"id": id,
		},
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://graphql.anilist.co", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anilist api error status: %d", resp.StatusCode)
	}

	var res anilistGraphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return &res.Data.Media, nil
}
