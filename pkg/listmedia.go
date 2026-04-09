package pkg

import (
	"context"
	"fmt"
	"strconv"
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
				sb.WriteString(fmt.Sprintf("%d. [%s] Dari: %s\n   Waktu: %s\n\n", i+1, strings.ToUpper(item.MsgType), sender, item.CreatedAt))
			}
			sb.WriteString("Ketik *.getmedia <nomor>* untuk mengambil kembali medianya. Contoh: *.getmedia 1*")

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
				m.Reply(ctx, "Harap sertakan nomor urut pesannya. Contoh:\n*.getmedia 1*")
				return
			}

			target := strings.TrimSpace(m.Query)
			var targetID string

			idx, errParse := strconv.Atoi(target)
			if errParse == nil && idx > 0 && idx <= 10 {
				items, errDB := message.MsgStore.GetRecentMedia(10)
				if errDB == nil && len(items) >= idx {
					targetID = items[idx-1].ID
				} else {
					m.Reply(ctx, "Nomor indeks tidak valid atau media tidak ditemukan.")
					return
				}
			} else {
				targetID = target
			}

			msgProto := message.MsgStore.Get(targetID)
			if msgProto == nil {
				m.Reply(ctx, "Pesan tidak ditemukan di database.")
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
