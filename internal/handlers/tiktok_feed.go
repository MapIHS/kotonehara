package handlers

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func init() {
	for _, kind := range []string{"search", "trending", "user"} {
		aliases := map[string][]string{"search": {"ttsearch"}, "trending": {"tttrending"}, "user": {"ttuser"}}
		descriptions := map[string]string{"search": "Cari TikTok: <kata kunci> [| cursor]", "trending": "TikTok trending: [kode negara, contoh ID]", "user": "Posting TikTok: <username> [| cursor]"}
		commands.Register(&commands.Command{
			Name: "tiktok" + kind, As: aliases[kind], Tags: "downloader", Description: descriptions[kind], IsPrefix: true, IsQuery: kind != "trending",
			Exec: func(ctx context.Context, _ *clients.Client, m *message.Message, cfg config.Config) {
				if strings.TrimSpace(cfg.BASEApiURL) == "" {
					m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
					return
				}
				query, cursor, err := parseTikTokFeedQuery(m.Query)
				if err != nil {
					m.Reply(ctx, err.Error())
					return
				}
				workCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
				defer cancel()
				ap := api.Shared(cfg.BASEApiURL, 60*time.Second)
				var feed *api.TikTokFeed
				switch kind {
				case "search":
					feed, err = ap.TikTokSearch(workCtx, query, cursor)
				case "user":
					feed, err = ap.TikTokUserFeed(workCtx, query, cursor)
				case "trending":
					if cursor != "" {
						m.Reply(ctx, "Gunakan tttrending [kode negara], contoh tttrending ID.")
						return
					}
					var items []api.TikTokResult
					items, err = ap.TikTokTrending(workCtx, query)
					feed = &api.TikTokFeed{Lists: items}
				}
				if err != nil {
					m.Reply(ctx, "Gagal mengambil TikTok: "+err.Error())
					return
				}
				m.Reply(ctx, formatTikTokFeed(feed, cfg.Prefix+"tt"+kind, query, cfg.Prefix))
			},
		})
	}
}

func parseTikTokFeedQuery(input string) (string, string, error) {
	parts := strings.Split(input, "|")
	if len(parts) > 2 {
		return "", "", fmt.Errorf("gunakan <kata kunci/username> | cursor untuk halaman berikutnya")
	}
	query := strings.TrimSpace(parts[0])
	cursor := ""
	if len(parts) == 2 {
		cursor = strings.TrimSpace(parts[1])
		if query == "" || cursor == "" || strings.ContainsAny(cursor, " \t\r\n") {
			return "", "", fmt.Errorf("kata kunci/username dan cursor tidak boleh kosong atau tidak valid")
		}
	}
	return query, cursor, nil
}

func formatTikTokFeed(feed *api.TikTokFeed, command, query, prefix string) string {
	if feed == nil || len(feed.Lists) == 0 {
		return "Tidak ada hasil TikTok yang ditemukan."
	}
	var entries []string
	for _, item := range feed.Lists {
		if item.ID == "" || item.Author.Username == "" {
			continue
		}
		kind := "video"
		if len(item.Images) > 0 {
			kind = "photo"
		}
		link := "https://www.tiktok.com/@" + url.PathEscape(item.Author.Username) + "/" + kind + "/" + url.PathEscape(item.ID)
		entries = append(entries, fmt.Sprintf("%d. %s\n@%s · %d detik · %d tayangan\n%s", len(entries)+1, wikiExcerpt(item.Title, 120), wikiExcerpt(item.Author.Username, 24), item.Duration, item.Played, link))
		if len(entries) == 15 {
			break
		}
	}
	if len(entries) == 0 {
		return "Hasil TikTok tidak memiliki tautan posting yang valid."
	}
	text := "Hasil TikTok:\n\n" + strings.Join(entries, "\n\n") + "\n\nUnduh dengan " + prefix + "tt <URL di atas>."
	if feed.Next && feed.NextID != "" {
		text += "\nHalaman berikutnya: " + command + " " + query + " | " + feed.NextID
	}
	return text
}
