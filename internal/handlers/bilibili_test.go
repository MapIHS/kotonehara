package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	bilimedia "github.com/MapIHS/kotonehara/internal/media/bilibili"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
	"go.mau.fi/whatsmeow"
)

func TestBilibiliQuery(t *testing.T) {
	for _, query := range []string{"", "https://evil.test/video/BV1Ak8Q6hECg/", "https://bilibili.com.evil.test/a", "https://user:pass@b23.tv/test", "https://b23.tv:8080/test", "https://b23.tv", "https://www.bilibili.com/bangumi/play/ep1", "https://b23.tv/test bad", "https://b23.tv/test 720p extra"} {
		if _, _, err := parseBilibiliQuery(query); err == nil {
			t.Fatalf("invalid query accepted: %s", query)
		}
	}
	for _, tc := range []struct{ query, quality string }{
		{"https://www.bilibili.com/video/BV1Ak8Q6hECg/?p=2", "1080p"},
		{"https://b23.tv/test 720", "720p"},
		{"https://bili2233.cn/test 4K", "4k"},
	} {
		url, quality, err := parseBilibiliQuery(tc.query)
		if err != nil || quality != tc.quality || url == "" {
			t.Fatalf("parse: %s %s %v", url, quality, err)
		}
	}
	for _, name := range []string{"bilibili", "bili", "bilidl"} {
		if !commands.CanHandle("."+name+" https://b23.tv/test", config.Config{Prefix: "."}) {
			t.Fatalf("command missing: %s", name)
		}
	}
}

func TestBilibiliHandler(t *testing.T) {
	for _, stage := range []string{"success", "invalid", "fetch", "prepare", "send", "missing audio"} {
		t.Run(stage, func(t *testing.T) {
			var replies []string
			m := &message.Message{Query: "https://www.bilibili.com/video/BV1Ak8Q6hECg/ 1080p"}
			if stage == "invalid" {
				m.Query = "invalid"
			}
			m.Reply = func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
				replies = append(replies, text)
				return whatsmeow.SendResponse{}, nil
			}
			fetched, prepared, sent := 0, 0, 0
			handleBilibili(context.Background(), m,
				func(ctx context.Context, url, quality string) (*api.BilibiliResult, error) {
					fetched++
					if _, ok := ctx.Deadline(); !ok || quality != "1080p" {
						t.Error("missing timeout or wrong quality")
					}
					if stage == "fetch" {
						return nil, errors.New("upstream")
					}
					result := &api.BilibiliResult{Title: "知更鸟", Page: 1, Format: "dash", Media: []api.BilibiliMedia{
						{Type: "video", Quality: 64, Codecs: "avc1", URL: "https://cdn.bilivideo.com/video"},
						{Type: "audio", Codecs: "mp4a", URL: "https://cdn.bilivideo.com/audio"},
					}}
					if stage == "missing audio" {
						result.Media = result.Media[:1]
					}
					return result, nil
				},
				func(ctx context.Context, selection bilimedia.Selection, headers map[string]string) ([]byte, error) {
					prepared++
					if selection.Audio == nil {
						t.Fatal("silent DASH passed to prepare")
					}
					if stage == "prepare" {
						return nil, errors.New("merge failed")
					}
					return []byte("merged video and audio"), nil
				},
				func(ctx context.Context, data []byte, caption string) error {
					sent++
					if string(data) != "merged video and audio" || !strings.Contains(caption, "720p") || !strings.Contains(caption, "知更鸟") {
						t.Fatal("wrong video/caption")
					}
					if stage == "send" {
						return errors.New("offline")
					}
					return nil
				})
			if stage == "invalid" && fetched != 0 {
				t.Fatal("invalid URL fetched")
			}
			if (stage == "fetch" || stage == "missing audio") && prepared != 0 {
				t.Fatal("invalid result prepared")
			}
			if stage != "success" && stage != "send" && sent != 0 {
				t.Fatal("sent video after failure")
			}
			if stage == "success" && (sent != 1 || !strings.Contains(strings.Join(replies, " "), "memakai 720p")) {
				t.Fatal("missing send/fallback notice")
			}
			if stage == "send" && !strings.Contains(replies[len(replies)-1], "offline") {
				t.Fatal("send failure hidden")
			}
		})
	}
}
