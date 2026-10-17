package handlers

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func init() {
	commands.Register(&commands.Command{
		Name: "nhpdf", As: []string{"nhentaipdf"}, Tags: "nsfw", Description: "Unduh gallery sebagai PDF: <kode gallery>", IsPrefix: true, IsQuery: true, IsPrivate: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if strings.TrimSpace(cfg.BASEApiURL) == "" {
				m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
				return
			}
			code := strings.TrimSpace(m.Query)
			workCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			file, err := api.Shared(cfg.BASEApiURL, 10*time.Minute).NhentaiPDF(workCtx, code)
			if err != nil {
				m.Reply(ctx, "Gagal mengunduh PDF: "+err.Error())
				return
			}
			defer os.Remove(file.Name())
			defer file.Close()
			if _, err := client.SendDocumentFile(workCtx, m.From, file, "nhentai-"+code+".pdf", "Gallery "+code, m.ID); err != nil {
				m.Reply(ctx, "Gagal mengirim PDF: "+err.Error())
			}
		},
	})
}
