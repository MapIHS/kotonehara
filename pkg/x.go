package pkg

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func parseQuality(quality string) int {
	re := regexp.MustCompile(`\d+`)
	matches := re.FindAllString(quality, -1)
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
		Name:     "x",
		As:       []string{"x", "twitter"},
		Tags:     "downloader",
		IsQuery:  true,
		IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			args := strings.Fields(m.Query)
			if len(args) == 0 {
				m.Reply(ctx, "Link X/Twitter-nya belum ada.")
				return
			}

			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.New(cfg.BASEApiURL, 60*time.Second)

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

			for _, media := range mediaLinks {
				mediaURL := resolveSnapCDNURL(media.URL)

				switch media.MediaType {
				case api.TwitterMediaTypeVideo:
					buff, err := client.FetchBytes(mediaURL)
					if err != nil {
						m.Reply(ctx, "Gagal mengambil video: "+err.Error())
						return
					}

					if _, err := client.SendVideo(ctx, m.From, buff, false, "", m.ID); err != nil {
						m.Reply(ctx, fmt.Sprintf("Gagal mengirim video (%s): %v", media.Quality, err))
						return
					}

				case api.TwitterMediaTypeImage:
					buff, err := client.FetchBytes(mediaURL)
					if err != nil {
						m.Reply(ctx, "Gagal mengambil gambar: "+err.Error())
						return
					}

					if _, err := client.SendImage(ctx, m.From, buff, "", m.ID); err != nil {
						m.Reply(ctx, fmt.Sprintf("Gagal mengirim gambar (%s): %v", media.Quality, err))
						return
					}
				}

			}
		},
	})
}
