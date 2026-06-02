package pkg

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func threads(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	args := strings.Fields(m.Query)
	if len(args) == 0 {
		m.Reply(ctx, "Link Threads-nya belum ada.")
		return
	}

	m.Reply(ctx, "Tunggu Sebentar ya.")

	ap := api.New(cfg.BASEApiURL, 90*time.Second)
	res, err := ap.Threads(ctx, args[0])
	if err != nil {
		m.Reply(ctx, "Gagal: "+err.Error())
		return
	}

	sent := 0
	var lastErr error
	for _, item := range res.Items {
		mediaURL := strings.TrimSpace(item.BestURL())
		if mediaURL == "" {
			continue
		}

		buff, err := client.FetchBytes(mediaURL)
		if err != nil {
			lastErr = err
			continue
		}

		caption := ""
		if sent == 0 {
			caption = threadsCaption(item)
		}

		if err := sendThreadsItem(ctx, client, m, item, buff, caption); err != nil {
			lastErr = err
			continue
		}
		sent++
	}

	if sent == 0 {
		if lastErr != nil {
			m.Reply(ctx, "Media Threads ditemukan, tapi gagal dikirim: "+lastErr.Error())
			return
		}
		m.Reply(ctx, "Media Threads tidak ditemukan.")
	}
}

func threadsCaption(item api.ThreadsMediaItem) string {
	parts := make([]string, 0, 2)
	if item.Username != "" {
		parts = append(parts, item.Username)
	}
	if item.Caption != "" {
		parts = append(parts, item.Caption)
	}
	caption := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if len(caption) > 900 {
		caption = caption[:900] + "..."
	}
	return caption
}

func sendThreadsItem(ctx context.Context, client *clients.Client, m *message.Message, item api.ThreadsMediaItem, data []byte, caption string) error {
	switch item.MediaType {
	case api.ThreadsMediaTypeVideo:
		_, err := client.SendVideo(ctx, m.From, data, false, caption, m.ID)
		return err
	case api.ThreadsMediaTypeGIF:
		var videoErr error
		if _, err := client.SendVideo(ctx, m.From, data, true, caption, m.ID); err == nil {
			return nil
		} else {
			videoErr = err
		}
		if _, err := client.SendImage(ctx, m.From, data, caption, m.ID); err == nil {
			return nil
		}
		return videoErr
	case api.ThreadsMediaTypeImage:
		_, err := client.SendImage(ctx, m.From, data, caption, m.ID)
		return err
	default:
		mime := http.DetectContentType(data)
		if strings.HasPrefix(mime, "video/") {
			_, err := client.SendVideo(ctx, m.From, data, false, caption, m.ID)
			return err
		}
		if strings.HasPrefix(mime, "image/") {
			_, err := client.SendImage(ctx, m.From, data, caption, m.ID)
			return err
		}
		if mime == "application/octet-stream" {
			_, err := client.SendVideo(ctx, m.From, data, false, caption, m.ID)
			return err
		}
		return fmt.Errorf("tipe media Threads tidak didukung: %s", mime)
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:     "threads",
		As:       []string{"thread", "threadsdl", "thdl"},
		Tags:     "downloader",
		IsQuery:  true,
		IsPrefix: true,
		Exec:     threads,
	})
}
