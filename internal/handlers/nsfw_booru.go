package handlers

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

type booruFetcher func(ctx context.Context, cfg config.Config, tags string, limit int) (url string, caption string, err error)

func init() {
	commands.Register(&commands.Command{
		Name:        "rule34",
		As:          []string{"r34"},
		Tags:        "nsfw",
		Description: "Cari gambar atau video di Rule34",
		IsPrefix:    true,
		IsQuery:     true,
		IsPrivate:   true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			handleBooru(ctx, client, m, cfg, rule34Fetcher)
		},
	})
	commands.Register(&commands.Command{
		Name:        "danbooru",
		As:          []string{"dan"},
		Tags:        "nsfw",
		Description: "Cari gambar di Danbooru",
		IsPrefix:    true,
		IsQuery:     true,
		Disable:     true,
		IsPrivate:   true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			handleBooru(ctx, client, m, cfg, danbooruFetcher)
		},
	})
	commands.Register(&commands.Command{
		Name:        "yandere",
		As:          []string{"yande"},
		Tags:        "nsfw",
		Description: "Cari gambar di Yandere",
		IsPrefix:    true,
		IsQuery:     true,
		IsPrivate:   true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			handleBooru(ctx, client, m, cfg, yandereFetcher)
		},
	})
}

func handleBooru(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config, fetcher booruFetcher) {
	tags := strings.Join(strings.Fields(m.Query), " ")
	mediaURL, caption, err := fetcher(ctx, cfg, tags, 10)
	if err != nil {
		_, _ = m.Reply(ctx, "❌ "+err.Error())
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	buff, err := client.FetchBytes(mediaURL)
	if err != nil {
		_, _ = m.Reply(ctx, "❌ Gagal mengunduh media: "+err.Error())
		return
	}

	if err := sendBooruMedia(opCtx, client, m, mediaURL, buff, caption); err != nil {
		_, _ = m.Reply(ctx, "❌ Gagal mengirim media: "+err.Error())
	}
}

type booruMediaKind int

const (
	booruMediaImage booruMediaKind = iota
	booruMediaGIF
	booruMediaVideo
	booruMediaDocument
)

func detectBooruMediaKind(mediaURL string, data []byte) booruMediaKind {
	mime := http.DetectContentType(data)
	ext := urlPathExt(mediaURL)

	switch {
	case mime == "image/gif" || (mime == "application/octet-stream" && ext == ".gif"):
		return booruMediaGIF
	case mime == "video/webm" || (mime == "application/octet-stream" && ext == ".webm"):
		return booruMediaDocument
	case strings.HasPrefix(mime, "video/") || (mime == "application/octet-stream" && (ext == ".mp4" || ext == ".m4v" || ext == ".mov")):
		return booruMediaVideo
	case strings.HasPrefix(mime, "image/"):
		return booruMediaImage
	default:
		return booruMediaDocument
	}
}

func urlPathExt(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err == nil {
		return strings.ToLower(path.Ext(u.Path))
	}
	return strings.ToLower(path.Ext(rawURL))
}

func sendBooruMedia(ctx context.Context, client *clients.Client, m *message.Message, mediaURL string, buff []byte, caption string) error {
	switch detectBooruMediaKind(mediaURL, buff) {
	case booruMediaGIF:
		_, err := client.SendVideo(ctx, m.From, buff, true, caption, m.ID)
		return err
	case booruMediaVideo:
		_, err := client.SendVideo(ctx, m.From, buff, false, caption, m.ID)
		return err
	case booruMediaImage:
		_, err := client.SendImage(ctx, m.From, buff, caption, m.ID)
		return err
	default:
		name := "booru_media" + urlPathExt(mediaURL)
		_, err := client.SendDocument(ctx, m.From, buff, name, caption, m.ID)
		return err
	}
}

func formatBooruCaption(tags string, score int) string {
	fields := strings.Fields(tags)
	if len(fields) > 15 {
		fields = append(fields[:15], "...")
	}
	tagText := strings.Join(fields, " ")
	if tagText == "" {
		tagText = "-"
	}
	return fmt.Sprintf("🏷️ Tags: %s\n⭐ Score: %d", tagText, score)
}

func pickRule34MediaURL(post api.Rule34Post) string {
	if u := strings.TrimSpace(post.FileURL); u != "" {
		return u
	}
	return strings.TrimSpace(post.SampleURL)
}

func selectRule34Post(posts []api.Rule34Post, randIntn func(int) int) (api.Rule34Post, string, bool) {
	var preferred, fallback []struct {
		post api.Rule34Post
		url  string
	}
	for _, post := range posts {
		u := pickRule34MediaURL(post)
		if u == "" {
			continue
		}
		entry := struct {
			post api.Rule34Post
			url  string
		}{post: post, url: u}
		if urlPathExt(u) == ".webm" {
			fallback = append(fallback, entry)
		} else {
			preferred = append(preferred, entry)
		}
	}
	pool := preferred
	if len(pool) == 0 {
		pool = fallback
	}
	if len(pool) == 0 {
		return api.Rule34Post{}, "", false
	}
	chosen := pool[randIntn(len(pool))]
	return chosen.post, chosen.url, true
}

func rule34Fetcher(ctx context.Context, cfg config.Config, tags string, limit int) (string, string, error) {
	if cfg.BASEApiURL == "" {
		return "", "", fmt.Errorf("Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
	}
	ap := api.Shared(cfg.BASEApiURL, 90*time.Second)
	posts, err := ap.Rule34(ctx, tags, limit)
	if err != nil {
		return "", "", fmt.Errorf("Gagal mengambil data dari Rule34: %v", err)
	}
	post, mediaURL, ok := selectRule34Post(posts, rand.Intn)
	if !ok {
		return "", "", fmt.Errorf("Media tidak ditemukan.")
	}
	return mediaURL, formatBooruCaption(post.Tags, post.Score), nil
}

func danbooruFetcher(ctx context.Context, cfg config.Config, tags string, limit int) (string, string, error) {
	if cfg.BASEApiURL == "" {
		return "", "", fmt.Errorf("Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
	}
	ap := api.Shared(cfg.BASEApiURL, 90*time.Second)
	posts, err := ap.Danbooru(ctx, tags, limit)
	if err != nil {
		return "", "", fmt.Errorf("Gagal mengambil data dari Danbooru: %v", err)
	}
	if len(posts) == 0 {
		return "", "", fmt.Errorf("Gambar tidak ditemukan.")
	}
	post := posts[rand.Intn(len(posts))]

	mediaURL := post.LargeURL
	if mediaURL == "" {
		mediaURL = post.FileURL
	}
	return mediaURL, formatBooruCaption(post.Tags, post.Score), nil
}

func yandereFetcher(ctx context.Context, cfg config.Config, tags string, limit int) (string, string, error) {
	ap := api.Shared("https://yande.re", 15*time.Second)
	posts, err := ap.Yandere(ctx, tags, limit)
	if err != nil {
		return "", "", fmt.Errorf("Gagal mengambil data dari Yandere: %v", err)
	}
	if len(posts) == 0 {
		return "", "", fmt.Errorf("Gambar tidak ditemukan.")
	}
	post := posts[rand.Intn(len(posts))]

	mediaURL := post.SampleURL
	if mediaURL == "" {
		mediaURL = post.FileURL
	}
	return mediaURL, formatBooruCaption(post.Tags, post.Score), nil
}
