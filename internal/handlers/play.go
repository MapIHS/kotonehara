package handlers

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

var rePlayCleanFilename = regexp.MustCompile(`[|\\?*<:>+\[\]/]`)

func fmtPlayDuration(seconds float64) string {
	total := int(seconds)
	min := total / 60
	sec := total % 60
	return fmt.Sprintf("%d:%02d", min, sec)
}

func play(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	query := strings.TrimSpace(m.Query)
	if query == "" {
		m.Reply(ctx, "Tulis judul lagu yang mau dicari, yaa.\nContoh: .play Coldplay Yellow")
		return
	}

	m.Reply(ctx, fmt.Sprintf("🔎 Mencari *%s*...", query))

	ap := api.Shared(cfg.BASEApiURL, 0)

	// Search YouTube
	results, err := ap.YoutubeSearch(ctx, query, 1)
	if err != nil || len(results) == 0 {
		m.Reply(ctx, "Tidak ditemukan hasil untuk pencarian tersebut.")
		return
	}

	first := results[0]
	videoURL := first.URL
	if videoURL == "" {
		videoURL = fmt.Sprintf("https://www.youtube.com/watch?v=%s", first.ID)
	}

	m.Reply(ctx, fmt.Sprintf("🎵 *%s*\n👤 %s | ⏱ %s\n\n📥 Sedang mengunduh audio...",
		first.Title, first.Channel.Name, fmtPlayDuration(first.Duration)))

	// Download audio
	audioData, err := ap.YoutubeDownloadFile(ctx, videoURL, "", false)
	if err != nil {
		m.Reply(ctx, fmt.Sprintf("Gagal mengunduh audio: %s", err.Error()))
		return
	}

	defer os.Remove(audioData.Name())
	defer audioData.Close()
	info, err := audioData.Stat()
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}

	cleanTitle := rePlayCleanFilename.ReplaceAllString(first.Title, "_")
	fileName := cleanTitle + ".mp3"

	// Send as audio if <= 15MB, else as document
	const maxAudioSize = 15 * 1024 * 1024
	if info.Size() <= maxAudioSize {
		if _, err := client.SendAudioFile(ctx, m.From, audioData, false, m.ID); err != nil {
			// Fallback: send as document
			client.SendDocumentFile(ctx, m.From, audioData, fileName, "", m.ID)
		}
	} else {
		client.SendDocumentFile(ctx, m.From, audioData, fileName, "", m.ID)
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:     "play",
		As:       []string{"play", "lagu"},
		Tags:     "downloader",
		IsQuery:  true,
		IsPrefix: true,
		Exec:     play,
	})
}
