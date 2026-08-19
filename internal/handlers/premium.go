package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/quota"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "addpremium",
		As:          []string{"addprem"},
		Tags:        "owner",
		Description: "Tambah user premium",
		IsPrefix:    true,
		IsOwner:     true,
		SkipQuota:   true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			args := strings.Fields(m.Query)
			if len(args) == 0 {
				m.Reply(ctx, "Format: `.addpremium <nomor>` atau `.addpremium <nomor> <hari>`\n\nContoh:\n`.addpremium 628123456789` → permanent\n`.addpremium 628123456789 30` → 30 hari")
				return
			}

			phone := strings.TrimPrefix(args[0], "+")
			phone = strings.TrimPrefix(phone, "0")
			if !strings.HasPrefix(phone, "62") {
				phone = "62" + phone
			}

			targetJID := types.NewJID(phone, types.DefaultUserServer)

			days := 0
			if len(args) >= 2 {
				d, err := strconv.Atoi(args[1])
				if err != nil || d <= 0 {
					m.Reply(ctx, "Jumlah hari harus berupa angka positif.")
					return
				}
				days = d
			}

			qc := quota.Global()
			if qc == nil {
				m.Reply(ctx, "Sistem kuota belum aktif.")
				return
			}

			err := qc.AddPremium(ctx, targetJID.String(), m.Sender.String(), days)
			if err != nil {
				m.Reply(ctx, "Gagal menambahkan premium: "+err.Error())
				return
			}

			durasi := "permanent"
			if days > 0 {
				durasi = fmt.Sprintf("%d hari", days)
			}

			m.Reply(ctx, fmt.Sprintf("✅ User *%s* berhasil ditambahkan sebagai *Premium* (%s).", phone, durasi))

			// Kirim notifikasi ke user target
			notif := "🎉 *Selamat!* Akunmu telah di-upgrade ke *Premium*!\n\nKamu sekarang bisa menggunakan semua fitur tanpa batas harian.\nTerima kasih atas dukunganmu! 🙏"
			if days > 0 {
				notif = fmt.Sprintf("🎉 *Selamat!* Akunmu telah di-upgrade ke *Premium* selama *%d hari*!\n\nKamu sekarang bisa menggunakan semua fitur tanpa batas harian.\nTerima kasih atas dukunganmu! 🙏", days)
			}
			client.SendText(ctx, targetJID, notif, nil)
		},
	})

	commands.Register(&commands.Command{
		Name:        "delpremium",
		As:          []string{"delprem", "removepremium"},
		Tags:        "owner",
		Description: "Hapus user premium",
		IsPrefix:    true,
		IsOwner:     true,
		SkipQuota:   true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			args := strings.Fields(m.Query)
			if len(args) == 0 {
				m.Reply(ctx, "Format: `.delpremium <nomor>`\n\nContoh: `.delpremium 628123456789`")
				return
			}

			phone := strings.TrimPrefix(args[0], "+")
			phone = strings.TrimPrefix(phone, "0")
			if !strings.HasPrefix(phone, "62") {
				phone = "62" + phone
			}

			targetJID := types.NewJID(phone, types.DefaultUserServer)

			qc := quota.Global()
			if qc == nil {
				m.Reply(ctx, "Sistem kuota belum aktif.")
				return
			}

			err := qc.RemovePremium(ctx, targetJID.String())
			if err != nil {
				m.Reply(ctx, "Gagal menghapus premium: "+err.Error())
				return
			}

			m.Reply(ctx, fmt.Sprintf("✅ User *%s* sudah dihapus dari daftar *Premium*.", phone))
		},
	})

	commands.Register(&commands.Command{
		Name:        "listpremium",
		As:          []string{"listprem"},
		Tags:        "owner",
		Description: "Lihat daftar user premium",
		IsPrefix:    true,
		IsOwner:     true,
		SkipQuota:   true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			qc := quota.Global()
			if qc == nil {
				m.Reply(ctx, "Sistem kuota belum aktif.")
				return
			}

			users, err := qc.ListPremium(ctx)
			if err != nil {
				m.Reply(ctx, "Gagal mengambil daftar premium: "+err.Error())
				return
			}

			if len(users) == 0 {
				m.Reply(ctx, "Belum ada user premium.")
				return
			}

			var txt strings.Builder
			txt.WriteString(fmt.Sprintf("⭐ *Daftar User Premium* (%d)\n─────────────────\n\n", len(users)))

			for i, u := range users {
				jid, _ := types.ParseJID(u.JID)
				phone := jid.User
				expiry := "Permanent"
				if u.ExpiresAt != nil {
					expiry = u.ExpiresAt.Format("02 Jan 2006")
				}
				txt.WriteString(fmt.Sprintf("%d. *%s*\n   📅 Berlaku: %s\n\n", i+1, phone, expiry))
			}

			m.Reply(ctx, txt.String())
		},
	})
}
