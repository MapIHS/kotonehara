package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"path"
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

var reDigits = regexp.MustCompile(`\d+`)

func parseQuality(quality string) int {
	matches := reDigits.FindAllString(quality, -1)
	if len(matches) == 0 {
		return 0
	}

	best := 0
	for _, m := range matches {
		n, err := strconv.Atoi(m)
		if err != nil {
			continue
		}
		if n > best {
			best = n
		}
	}

	return best
}

func parseQualityFromURL(rawURL string) int {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}

	file := path.Base(u.Path)
	if q := parseQuality(file); q > 0 {
		return q
	}

	segments := strings.Split(u.Path, "/")
	for _, segment := range segments {
		segment = strings.ToLower(segment)
		if !strings.Contains(segment, "x") {
			continue
		}

		parts := strings.Split(segment, "x")
		for _, part := range parts {
			if n, err := strconv.Atoi(part); err == nil && n > 0 {
				return n
			}
		}
	}

	return 0
}

func mediaQuality(media api.TwitterMediaLink) int {
	q := parseQuality(media.Quality)
	if q > 0 {
		return q
	}
	return parseQualityFromURL(media.URL)
}

func resolveSnapCDNURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	if !strings.EqualFold(u.Hostname(), "dl.snapcdn.app") {
		return rawURL
	}

	token := u.Query().Get("token")
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return rawURL
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return rawURL
	}

	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return rawURL
	}
	if payload.URL == "" {
		return rawURL
	}

	parsedPayload, err := url.Parse(payload.URL)
	if err != nil {
		return rawURL
	}
	if parsedPayload.Scheme != "https" && parsedPayload.Scheme != "http" {
		return rawURL
	}

	return payload.URL
}

func SelectMediaToSend(mediaLinks []api.TwitterMediaLink) []api.TwitterMediaLink {
	var bestVideo *api.TwitterMediaLink
	bestQuality := 0

	var images []api.TwitterMediaLink

	for i := range mediaLinks {
		media := mediaLinks[i]

		switch media.MediaType {
		case api.TwitterMediaTypeVideo:
			q := mediaQuality(media)

			if bestVideo == nil || q > bestQuality {
				bestVideo = &mediaLinks[i]
				bestQuality = q
			}

		case api.TwitterMediaTypeImage:
			images = append(images, media)
		}
	}

	if bestVideo != nil {
		return []api.TwitterMediaLink{*bestVideo}
	}

	return images
}

func init() {
	commands.Register(&commands.Command{
		Name:        "x",
		Description: "Download video/gambar dari link X (Twitter)",
		As:          []string{"x", "twitter"},
		Tags:        "downloader",
		IsQuery:     true,
		IsPrefix:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			args := strings.Fields(m.Query)
			if len(args) == 0 || !message.IsValidURL(args[0]) {
				m.Reply(ctx, "Link X/Twitter tidak valid. Pastikan kamu mengirimkan link yang benar.")
				return
			}

			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.Shared(cfg.BASEApiURL, 90*time.Second)

			res, err := ap.X(ctx, args[0])
			if err != nil {
				m.Reply(ctx, "Gagal: "+err.Error())
				return
			}

			mediaLinks := SelectMediaToSend(res.MediaLinks)
			if len(mediaLinks) == 0 {
				m.Reply(ctx, "Media tidak ditemukan dari link X/Twitter ini.")
				return
			}

			var albumMedia []clients.AlbumMedia
			for _, media := range mediaLinks {
				mediaURL := resolveSnapCDNURL(media.URL)
				buff, err := client.FetchBytes(mediaURL)
				if err != nil {
					continue
				}

				switch media.MediaType {
				case api.TwitterMediaTypeVideo:
					albumMedia = append(albumMedia, clients.NewAlbumVideo(buff, false))
				case api.TwitterMediaTypeImage:
					albumMedia = append(albumMedia, clients.NewAlbumImage(buff))
				}
			}

			if _, err := client.SendAlbum(ctx, m.From, albumMedia, m.ID); err != nil {
				m.Reply(ctx, "Gagal mengirim media X/Twitter: "+err.Error())
			}
		},
	})
}
