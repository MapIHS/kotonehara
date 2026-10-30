package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type traceRoundTrip func(*http.Request) (*http.Response, error)

func (f traceRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTraceRespectsGroupNSFWBeforeSendingResults(t *testing.T) {
	for _, tc := range []struct {
		name                                                             string
		group, enabled, traceAdult, anilistAdult, metadataFails, blocked bool
	}{
		{name: "adult trace blocked in disabled group", group: true, traceAdult: true, blocked: true},
		{name: "adult AniList metadata blocked in disabled group", group: true, anilistAdult: true, blocked: true},
		{name: "ordinary result allowed in disabled group", group: true},
		{name: "adult trace allowed in enabled group", group: true, enabled: true, traceAdult: true},
		{name: "adult AniList metadata allowed in enabled group", group: true, enabled: true, anilistAdult: true},
		{name: "trace adult flag survives conflicting safe metadata", group: true, enabled: true, traceAdult: true},
		{name: "adult fallback allowed in enabled group", group: true, enabled: true, traceAdult: true, metadataFails: true},
		{name: "ordinary fallback allowed in disabled group", group: true, metadataFails: true},
		{name: "adult private result allowed", traceAdult: true},
		{name: "adult private fallback allowed", traceAdult: true, metadataFails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			best := api.TraceMoeResult{}
			best.Anilist.ID = 123
			best.Anilist.Title.Romaji = "PRIVATE_RESULT_TITLE"
			best.Anilist.IsAdult = tc.traceAdult
			if tc.blocked {
				best.Video = "https://preview.invalid/private-video"
			}
			calls := 0
			apiClient := api.New("", time.Second)
			apiClient.HTTP.Transport = traceRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "https://graphql.anilist.co" {
					t.Fatalf("preview fetched for blocked result: %s", r.URL)
				}
				var payload struct {
					Query string `json:"query"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(payload.Query, "isAdult") {
					t.Fatal("AniList classification not requested")
				}
				if tc.metadataFails {
					return nil, errors.New("metadata unavailable")
				}
				info := api.AnilistInfo{IsAdult: tc.anilistAdult}
				info.Title.Romaji = "PRIVATE_RESULT_TITLE"
				if tc.blocked {
					info.BannerImage = "https://preview.invalid/private-cover"
				}
				body, err := json.Marshal(map[string]any{"data": map[string]any{"Media": info}})
				if err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
			})
			group := types.NewJID("120363000000000", types.GroupServer)
			replies := []string{}
			m := &message.Message{IsGroup: tc.group, From: group, Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
				replies = append(replies, text)
				return whatsmeow.SendResponse{}, nil
			}}
			lookups := 0
			enabled := func(jid string) bool {
				lookups++
				if !tc.group {
					t.Fatal("private chat must not query group settings")
				}
				if jid != group.String() {
					t.Fatalf("wrong group setting key: %s", jid)
				}
				return tc.enabled
			}
			// A nil WhatsApp client makes any accidental image/video send fail this test.
			replyTraceResult(context.Background(), nil, m, apiClient, best, enabled)
			if len(replies) != 1 {
				t.Fatalf("expected one result, got %v", replies)
			}
			text := replies[0]
			if tc.blocked {
				if !strings.Contains(text, "diblokir") || strings.Contains(text, "PRIVATE_RESULT_TITLE") || strings.Contains(text, "https://preview.invalid") {
					t.Fatalf("adult result leaked: %s", text)
				}
				expectedCalls := 1
				if tc.traceAdult {
					expectedCalls = 0
				}
				if calls != expectedCalls {
					t.Fatalf("unexpected upstream requests: %d", calls)
				}
			} else {
				if !strings.Contains(text, "PRIVATE_RESULT_TITLE") || strings.Contains(text, "diblokir") {
					t.Fatal(text)
				}
				if strings.Contains(text, "PERINGATAN") != (tc.traceAdult || tc.anilistAdult) {
					t.Fatalf("incorrect adult warning: %s", text)
				}
				if calls != 1 {
					t.Fatalf("metadata requests: %d", calls)
				}
			}
			if !tc.group && lookups != 0 {
				t.Fatal("private lookup")
			}
		})
	}
}
