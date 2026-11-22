package handlers

import (
	"context"
	"fmt"
	neturl "net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func ytv(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	args := strings.Fields(m.Query)
	if len(args) == 0 || !message.IsValidURL(args[0]) {
		m.Reply(ctx, "Link YouTube tidak valid. Pastikan kamu mengirimkan link yang benar.")
		return
	}

	m.Reply(ctx, "Tunggu Sebentar ya.")

	targetURL := args[0]
	quality := "360p"
	qualityRequested := false

	ap := api.Shared(cfg.BASEApiURL, 0)

	if len(args) > 1 {
		quality = strings.TrimSuffix(args[1], "p") + "p"
		qualityRequested = true
	}

	if isYoutubePostURL(targetURL) {
		post, err := ap.YoutubePost(ctx, targetURL)
		if err != nil {
			m.Reply(ctx, "Gagal mengambil post YouTube. Coba lagi sebentar, yaa.")
			return
		}
		if len(post.Photos) > 0 {
			err := sendYoutubePostPhotos(post, func(items []clients.AlbumMedia) error {
				_, err := client.SendAlbum(ctx, m.From, items, m.ID)
				return err
			})
			if err != nil {
				replyAlbumFailure(ctx, m, err, "Gagal mengirim gambar post YouTube.")
			}
			return
		}
		if post.VideoURL == "" {
			m.Reply(ctx, "Post YouTube ini tidak memiliki gambar atau video, yaa.")
			return
		}
		targetURL = post.VideoURL
	}

	info, err := ap.YoutubeInfo(ctx, targetURL)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}

	availableQualities := info.Videos

	if !contains(availableQualities, quality) {
		if qualityRequested {
			// Coba cari resolusi terdekat yang <= yang diminta
			reqH := parseHeight(quality)
			best := ""
			bestH := 0
			for _, q := range availableQualities {
				h := parseHeight(q)
				if h <= reqH && h > bestH {
					bestH = h
					best = q
				}
			}
			if best == "" && len(availableQualities) > 0 {
				best = availableQualities[0]
			}
			quality = best
		} else {
			// Tidak ada quality request, pakai fallback default
			if contains(availableQualities, "480p") {
				quality = "480p"
			} else if contains(availableQualities, "720p") {
				quality = "720p"
			} else if len(availableQualities) > 0 {
				quality = availableQualities[0]
			}
		}
	}

	m.Reply(ctx, fmt.Sprintf("Lagi didownload '%s' (%s)...", info.Title, quality))

	res, err := ap.YoutubeDownloadFile(ctx, targetURL, quality, true)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}

	defer os.Remove(res.Name())
	defer res.Close()
	if _, err := client.SendVideoFile(ctx, m.From, res, false, "", m.ID); err != nil {
		m.Reply(ctx, err.Error())
	}

}

func isYoutubePostURL(raw string) bool {
	u, err := neturl.Parse(raw)
	if err != nil || u.User != nil || u.Port() != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return (host == "youtube.com" || host == "www.youtube.com" || host == "m.youtube.com") && strings.HasPrefix(u.Path, "/post/")
}

func sendYoutubePostPhotos(post *api.YoutubePostResult, send func([]clients.AlbumMedia) error) error {
	if post == nil || len(post.Photos) == 0 || len(post.Photos) > clients.MaxAlbumItems {
		return clients.ErrAlbumLimit
	}
	caption := post.Text
	if post.Author != "" {
		caption = post.Author + "\n\n" + caption
	}
	if text := []rune(caption); len(text) > 1024 {
		caption = string(text[:1024])
	}
	medias := make([]clients.AlbumMedia, 0, len(post.Photos))
	for _, photo := range post.Photos {
		if strings.TrimSpace(photo.URL) == "" {
			continue
		}
		item := clients.NewAlbumImageURL(photo.URL)
		if len(medias) == 0 {
			item.Caption = caption
		}
		medias = append(medias, item)
	}
	if len(medias) == 0 {
		return clients.ErrAlbumLimit
	}
	return send(medias)
}

func yta(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	args := strings.Fields(m.Query)
	if len(args) == 0 || !message.IsValidURL(args[0]) {
		m.Reply(ctx, "Link YouTube tidak valid. Pastikan kamu mengirimkan link yang benar.")
		return
	}

	m.Reply(ctx, "Tunggu Sebentar ya.")

	targetURL := args[0]

	ap := api.Shared(cfg.BASEApiURL, 0)

	info, err := ap.YoutubeInfo(ctx, targetURL)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}

	m.Reply(ctx, fmt.Sprintf("Lagi didownload '%s'...", info.Title))

	res, err := ap.YoutubeDownloadFile(ctx, targetURL, "", false)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}

	defer os.Remove(res.Name())
	defer res.Close()
	cleanTitle := regexp.MustCompile(`[|\\?*<:>+\[\]\/]`).ReplaceAllString(info.Title, "_")
	fileName := cleanTitle + ".mp3"

	if _, err := client.SendDocumentFile(ctx, m.From, res, fileName, "", m.ID); err != nil {
		m.Reply(ctx, err.Error())
	}

}

func init() {
	commands.Register(&commands.Command{
		Name:        "youtubevideo",
		Description: "Download video atau gambar post dari link YouTube",
		As:          []string{"ytv"},
		Tags:        "downloader",
		IsQuery:     true,
		IsPrefix:    true,
		Exec:        ytv,
	})

	commands.Register(&commands.Command{
		Name:        "youtubeaudio",
		Description: "Download audio dari link YouTube",
		As:          []string{"yta"},
		Tags:        "downloader",
		IsQuery:     true,
		IsPrefix:    true,
		Exec:        yta,
	})
}

func parseHeight(quality string) int {
	n, _ := strconv.Atoi(strings.TrimSuffix(quality, "p"))
	return n
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
