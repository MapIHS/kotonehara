package handlers

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/identity"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	commands.Register(&commands.Command{
		Name:      "getpp",
		Tags:      "main",
		As:        []string{"getprofilepicture"},
		IsPrefix:  true,
		SkipQuota: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			var targetJID types.JID
			var parseErr error

			if m.QuotedMsg != nil {
				if ext := m.Message.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil {
					participant := ext.GetContextInfo().GetParticipant()
					if participant != "" {
						targetJID, parseErr = types.ParseJID(participant)
					}
				}
			} else if m.ID != nil && len(m.ID.MentionedJID) > 0 {
				targetJID, parseErr = types.ParseJID(m.ID.MentionedJID[0])
			} else if m.Query != "" {
				targetJID, parseErr = parseProfileTarget(m.Query)
			}

			if parseErr != nil {
				m.Reply(ctx, "JID atau nomor target tidak valid.")
				return
			}
			targetJID = identity.Normalize(targetJID)
			if targetJID.IsEmpty() {
				m.Reply(ctx, "Balas pesan orangnya, tag, atau masukkan nomornya untuk mendapatkan profile picture.")
				return
			}

			pp, err := client.WA.GetProfilePictureInfo(ctx, targetJID, &whatsmeow.GetProfilePictureParams{})
			if err != nil {
				m.Reply(ctx, "Gagal mendapatkan profile picture (mungkin diprivasi): "+err.Error())
				return
			}

			buff, err := client.FetchBytes(pp.URL)
			if err != nil {
				m.Reply(ctx, "Gagal mengunduh gambar: "+err.Error())
				return
			}
			_, err = client.SendImage(ctx, m.From, buff, "", m.ID)
			if err != nil {
				m.Reply(ctx, "Gagal mengirim gambar: "+err.Error())
			}
		},
	})
}

func parseProfileTarget(input string) (types.JID, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	if strings.Contains(input, "@") {
		return identity.ParseUser(input)
	}

	phone := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, input)
	if len(phone) < 7 || len(phone) > 15 {
		return types.EmptyJID, fmt.Errorf("invalid phone number")
	}
	return types.NewJID(phone, types.DefaultUserServer), nil
}
