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
		Name:        "avscan",
		As:          []string{"scanav"},
		Tags:        "nsfw",
		Description: "Mencari kode AV dari screenshot atau cuplikan adegan",
		IsPrefix:    true,
		IsPrivate:   true,
		Exec:        handleAvscan,
	})
}

func handleAvscan(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	if strings.TrimSpace(cfg.BASEApiURL) == "" {
		_, _ = m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
		return
	}

	if m.IsVideo || m.IsQuotedVideo || m.IsGif || m.IsQuotedGif {
		_, _ = m.Reply(ctx, "Perintah ini hanya mendukung gambar, bukan video atau GIF.")
		return
	}

	var imageBytes []byte
	var imageURL string

	if (m.IsImage || m.IsQuotedImage) && m.Media != nil {
		dlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		raw, err := client.DownloadMedia(dlCtx, m)
		cancel()
		if err != nil || len(raw) == 0 {
			_, _ = m.Reply(ctx, "Gambarnya belum bisa diunduh.")
			return
		}
		imageBytes = raw
	} else if queryURL := strings.TrimSpace(m.Query); queryURL != "" {
		if strings.HasPrefix(queryURL, "http://") || strings.HasPrefix(queryURL, "https://") {
			imageURL = queryURL
		} else {
			_, _ = m.Reply(ctx, "URL gambar tidak valid. Awali dengan http:// atau https://.")
			return
		}
	} else {
		_, _ = m.Reply(ctx, "Kirim atau balas gambar, atau sertakan URL gambar dengan perintah ini.")
		return
	}

	_, _ = m.Reply(ctx, "Tunggu sebentar, yaa.")

	opCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	apiClient := api.Shared(cfg.BASEApiURL, 90*time.Second)
	results, err := apiClient.AVScan(opCtx, imageBytes, imageURL)
	if err != nil {
		_, _ = m.Reply(ctx, "Pencarian gagal: "+err.Error())
		return
	}

	if len(results) == 0 {
		_, _ = m.Reply(ctx, "Tidak menemukan hasil yang cocok untuk gambar ini.")
		return
	}

	limit := 3
	if len(results) > limit {
		results = results[:limit]
	}

	for i, item := range results {
		caption := formatAvscanCaption(item, i, len(results))
		selectedFrames := selectAvscanFrames(item, 3)

		var albumMedia []clients.AlbumMedia
		for _, frame := range selectedFrames {
			if frame.ThumbURL == "" {
				continue
			}
			imgData, err := client.FetchBytes(frame.ThumbURL)
			if err != nil || len(imgData) == 0 {
				continue
			}

			// Only the first media in the album carries the caption.
			itemCaption := ""
			if len(albumMedia) == 0 {
				itemCaption = caption
			}

			albumMedia = append(albumMedia, clients.NewAlbumImage(imgData, itemCaption))
		}

		if len(albumMedia) > 0 {
			if _, err := client.SendAlbum(opCtx, m.From, albumMedia, m.ID); err != nil {
				_, _ = m.Reply(ctx, caption)
			}
		} else {
			_, _ = m.Reply(ctx, caption)
		}

		if i < len(results)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func normalizeTimestamp(ts string) string {
	t := strings.TrimSpace(strings.ToLower(ts))
	return strings.TrimPrefix(t, "00:")
}

func timestampsMatch(ts1, ts2 string) bool {
	t1 := strings.TrimSpace(ts1)
	t2 := strings.TrimSpace(ts2)
	if t1 == "" || t2 == "" || t1 == "--:--:--" || t2 == "--:--:--" {
		return false
	}
	if strings.EqualFold(t1, t2) {
		return true
	}
	return normalizeTimestamp(t1) == normalizeTimestamp(t2)
}

func selectAvscanFrames(item api.AvscanItem, maxFrames int) []api.AvscanFrame {
	if maxFrames <= 0 {
		return nil
	}

	if len(item.Frames) == 0 {
		if item.BestThumb != "" {
			return []api.AvscanFrame{{
				ThumbURL:   item.BestThumb,
				Timestamp:  item.BestTimestamp,
				Similarity: item.BestSimilarity,
			}}
		}
		return nil
	}

	if len(item.Frames) <= maxFrames {
		return item.Frames
	}

	bestIdx := -1
	if item.BestTimestamp != "" {
		for i, f := range item.Frames {
			if timestampsMatch(f.Timestamp, item.BestTimestamp) {
				bestIdx = i
				break
			}
		}
	}

	selected := make([]api.AvscanFrame, 0, maxFrames)
	if bestIdx != -1 {
		selected = append(selected, item.Frames[bestIdx])
	}

	for i, f := range item.Frames {
		if len(selected) >= maxFrames {
			break
		}
		if i == bestIdx {
			continue
		}
		selected = append(selected, f)
	}

	return selected
}

func formatSimilarity(sim float64) string {
	if sim <= 1.0 && sim > 0 {
		return fmt.Sprintf("%.2f%%", sim*100)
	}
	return fmt.Sprintf("%.2f%%", sim)
}

func formatAvscanCaption(item api.AvscanItem, index, total int) string {
	var b strings.Builder
	if total > 1 {
		fmt.Fprintf(&b, "🔍 *Hasil AVScan (%d/%d)*\n", index+1, total)
	} else {
		b.WriteString("🔍 *Hasil AVScan*\n")
	}
	b.WriteString("─────────────────\n")
	if item.VideoCode != "" {
		fmt.Fprintf(&b, "📌 *Kode*: %s\n", item.VideoCode)
	}
	if item.BestSimilarity > 0 {
		fmt.Fprintf(&b, "💯 *Kemiripan*: %s\n", formatSimilarity(item.BestSimilarity))
	}
	if item.BestTimestamp != "" && item.BestTimestamp != "--:--:--" {
		fmt.Fprintf(&b, "⏱️ *Timestamp*: %s\n", item.BestTimestamp)
	}

	urls := make([]string, 0, 4)
	if item.WatchURLs.Missav != "" {
		urls = append(urls, fmt.Sprintf("• MissAV: %s", item.WatchURLs.Missav))
	}
	if item.WatchURLs.Javdb != "" {
		urls = append(urls, fmt.Sprintf("• JavDB: %s", item.WatchURLs.Javdb))
	}
	if item.WatchURLs.Javbus != "" {
		urls = append(urls, fmt.Sprintf("• JavBus: %s", item.WatchURLs.Javbus))
	}
	if item.WatchURLs.AvscanDirect != "" {
		urls = append(urls, fmt.Sprintf("• AVScan: %s", item.WatchURLs.AvscanDirect))
	}

	if len(urls) > 0 {
		b.WriteString("\n🔗 *Tautan*:\n")
		b.WriteString(strings.Join(urls, "\n"))
	}

	return strings.TrimSpace(b.String())
}
