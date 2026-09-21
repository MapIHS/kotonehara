package handlers

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

var redditPostPath = regexp.MustCompile(`(?i)^/(?:r/[^/]+/|(?:user|u)/[^/]+/)?comments/[a-z0-9]+(?:/|$)`)
var redditSharePath = regexp.MustCompile(`(?i)^/(?:r/[^/]+/)?s/[a-z0-9]+/?$`)
var redditGalleryPath = regexp.MustCompile(`(?i)^/gallery/[a-z0-9]+/?$`)
var redditShortPath = regexp.MustCompile(`(?i)^/[a-z0-9]+/?$`)

func parseRedditQuery(query string) (string, int, error) {
	args := strings.Fields(query)
	if len(args) < 1 || len(args) > 2 {
		return "", 0, fmt.Errorf("gunakan reddit <url> [kualitas], contoh: reddit https://www.reddit.com/r/Kucing/s/zd1rVlyJGL 720p")
	}
	u, err := url.Parse(args[0])
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return "", 0, fmt.Errorf("link Reddit tidak valid")
	}
	valid := false
	switch strings.ToLower(u.Hostname()) {
	case "reddit.com", "www.reddit.com", "old.reddit.com", "new.reddit.com", "m.reddit.com":
		valid = redditPostPath.MatchString(u.Path) || redditSharePath.MatchString(u.Path) || redditGalleryPath.MatchString(u.Path)
	case "redd.it", "www.redd.it":
		valid = redditShortPath.MatchString(u.Path)
	}
	if !valid {
		return "", 0, fmt.Errorf("gunakan link postingan atau tautan berbagi Reddit")
	}
	quality := 0
	if len(args) == 2 {
		quality, err = strconv.Atoi(strings.TrimSuffix(strings.ToLower(args[1]), "p"))
		if err != nil || quality < 1 || quality > 4320 {
			return "", 0, fmt.Errorf("kualitas tidak valid; contoh: 720p atau 1080p")
		}
	}
	return u.String(), quality, nil
}

func selectRedditMedia(result *api.RedditResult, quality int) (api.RedditMedia, error) {
	if result == nil {
		return api.RedditMedia{}, fmt.Errorf("media Reddit tidak ditemukan")
	}
	var best api.RedditMedia
	for _, item := range result.Media {
		if item.Type != "video" && item.Type != "image" && item.Type != "audio" {
			continue
		}
		if api.ValidateRedditMediaURL(item.URL) != nil {
			continue
		}
		if quality != 0 && item.QualityNumber != quality {
			continue
		}
		if best.URL == "" || (item.Type == "video" && best.Type != "video") || (item.Type == best.Type && item.QualityNumber > best.QualityNumber) {
			best = item
		}
	}
	if best.URL == "" {
		if quality != 0 {
			return best, fmt.Errorf("kualitas %dp tidak tersedia; coba tanpa pilihan kualitas", quality)
		}
		return best, fmt.Errorf("media Reddit yang didukung tidak ditemukan")
	}
	return best, nil
}

// Gallery entries are separate media, while ordinary video entries are quality alternatives.
func selectRedditItems(result *api.RedditResult, quality int) ([]api.RedditMedia, error) {
	if result == nil || !result.IsGallery {
		item, err := selectRedditMedia(result, quality)
		if err != nil {
			return nil, err
		}
		return []api.RedditMedia{item}, nil
	}
	if quality != 0 {
		return nil, fmt.Errorf("galeri memakai kualitas asli; coba tanpa pilihan kualitas")
	}
	var items []api.RedditMedia
	seen := make(map[string]bool)
	for _, item := range result.Media {
		if len(items) >= 20 {
			break
		}
		if item.Type != "image" && item.Type != "video" {
			continue
		}
		if seen[item.URL] || api.ValidateRedditMediaURL(item.URL) != nil {
			continue
		}
		seen[item.URL] = true
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("media galeri Reddit tidak ditemukan")
	}
	return items, nil
}

func handleReddit(ctx context.Context, m *message.Message,
	fetch func(context.Context, string) (*api.RedditResult, error),
	download func(context.Context, api.RedditMedia) ([]byte, error),
	send func(context.Context, api.RedditMedia, []byte, string) error,
) {
	target, quality, err := parseRedditQuery(m.Query)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}
	workCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	m.Reply(ctx, "Sedang mengambil media Reddit...")
	result, err := fetch(workCtx, target)
	if err != nil {
		m.Reply(ctx, "Gagal mengambil Reddit: "+err.Error())
		return
	}
	items, err := selectRedditItems(result, quality)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}
	title := []rune(strings.TrimSpace(result.Title))
	if len(title) > 700 {
		title = append(title[:700], []rune("...")...)
	}
	for i, item := range items {
		if workCtx.Err() != nil {
			m.Reply(ctx, "Waktu unduh Reddit habis.")
			return
		}
		caption := strings.TrimSpace(fmt.Sprintf("%s\n%s • %s", string(title), result.Uploader, item.Quality))
		if len(items) > 1 {
			caption += fmt.Sprintf("\n%d/%d", i+1, len(items))
		}
		if item.Type == "video" && item.AudioURL == "" && item.HasAudio != nil && !*item.HasAudio && !item.IsGIF {
			caption += "\nVideo tanpa audio."
		}
		data, err := download(workCtx, item)
		if err != nil {
			m.Reply(ctx, "Media gagal diunduh: "+err.Error()+"\nTautan unduhan:\n"+item.URL)
			continue
		}
		if err := send(workCtx, item, data, caption); err != nil {
			m.Reply(ctx, "Media Reddit gagal dikirim: "+err.Error()+"\nTautan unduhan:\n"+item.URL)
		}
	}
}

func init() {
	commands.Register(&commands.Command{
		Name: "reddit", As: []string{"redditdl", "rdl"}, Tags: "downloader", Disable: true,
		Description: "Unduh media Reddit: <url> [kualitas]", IsQuery: true, IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			ap := api.Shared(cfg.BASEApiURL, 90*time.Second)
			handleReddit(ctx, m, ap.Reddit, ap.RedditMediaBytes,
				func(ctx context.Context, item api.RedditMedia, data []byte, caption string) error {
					switch item.Type {
					case "video":
						_, err := client.SendVideo(ctx, m.From, data, item.IsGIF, caption, m.ID)
						return err
					case "image":
						if item.Format == "gif" {
							_, err := client.SendVideo(ctx, m.From, data, true, caption, m.ID)
							return err
						}
						_, err := client.SendImage(ctx, m.From, data, caption, m.ID)
						return err
					case "audio":
						_, err := client.SendAudio(ctx, m.From, data, false, m.ID)
						return err
					default:
						return fmt.Errorf("tipe media Reddit tidak didukung")
					}
				})
		},
	})
}
