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

// An explicit flag keeps ordinary search words such as "safe" or "all" intact.
func parsePixivQuery(raw string) (string, string, error) {
	fields := strings.Fields(raw)
	query := make([]string, 0, len(fields))
	mode, seen := "all", false
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if field != "--mode" && !strings.HasPrefix(field, "--mode=") {
			query = append(query, field)
			continue
		}
		if seen {
			return "", "", fmt.Errorf("pilih satu --mode: safe, r18, atau all")
		}
		seen = true
		if field == "--mode" {
			i++
			if i >= len(fields) {
				return "", "", fmt.Errorf("isi --mode dengan safe, r18, atau all")
			}
			mode = fields[i]
		} else {
			mode = strings.TrimPrefix(field, "--mode=")
		}
		if mode != "safe" && mode != "r18" && mode != "all" {
			return "", "", fmt.Errorf("mode Pixiv harus safe, r18, atau all")
		}
	}
	if len(query) == 0 {
		return "", "", fmt.Errorf("masukkan kata kunci atau ID/URL profil; opsi: --mode safe|r18|all")
	}
	return strings.Join(query, " "), mode, nil
}

func profileText(raw string) string {
	text := []rune(strings.TrimSpace(raw))
	if len(text) > 700 {
		return string(text[:700]) + "…"
	}
	return string(text)
}

func formatPinterestProfile(result *api.PinterestUserResponse) string {
	user := result.User
	var b strings.Builder
	fmt.Fprintf(&b, "📌 *%s* (@%s)\nhttps://www.pinterest.com/%s/", profileText(user.FullName), user.Username, url.PathEscape(user.Username))
	if bio := profileText(user.Bio); bio != "" {
		fmt.Fprintf(&b, "\n\n%s", bio)
	}
	for _, stat := range []struct {
		label string
		value *int
	}{
		{"Pengikut", user.Followers}, {"Mengikuti", user.Following}, {"Pin", user.PinsCount}, {"Board", user.BoardsCount},
	} {
		if stat.value != nil {
			fmt.Fprintf(&b, "\n%s: %d", stat.label, *stat.value)
		}
	}
	if len(result.Pins) == 0 {
		b.WriteString("\n\nTidak ada pin yang ditampilkan.")
	}
	for i, pin := range result.Pins {
		if i == 5 {
			break
		}
		fmt.Fprintf(&b, "\n\n%d. %s\n%s", i+1, profileText(pin.Title), pin.Link)
	}
	return b.String()
}

func formatPixivProfile(result *api.PixivUserResponse, mode string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🎨 *%s*\nhttps://www.pixiv.net/users/%s\nMode: %s", profileText(result.User.Name), url.PathEscape(result.User.ID), mode)
	if comment := profileText(result.User.Comment); comment != "" {
		fmt.Fprintf(&b, "\n\n%s", comment)
	}
	if len(result.Artworks) == 0 {
		b.WriteString("\n\nTidak ada artwork untuk mode ini.")
	}
	for i, artwork := range result.Artworks {
		if i == 5 {
			break
		}
		fmt.Fprintf(&b, "\n\n%d. %s\nhttps://www.pixiv.net/artworks/%s", i+1, profileText(artwork.Title), url.PathEscape(artwork.ID))
	}
	return b.String()
}

func init() {
	commands.Register(&commands.Command{
		Name: "pinuser", As: []string{"pinterestuser"}, Tags: "search", Description: "Profil Pinterest: <username atau URL>", IsPrefix: true, IsQuery: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if strings.TrimSpace(cfg.BASEApiURL) == "" {
				m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
				return
			}
			result, err := api.Shared(cfg.BASEApiURL, 60*time.Second).PinterestUser(ctx, strings.TrimSpace(m.Query))
			if err != nil {
				m.Reply(ctx, "Gagal mengambil profil Pinterest: "+err.Error())
				return
			}
			m.Reply(ctx, formatPinterestProfile(result))
		},
	})
	commands.Register(&commands.Command{
		Name: "pixivuser", As: []string{"pxuser"}, Tags: "search", Description: "Profil Pixiv: <ID atau URL> [--mode safe|r18|all]", IsPrefix: true, IsQuery: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			target, mode, err := parsePixivQuery(m.Query)
			if err != nil {
				m.Reply(ctx, err.Error())
				return
			}
			if strings.TrimSpace(cfg.BASEApiURL) == "" {
				m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
				return
			}
			result, err := api.Shared(cfg.BASEApiURL, 60*time.Second).PixivUser(ctx, target, mode)
			if err != nil {
				m.Reply(ctx, "Gagal mengambil profil Pixiv: "+err.Error())
				return
			}
			m.Reply(ctx, formatPixivProfile(result, mode))
		},
	})
}
