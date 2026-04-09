package pkg

import (
	"context"
	"fmt"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "listmedia",
		Description: "Menampilkan daftar media terakhir yang tersimpan di memori bot.",
		Tags:        "main",
		IsOwner:     true,
		IsPrefix:    true,
		As:          []string{"listmedia", "lm"},
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			items, err := message.MsgStore.GetRecentMedia(10)
			if err != nil {
				m.Reply(ctx, "Gagal mengambil daftar media: "+err.Error())
				return
			}

			if len(items) == 0 {
				m.Reply(ctx, "Belum ada media yang tersimpan sejak bot di-restart (atau tidak ada data).")
				return
			}

			var sb strings.Builder
			sb.WriteString("*Daftar Media Tersimpan Terakhir:*\n\n")
			for i, item := range items {
				sender := item.Sender
				if sender == "" {
					sender = "Unknown"
				}
				sb.WriteString(fmt.Sprintf("%d. [%s] ID: %s\n   Dari: @%s\n   Waktu: %s\n\n", i+1, strings.ToUpper(item.MsgType), item.ID, sender, item.CreatedAt))
			}
			sb.WriteString("Ketik *.getmedia <ID>* untuk mengambil kembali medianya.")

			m.Reply(ctx, sb.String())
		},
	})

	commands.Register(&commands.Command{
		Name:        "getmedia",
		Description: "Mengambil pesan/media dari ID yang tersimpan.",
		Tags:        "main",
		IsOwner:     true,
		IsPrefix:    true,
		As:          []string{"getmedia", "gm"},
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if m.Query == "" {
				m.Reply(ctx, "Harap sertakan ID pesannya. Contoh:\n*.getmedia 3EB0Cxxxx*")
				return
			}

			targetID := strings.TrimSpace(m.Query)
			msgProto := message.MsgStore.Get(targetID)
			if msgProto == nil {
				m.Reply(ctx, "Pesan tidak ditemukan di database atau ID salah.")
				return
			}

			forwardedMsg := proto.Clone(msgProto).(*waE2E.Message)

			_, err := client.WA.SendMessage(ctx, m.From, forwardedMsg)
			if err != nil {
				m.Reply(ctx, "Gagal mengirimkan pesan/media yang diminta.")
			}
		},
	})
}
