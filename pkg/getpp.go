package pkg

import (
	"context"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	commands.Register(&commands.Command{
		Name:     "getpp",
		Tags:     "main",
		As:       []string{"getpp", "getprofilepicture"},
		IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			var targetJID types.JID

			if m.QuotedMsg != nil {
				if ext := m.Message.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil {
					participant := ext.GetContextInfo().GetParticipant()
					if participant != "" {
						targetJID, _ = types.ParseJID(participant)
					}
				}
			} else if m.ID != nil && len(m.ID.MentionedJID) > 0 {
				targetJID, _ = types.ParseJID(m.ID.MentionedJID[0])
			} else if m.Query != "" {
				user := strings.TrimSpace(m.Query)
				user = strings.ReplaceAll(user, "@", "")
				user = strings.ReplaceAll(user, " ", "")
				user = strings.ReplaceAll(user, "-", "")
				targetJID = types.NewJID(user, "s.whatsapp.net")
			}

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
