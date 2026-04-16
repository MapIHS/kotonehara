package pkg

import (
	"context"

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
			if m.QuotedMsg != nil {
				jidStr := ""
				if m.QuotedMsg.GetParticipant() != "" {
					jidStr = m.QuotedMsg.GetParticipant()
				} else if len(m.QuotedMsg.MentionedJID) > 0 {
					jidStr = m.QuotedMsg.MentionedJID[0]
				}

				jid, err := types.ParseJID(jidStr)
				if err != nil {
					m.Reply(ctx, "Failed to parse JID from quoted message")
					return
				}
				pp, err := client.WA.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{})
				if err != nil {
					m.Reply(ctx, err.Error())
					return
				}

				buff, err := client.FetchBytes(pp.URL)
				if err != nil {
					m.Reply(ctx, err.Error())
					return
				}
				_, err = client.SendImage(ctx, m.From, buff, "", m.ID)
			} else {
				m.Reply(ctx, "Kosong keknya")
			}
		},
	})
}
