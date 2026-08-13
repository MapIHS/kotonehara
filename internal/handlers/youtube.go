package handlers

import (
	"context"
	"fmt"
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

	res, err := ap.YoutubeDownload(ctx, targetURL, quality, true)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}

	client.SendVideo(ctx, m.From, res, false, "", m.ID)

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

	res, err := ap.YoutubeDownload(ctx, targetURL, "", false)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}

	cleanTitle := regexp.MustCompile(`[|\\?*<:>+\[\]\/]`).ReplaceAllString(info.Title, "_")
	fileName := cleanTitle + ".mp3"

	client.SendDocument(ctx, m.From, res, fileName, "", m.ID)

}

func init() {
	commands.Register(&commands.Command{
		Name:     "youtubevideo",
		As:       []string{"ytv"},
		Tags:     "downloader",
		IsQuery:  true,
		IsPrefix: true,
		Exec:     ytv,
	})

	commands.Register(&commands.Command{
		Name:     "youtubeaudio",
		As:       []string{"yta"},
		Tags:     "downloader",
		IsQuery:  true,
		IsPrefix: true,
		Exec:     yta,
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
