package handlers

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	bilimedia "github.com/MapIHS/kotonehara/internal/media/bilibili"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

var bilibiliPath = regexp.MustCompile(`^/video/(BV[0-9A-Za-z]{10}|av[1-9][0-9]*)/?$`)
var bilibiliOpusPath = regexp.MustCompile(`^/opus/[1-9][0-9]{0,24}/?$`)

func parseBilibiliQuery(query string) (string, string, error) {
	args := strings.Fields(query)
	if len(args) < 1 || len(args) > 2 {
		return "", "", fmt.Errorf("gunakan bilibili <url> [kualitas], contoh: bilibili https://www.bilibili.com/video/BV1Ak8Q6hECg/ 1080p")
	}
	u, err := url.Parse(args[0])
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return "", "", fmt.Errorf("link Bilibili tidak valid")
	}
	switch strings.ToLower(u.Hostname()) {
	case "bilibili.com", "www.bilibili.com", "m.bilibili.com":
		if !bilibiliPath.MatchString(u.Path) && !bilibiliOpusPath.MatchString(u.Path) {
			return "", "", fmt.Errorf("gunakan link video Bilibili BV/av atau link Opus")
		}
	case "b23.tv", "bili2233.cn":
		if strings.Trim(u.Path, "/") == "" {
			return "", "", fmt.Errorf("short link Bilibili tidak lengkap")
		}
	default:
		return "", "", fmt.Errorf("domain Bilibili tidak didukung; gunakan bilibili.com atau b23.tv")
	}
	quality := "1080p"
	if len(args) == 2 {
		quality = strings.ToLower(args[1])
		if quality == "360" || quality == "480" || quality == "720" || quality == "1080" {
			quality += "p"
		}
	}
	if _, ok := bilimedia.QualityCode(quality); !ok {
		return "", "", fmt.Errorf("kualitas: 360p, 480p, 720p, 720p60, 1080p, 1080p60, atau 4k")
	}
	return u.String(), quality, nil
}

func handleBilibili(ctx context.Context, m *message.Message,
	fetch func(context.Context, string, string) (*api.BilibiliResult, error),
	prepare func(context.Context, bilimedia.Selection, map[string]string) ([]byte, error),
	send func(context.Context, []byte, string) error,
	sendImages func(context.Context, *api.BilibiliResult) error,
) {
	target, quality, err := parseBilibiliQuery(m.Query)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}
	m.Reply(ctx, "Sedang mengambil media Bilibili...")
	workCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	result, err := fetch(workCtx, target, quality)
	if err != nil {
		m.Reply(ctx, "Gagal mengambil Bilibili: "+err.Error())
		return
	}
	if result != nil && result.Format == "images" {
		if err := sendImages(workCtx, result); err != nil {
			m.Reply(ctx, "Gagal mengirim media Opus: "+err.Error())
		}
		return
	}
	selection, err := bilimedia.Select(result, quality)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}
	if selection.QualityLabel() != quality {
		m.Reply(ctx, fmt.Sprintf("Kualitas %s tidak tersedia; memakai %s.", quality, selection.QualityLabel()))
	}
	data, err := prepare(workCtx, selection, result.Headers)
	if err != nil {
		m.Reply(ctx, "Gagal menyiapkan video Bilibili: "+err.Error())
		return
	}
	title := []rune(result.Title)
	if len(title) > 700 {
		title = append(title[:700], []rune("...")...)
	}
	caption := fmt.Sprintf("%s\n%s • %s • Bagian %d", string(title), result.Author.Name, selection.QualityLabel(), result.Page)
	if err := send(workCtx, data, caption); err != nil {
		m.Reply(ctx, "Video Bilibili gagal dikirim: "+err.Error())
	}
}

func init() {
	commands.Register(&commands.Command{
		Name: "bilibili", As: []string{"bili", "bilidl"}, Tags: "downloader",
		Description: "Unduh video atau gambar/GIF Opus Bilibili: <url> [kualitas]",
		IsQuery:     true, IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			ap := api.Shared(cfg.BASEApiURL, 90*time.Second)
			handleBilibili(ctx, m, ap.Bilibili,
				func(ctx context.Context, selection bilimedia.Selection, headers map[string]string) ([]byte, error) {
					return bilimedia.Prepare(ctx, ap.HTTP, selection, headers)
				},
				func(ctx context.Context, data []byte, caption string) error {
					_, err := client.SendVideo(ctx, m.From, data, false, caption, m.ID)
					return err
				},
				func(ctx context.Context, result *api.BilibiliResult) error {
					return sendBilibiliImages(ctx, result,
						func(ctx context.Context, item api.BilibiliMedia, remaining int64) ([]byte, error) {
							return bilimedia.FetchImage(ctx, ap.HTTP, item, result.Headers, remaining)
						},
						func(ctx context.Context, data []byte, gif bool, caption string) error {
							if gif {
								_, err := client.SendVideo(ctx, m.From, data, true, caption, m.ID)
								return err
							}
							_, err := client.SendImage(ctx, m.From, data, caption, m.ID)
							return err
						},
					)
				},
			)
		},
	})
}
