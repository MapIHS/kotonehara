package handlers

import (
	"context"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/static"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "donasi",
		As:          []string{"donate", "sumbang"},
		Tags:        "main",
		Description: "Menampilkan QRIS untuk donasi",
		IsPrefix:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			caption := "Terima kasih atas dukunganmu! 🎉\n\nKamu bisa scan QRIS di atas menggunakan e-wallet atau m-banking favoritmu (GoPay, OVO, Dana, ShopeePay, BCA, dll).\nBerapapun nominalnya sangat berarti untuk kelangsungan bot ini."
			
			// Send the embedded QRIS image
			_, err := client.SendImage(ctx, m.From, static.QRISImage, caption, m.ID)
			if err != nil {
				m.Reply(ctx, "Maaf, terjadi kesalahan saat mengirim QRIS donasi.")
			}
		},
	})
}
