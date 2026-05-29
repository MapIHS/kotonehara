package pkg

import (
	"context"
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
	match := re.FindString(quality)

	if match == "" {
		return 0
	}

	n, err := strconv.Atoi(match)
	if err != nil {
		return 0
	}

	return n
}

func SelectMediaToSend(mediaLinks []api.TwitterMediaLink) []api.TwitterMediaLink {
	var bestVideo *api.TwitterMediaLink
	bestQuality := 0

	var images []api.TwitterMediaLink

	for i := range mediaLinks {
		media := mediaLinks[i]

		switch media.MediaType {
		case api.TwitterMediaTypeVideo:
			q := parseQuality(media.Quality)

			if bestVideo == nil || q > bestQuality {
				bestVideo = &mediaLinks[i]
				bestQuality = q
			}

		case api.TwitterMediaTypeImage:
			images = append(images, media)
		}
	}

	// Kalau ada video, kirim video paling HD saja.
	// Image yang ada di response video dianggap thumbnail, jadi skip.
	if bestVideo != nil {
		return []api.TwitterMediaLink{*bestVideo}
	}

	// Kalau tidak ada video, berarti post foto.
	// Kirim semua foto.
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

			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.New(cfg.BASEApiURL, 60*time.Second)

			res, err := ap.X(ctx, args[0])
			if err != nil {
				m.Reply(ctx, "Gagal.")
				return
			}

			mediaLinks := SelectMediaToSend(res.MediaLinks)
			for _, media := range mediaLinks {
				switch media.MediaType {
				case api.TwitterMediaTypeVideo:
					buff, err := client.FetchBytes(media.URL)
					if err != nil {
						m.Reply(ctx, "Gagal mengambil video.")
						return
					}
					client.SendVideo(ctx, m.From, buff, false, "", m.ID)

				case api.TwitterMediaTypeImage:
					buff, err := client.FetchBytes(media.URL)
					if err != nil {
						m.Reply(ctx, "Gagal mengambil gambar.")
						return
					}
					client.SendImage(ctx, m.From, buff, "", m.ID)
				}

			}
		},
	})
}
