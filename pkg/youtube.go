package pkg

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func ytv(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	args := strings.Fields(m.Query)

	m.Reply(ctx, "Tunggu Sebentar ya.")

	targetURL := args[0]
	quality := "360p"

	ap := api.New(cfg.BASEApiURL, cfg.APIKEY, 15*time.Second)

	if len(args) > 1 {
		quality = strings.TrimSuffix(args[1], "p") + "p"
	}

	info, err := ap.YoutubeInfo(ctx, targetURL)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}

	availableQualities := info.Videos
	isQualityAvailable := contains(availableQualities, quality)

	if len(args) <= 1 && !isQualityAvailable {
		if contains(availableQualities, "480p") {
			quality = "480p"
		} else if contains(availableQualities, "720p") {
			quality = "720p"
		} else if len(availableQualities) > 0 {
			quality = availableQualities[0]
		}
	}

	m.Reply(ctx, fmt.Sprintf("Lagi didownload '%s' (%s)...", info.Title, quality))

	res, err := ap.YoutubeDownload(ctx, targetURL, quality, true)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}

	client.SendVideo(ctx, m.From, res, "", m.ID)

}

func yta(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	args := strings.Fields(m.Query)

	m.Reply(ctx, "Tunggu Sebentar ya.")

	targetURL := args[0]

	ap := api.New(cfg.BASEApiURL, cfg.APIKEY, 15*time.Second)

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

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
