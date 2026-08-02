package handlers

import (
	"context"
	"fmt"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	commands.Register(&commands.Command{
		Name:     "delete",
		As:       []string{"delete", "del", "d"},
		Tags:     "admin",
		IsPrefix:    true,
		SkipQuota:   true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if m.IsBot {
				return
			}

			ctxInfo := m.ContextInfo
			if ctxInfo == nil || ctxInfo.GetStanzaID() == "" {
				_, _ = m.Reply(ctx, "Balas pesan yang mau dihapus, yaa.")
				return
			}

			isBotMessage := ctxInfo.GetParticipant() == client.BotJID()
			isGroupAdmin := false
			isBotAdmin := false

			if m.IsGroup {
				admins, err := client.GroupAdmins(ctx, m.From)
				if err == nil {
					senderStr := m.Sender.String()
					botStr := client.BotJID()
					for _, admin := range admins {
						if admin == senderStr {
							isGroupAdmin = true
						}
						if admin == botStr {
							isBotAdmin = true
						}
					}
				}
			}

			isAuthorized := m.IsOwner || (m.IsGroup && isGroupAdmin)
			if !isAuthorized {
				_, _ = m.Reply(ctx, "Perintah ini untuk owner atau admin grup, yaa.")
				return
			}

			if !isBotMessage {
				if !m.IsGroup {
					_, _ = m.Reply(ctx, "Hanya bisa menghapus pesan bot di chat pribadi.")
					return
				}
				if !isBotAdmin {
					_, _ = m.Reply(ctx, "Tolong jadikan bot sebagai admin dulu agar bisa menghapus pesan orang lain, yaa.")
					return
				}
			}

			fmt.Println("Deleting message with stanza ID:", ctxInfo.GetStanzaID())

			senderJID, _ := types.ParseJID(ctxInfo.GetParticipant())
			_, err := client.DeleteMessage(ctx, m.From, senderJID, ctxInfo.GetStanzaID())
			if err != nil {
				_, _ = m.Reply(ctx, "Gagal menghapus pesan.")
			}
		},
	})
}
