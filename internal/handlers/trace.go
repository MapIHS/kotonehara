package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func formatSeconds(seconds float64) string {
	mins := int(seconds) / 60
	secs := int(seconds) % 60
	return fmt.Sprintf("%02d:%02d", mins, secs)
}

func init() {
	commands.Register(&commands.Command{
		Name:        "trace",
		As:          []string{"whatanime", "wait"},
		Tags:        "tools",
		Description: "Mencari judul anime dari screenshot adegan",
		IsPrefix:    true,
		IsMedia:     true,
		ShowWait:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if m.Media == nil || m.IsVideo || m.IsQuotedVideo || m.IsGif || m.IsQuotedGif {
				m.Reply(ctx, "Kirim atau balas screenshot anime (bukan video/dokumen) dengan perintah ini untuk mencari judulnya.")
				return
			}

			opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			raw, err := client.WA.Download(opCtx, m.Media)
			if err != nil || len(raw) == 0 {
				m.Reply(ctx, "Gambarnya belum bisa diunduh.")
				return
			}

			apiClient := api.New(cfg.BASEApiURL, 60*time.Second)
			traceRes, err := apiClient.SearchTraceMoe(opCtx, raw)
			if err != nil {
				m.Reply(ctx, "Gagal mencari di trace.moe: "+err.Error())
				return
			}

			if len(traceRes.Result) == 0 {
				m.Reply(ctx, "Tidak menemukan hasil yang cocok untuk gambar ini.")
				return
			}

			// Get the best result
			best := traceRes.Result[0]

			// Fetch detailed info from AniList
			info, err := apiClient.FetchAnilistInfo(opCtx, best.Anilist.ID)
			var replyText string

			if err != nil || info == nil {
				// Fallback to basic info if AniList fetch fails
				title := best.Anilist.Title.Romaji
				if best.Anilist.Title.Native != "" {
					title += fmt.Sprintf(" (%s)", best.Anilist.Title.Native)
				}
				epStr := "?"
				if best.Episode != nil {
					epStr = fmt.Sprintf("%v", best.Episode)
				}
				timeStr := fmt.Sprintf("%s - %s", formatSeconds(best.From), formatSeconds(best.To))
				simStr := fmt.Sprintf("%.2f%%", best.Similarity*100)

				replyText = fmt.Sprintf(
					"🎬 *Hasil Pencarian trace.moe*\n"+
						"─────────────────\n"+
						"📌 *Judul*: %s\n"+
						"📺 *Episode*: %s\n"+
						"⏱️ *Waktu*: %s\n"+
						"💯 *Kecocokan*: %s\n",
					title, epStr, timeStr, simStr,
				)
			} else {
				// Full info
				title := info.Title.Romaji
				if info.Title.Native != "" && info.Title.Native != title {
					title += fmt.Sprintf(" (%s)", info.Title.Native)
				} else if info.Title.English != "" && info.Title.English != title {
					title += fmt.Sprintf(" (%s)", info.Title.English)
				}

				epStr := "?"
				if best.Episode != nil {
					epStr = fmt.Sprintf("%v", best.Episode)
				}
				timeStr := fmt.Sprintf("%s - %s", formatSeconds(best.From), formatSeconds(best.To))
				simStr := fmt.Sprintf("%.2f%%", best.Similarity*100)

				genres := strings.Join(info.Genres, ", ")
				if genres == "" {
					genres = "-"
				}

				seasonStr := "-"
				if info.Season != "" {
					seasonStr = fmt.Sprintf("%s %d", info.Season, info.SeasonYear)
				}

				// Clean basic HTML from description
				desc := info.Description
				desc = strings.ReplaceAll(desc, "<br>", "\n")
				desc = strings.ReplaceAll(desc, "<br/>", "\n")
				desc = strings.ReplaceAll(desc, "<br />", "\n")
				desc = strings.ReplaceAll(desc, "<i>", "_")
				desc = strings.ReplaceAll(desc, "</i>", "_")
				desc = strings.ReplaceAll(desc, "<b>", "*")
				desc = strings.ReplaceAll(desc, "</b>", "*")
				// Limit description length if too long
				if len(desc) > 300 {
					desc = desc[:300] + "..."
				}

				adultWarning := ""
				if best.Anilist.IsAdult {
					adultWarning = "\n⚠️ *PERINGATAN: Anime ini berlabel 18+ (NSFW)*\n"
				}

				replyText = fmt.Sprintf(
					"🎬 *Hasil Pencarian trace.moe*\n"+
						"─────────────────\n"+
						"📌 *Judul*: %s\n"+
						"📺 *Episode*: %s\n"+
						"⏱️ *Waktu*: %s\n"+
						"💯 *Kecocokan*: %s\n"+
						"─────────────────\n"+
						"📑 *Genre*: %s\n"+
						"✨ *Status*: %s\n"+
						"📅 *Musim*: %s\n"+
						"⭐ *Skor*: %d/100\n"+
						"🎞️ *Total Ep*: %d (%d mnt/ep)\n"+
						"🖼️ *Cover*: %s\n\n"+
						"📝 *Sinopsis*:\n%s%s",
					title, epStr, timeStr, simStr,
					genres, info.Status, seasonStr, info.MeanScore, info.Episodes, info.Duration,
					info.CoverImage.ExtraLarge, desc, adultWarning,
				)
			}

			// Try sending video preview first
			videoSent := false
			if best.Video != "" {
				// Muted video preview
				previewURL := best.Video
				if !strings.Contains(previewURL, "?") {
					previewURL += "?mute"
				} else {
					previewURL += "&mute"
				}
				// Download the preview
				reqVid, err := http.NewRequestWithContext(opCtx, http.MethodGet, previewURL, nil)
				if err == nil {
					resVid, err := apiClient.HTTP.Do(reqVid)
					if err == nil && resVid.StatusCode == http.StatusOK {
						defer resVid.Body.Close()
						vidRaw, _ := io.ReadAll(resVid.Body)
						if len(vidRaw) > 0 {
							// Send video preview
							_, _ = client.SendVideo(ctx, m.From, vidRaw, false, replyText, m.ID)
							videoSent = true
						}
					}
				}
			}

			// Fallback to Image if video failed or was empty
			if !videoSent {
				imageURL := ""
				if info != nil && info.BannerImage != "" {
					imageURL = info.BannerImage
				} else if info != nil && info.CoverImage.ExtraLarge != "" {
					imageURL = info.CoverImage.ExtraLarge
				}

				imageSent := false
				if imageURL != "" {
					reqImg, err := http.NewRequestWithContext(opCtx, http.MethodGet, imageURL, nil)
					if err == nil {
						resImg, err := apiClient.HTTP.Do(reqImg)
						if err == nil && resImg.StatusCode == http.StatusOK {
							defer resImg.Body.Close()
							imgRaw, _ := io.ReadAll(resImg.Body)
							if len(imgRaw) > 0 {
								// Send Image preview
								_, _ = client.SendImage(ctx, m.From, imgRaw, replyText, m.ID)
								imageSent = true
							}
						}
					}
				}

				if !imageSent {
					// Fallback to send text only
					m.Reply(ctx, replyText)
				}
			}
		},
	})
}
