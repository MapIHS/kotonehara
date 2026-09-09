package handlers

import (
	"context"
	"fmt"
	"log"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	commands.Register(&commands.Command{
		Name:      "delete",
		As:        []string{"delete", "del", "d"},
		Tags:      "admin",
		IsPrefix:  true,
		SkipQuota: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if m.IsBot {
				return
			}

			ctxInfo := m.ContextInfo
			if ctxInfo == nil || ctxInfo.GetStanzaID() == "" {
				_, _ = m.Reply(ctx, "Balas pesan yang mau dihapus, yaa.")
				return
			}

			var quotedSender types.JID
			participant := ctxInfo.GetParticipant()
			if participant != "" {
				var err error
				quotedSender, err = types.ParseJID(participant)
				if err != nil || quotedSender.IsEmpty() {
					_, _ = m.Reply(ctx, "Identitas pengirim pesan tidak valid.")
					return
				}
			}
			isBotMessage := !m.IsGroup && participant == ""
			if !quotedSender.IsEmpty() {
				var err error
				isBotMessage, err = client.SameUser(ctx, quotedSender, client.BotIdentity())
				if err != nil {
					log.Printf("resolve quoted sender: %v", err)
					_, _ = m.Reply(ctx, "Gagal memverifikasi pengirim pesan.")
					return
				}
			}
			isGroupAdmin := false
			isBotAdmin := false

			if m.IsGroup {
				admins, err := client.GroupAdmins(ctx, m.From)
				if err == nil {
					senderAliases := make(map[string]struct{})
					for _, alias := range m.Identity.AliasStrings() {
						senderAliases[alias] = struct{}{}
					}
					botIdentity := client.BotIdentity()
					for _, admin := range admins {
						if _, ok := senderAliases[admin]; ok {
							isGroupAdmin = true
						}
						if botIdentity.MatchesString(admin) {
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

			_, err := client.DeleteMessage(ctx, m.From, quotedSender, ctxInfo.GetStanzaID())
			if err != nil {
				_, _ = m.Reply(ctx, "Gagal menghapus pesan.")
			}
		},
	})
}
