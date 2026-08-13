package handlers

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

type booruFetcher func(ctx context.Context, tags string, limit int) (url string, caption string, err error)

func init() {
	commands.Register(&commands.Command{
		Name:        "r34",
		As:          []string{"rule34"},
		Tags:        "nsfw",
		Description: "Cari gambar di Rule34",
		IsPrefix:    true,
		IsQuery:     true,
		IsPrivate:   true,
		Exec:        func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			handleBooru(ctx, client, m, cfg, r34Fetcher)
		},
	})
	commands.Register(&commands.Command{
		Name:        "danbooru",
		As:          []string{"dan"},
		Tags:        "nsfw",
		Description: "Cari gambar di Danbooru",
		IsPrefix:    true,
		IsQuery:     true,
		IsPrivate:   true,
		Exec:        func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
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
		Exec:        func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			handleBooru(ctx, client, m, cfg, yandereFetcher)
		},
	})
}

func handleBooru(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config, fetcher booruFetcher) {
	tags := strings.ReplaceAll(m.Query, " ", "+")
	url, caption, err := fetcher(ctx, tags, 10)
	if err != nil {
		m.Reply(ctx, "❌ "+err.Error())
		return
	}

	buff, err := client.FetchBytes(url)
	if err != nil {
		m.Reply(ctx, "❌ Gagal mengunduh gambar: "+err.Error())
		return
	}

	_, err = client.SendImage(ctx, m.From, buff, caption, m.ID)
	if err != nil {
		m.Reply(ctx, "❌ Gagal mengirim gambar: "+err.Error())
	}
}

func r34Fetcher(ctx context.Context, tags string, limit int) (string, string, error) {
	ap := api.Shared("https://api.rule34.xxx", 15*time.Second)
	posts, err := ap.Rule34(ctx, tags, limit)
	if err != nil {
		return "", "", fmt.Errorf("Gagal mengambil data dari Rule34: %v", err)
	}
	if len(posts) == 0 {
		return "", "", fmt.Errorf("Gambar tidak ditemukan.")
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	post := posts[r.Intn(len(posts))]
	
	url := post.FileURL
	if url == "" {
		url = post.SampleURL
	}
	caption := fmt.Sprintf("🏷️ Tags: %s\n⭐ Score: %v", post.Tags, post.Score)
	return url, caption, nil
}

func danbooruFetcher(ctx context.Context, tags string, limit int) (string, string, error) {
	ap := api.Shared("https://danbooru.donmai.us", 15*time.Second)
	posts, err := ap.Danbooru(ctx, tags, limit)
	if err != nil {
		return "", "", fmt.Errorf("Gagal mengambil data dari Danbooru: %v", err)
	}
	if len(posts) == 0 {
		return "", "", fmt.Errorf("Gambar tidak ditemukan.")
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	post := posts[r.Intn(len(posts))]
	
	url := post.LargeURL
	if url == "" {
		url = post.FileURL
	}
	caption := fmt.Sprintf("🏷️ Tags: %s\n⭐ Score: %v", post.Tags, post.Score)
	return url, caption, nil
}

func yandereFetcher(ctx context.Context, tags string, limit int) (string, string, error) {
	ap := api.Shared("https://yande.re", 15*time.Second)
	posts, err := ap.Yandere(ctx, tags, limit)
	if err != nil {
		return "", "", fmt.Errorf("Gagal mengambil data dari Yandere: %v", err)
	}
	if len(posts) == 0 {
		return "", "", fmt.Errorf("Gambar tidak ditemukan.")
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	post := posts[r.Intn(len(posts))]
	
	url := post.SampleURL
	if url == "" {
		url = post.FileURL
	}
	caption := fmt.Sprintf("🏷️ Tags: %s\n⭐ Score: %v", post.Tags, post.Score)
	return url, caption, nil
}
