package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func init() {
	commands.Register(&commands.Command{
		Name: "youtubesearch", As: []string{"ytsearch", "yts"}, Tags: "downloader", Description: "Cari video YouTube: <kata kunci>", IsPrefix: true, IsQuery: true,
		Exec: func(ctx context.Context, _ *clients.Client, m *message.Message, cfg config.Config) {
			if strings.TrimSpace(cfg.BASEApiURL) == "" {
				m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
				return
			}
			workCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()
			results, err := api.Shared(cfg.BASEApiURL, 90*time.Second).YoutubeSearch(workCtx, strings.TrimSpace(m.Query), 5)
			if err != nil {
				m.Reply(ctx, "Gagal mencari YouTube: "+err.Error())
				return
			}
			if len(results) == 0 {
				m.Reply(ctx, "Tidak ada video YouTube yang ditemukan.")
				return
			}
			var entries []string
			for _, item := range results {
				entries = append(entries, fmt.Sprintf("%d. %s\n%s · %s · %d tayangan\n%s", len(entries)+1, wikiExcerpt(item.Title, 150), wikiExcerpt(item.Channel.Name, 100), fmtPlayDuration(item.Duration), item.Views, item.URL))
				if len(entries) == 5 {
					break
				}
			}
			m.Reply(ctx, "Hasil YouTube:\n\n"+strings.Join(entries, "\n\n")+"\n\nUnduh dengan "+cfg.Prefix+"yta <URL> atau "+cfg.Prefix+"ytv <URL>.")
		},
	})
	commands.Register(&commands.Command{
		Name: "youtubeinfo", As: []string{"ytinfo"}, Tags: "downloader", Description: "Info video YouTube: <URL>", IsPrefix: true, IsQuery: true,
		Exec: func(ctx context.Context, _ *clients.Client, m *message.Message, cfg config.Config) {
			if strings.TrimSpace(cfg.BASEApiURL) == "" {
				m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
				return
			}
			args := strings.Fields(m.Query)
			if len(args) != 1 || !message.IsValidURL(args[0]) {
				m.Reply(ctx, "Gunakan ytinfo <URL YouTube>.")
				return
			}
			workCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()
			info, err := api.Shared(cfg.BASEApiURL, 90*time.Second).YoutubeInfo(workCtx, args[0])
			if err != nil {
				m.Reply(ctx, "Gagal mengambil info YouTube: "+err.Error())
				return
			}
			if info == nil || info.ID == "" {
				m.Reply(ctx, "Info video YouTube tidak ditemukan.")
				return
			}
			m.Reply(ctx, fmt.Sprintf("*%s*\nChannel: %s\nDurasi: %s\nTayangan: %d\nLikes: %d\nKualitas: %s\n\n%s", wikiExcerpt(info.Title, 200), wikiExcerpt(info.Channel.Name, 100), fmtPlayDuration(info.Duration), info.Views, info.Likes, strings.Join(info.Videos, ", "), wikiExcerpt(info.Description, 2500)))
		},
	})
}
