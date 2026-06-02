package pkg

import (
	"context"
	"fmt"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func instagramStalk(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	args := strings.Fields(m.Query)
	if len(args) == 0 {
		m.Reply(ctx, "Username Instagram-nya belum ada.")
		return
	}

	username := normalizeInstagramStalkUsername(args[0])
	if username == "" {
		m.Reply(ctx, "Username Instagram-nya belum valid.")
		return
	}

	m.Reply(ctx, "Tunggu Sebentar ya.")

	ap := api.New(cfg.BASEApiURL, 120*time.Second)
	res, err := ap.InstagramStalk(ctx, username, api.InstagramStalkOptions{})
	if err != nil {
		m.Reply(ctx, "Gagal: "+err.Error())
		return
	}

	caption := formatInstagramStalkCaption(res)
	if picURL := strings.TrimSpace(res.Profile.ProfilePicURL); picURL != "" {
		if buff, err := client.FetchBytes(picURL); err == nil {
			if _, err := client.SendImage(ctx, m.From, buff, caption, m.ID); err == nil {
				return
			}
		}
	}

	m.Reply(ctx, caption)
}

func normalizeInstagramStalkUsername(input string) string {
	value := strings.TrimSpace(input)
	if value == "" {
		return ""
	}

	if parsed, err := neturl.Parse(value); err == nil && parsed.Host != "" {
		host := strings.ToLower(parsed.Hostname())
		if host == "instagram.com" || host == "www.instagram.com" {
			parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			if len(parts) > 0 {
				value = parts[0]
			}
		}
	}

	value = strings.TrimLeft(strings.TrimSpace(value), "@")
	value = strings.Trim(value, "/")
	if instagramStalkReservedPath(value) || !instagramStalkValidUsername(value) {
		return ""
	}
	return value
}

func instagramStalkReservedPath(value string) bool {
	switch strings.ToLower(value) {
	case "p", "reel", "reels", "stories", "explore", "accounts", "direct":
		return true
	default:
		return false
	}
}

func instagramStalkValidUsername(value string) bool {
	if value == "" || len(value) > 30 {
		return false
	}

	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '.' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func formatInstagramStalkCaption(res *api.InstagramStalkResult) string {
	profile := res.Profile
	var b strings.Builder

	b.WriteString("*Instagram Stalk*\n")
	if profile.Username != "" {
		fmt.Fprintf(&b, "Username: @%s\n", profile.Username)
	}
	if profile.FullName != "" {
		fmt.Fprintf(&b, "Nama: %s\n", profile.FullName)
	}
	if profile.Biography != "" {
		fmt.Fprintf(&b, "Bio: %s\n", profile.Biography)
	}
	if profile.ExternalURL != "" {
		fmt.Fprintf(&b, "Link: %s\n", profile.ExternalURL)
	}

	fmt.Fprintf(&b, "Followers: %s\n", instagramStalkFormatNumber(profile.FollowersCount))
	fmt.Fprintf(&b, "Following: %s\n", instagramStalkFormatNumber(profile.FollowingCount))
	fmt.Fprintf(&b, "Posts: %s\n", instagramStalkFormatNumber(profile.PostsCount))
	fmt.Fprintf(&b, "Private: %s\n", instagramStalkBoolText(profile.IsPrivate))
	fmt.Fprintf(&b, "Verified: %s\n", instagramStalkBoolText(profile.IsVerified))
	fmt.Fprintf(&b, "Business: %s", instagramStalkBoolText(profile.IsBusinessAccount))

	if about := res.AboutAccount; about != nil {
		if about.DateJoined != "" || about.AccountBasedIn != "" || about.VerifiedSince != "" || about.SharedFollowersCount != nil {
			b.WriteString("\n\n*About Account*")
			if about.DateJoined != "" {
				fmt.Fprintf(&b, "\nBergabung: %s", about.DateJoined)
			}
			if about.AccountBasedIn != "" {
				fmt.Fprintf(&b, "\nLokasi akun: %s", about.AccountBasedIn)
			}
			if about.VerifiedSince != "" {
				fmt.Fprintf(&b, "\nVerified sejak: %s", about.VerifiedSince)
			}
			if about.SharedFollowersCount != nil {
				fmt.Fprintf(&b, "\nShared followers: %s", instagramStalkFormatNumber(*about.SharedFollowersCount))
			}
		}
	}

	if len(res.Warnings) > 0 {
		b.WriteString("\n\n*Warnings*")
		for _, warning := range res.Warnings {
			warning = strings.TrimSpace(warning)
			if warning != "" {
				fmt.Fprintf(&b, "\n- %s", warning)
			}
		}
	}

	text := strings.TrimSpace(b.String())
	if len(text) > 900 {
		text = text[:900] + "..."
	}
	return text
}

func instagramStalkBoolText(value bool) string {
	if value {
		return "Ya"
	}
	return "Tidak"
}

func instagramStalkFormatNumber(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}

	s := strconv.FormatInt(value, 10)
	if len(s) <= 3 {
		return sign + s
	}

	var b strings.Builder
	prefix := len(s) % 3
	if prefix == 0 {
		prefix = 3
	}
	b.WriteString(s[:prefix])
	for i := prefix; i < len(s); i += 3 {
		b.WriteByte('.')
		b.WriteString(s[i : i+3])
	}
	return sign + b.String()
}

func init() {
	commands.Register(&commands.Command{
		Name:     "instagramstalk",
		As:       []string{"igstalk", "igprofile", "stalkig"},
		Tags:     "stalker",
		IsQuery:  true,
		IsPrefix: true,
		Exec:     instagramStalk,
	})
}
