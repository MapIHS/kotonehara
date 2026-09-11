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
				m.Reply(ctx, "Format: `.addpremium <nomor>` atau `.addpremium <nomor> <hari>`\nNomor internasional: kode negara + nomor, tanpa spasi. Awalan + atau 00 boleh dipakai.\n\nContoh:\n`.addpremium +14155552671` → permanent\n`.addpremium 447700900123 30` → 30 hari\n`.addpremium 08123456789 30` → nomor Indonesia, 30 hari")
				return
			}

			phone, err := normalizePremiumPhone(args[0])
			if err != nil {
				m.Reply(ctx, err.Error())
				return
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

			err = qc.AddPremium(ctx, targetJID.String(), m.Identity.StateJID(), days)
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
				m.Reply(ctx, "Format: `.delpremium <nomor>`\nGunakan kode negara + nomor tanpa spasi; awalan + atau 00 boleh dipakai.\n\nContoh: `.delpremium +14155552671`\nNomor Indonesia juga bisa memakai 08…")
				return
			}

			phone, err := normalizePremiumPhone(args[0])
			if err != nil {
				m.Reply(ctx, err.Error())
				return
			}

			targetJID := types.NewJID(phone, types.DefaultUserServer)

			qc := quota.Global()
			if qc == nil {
				m.Reply(ctx, "Sistem kuota belum aktif.")
				return
			}

			err = qc.RemovePremium(ctx, targetJID.String())
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

// normalizePremiumPhone preserves international country codes. Only an explicit
// local 08 prefix uses the Indonesian shorthand; bare 81/82/etc. must stay intact.
// This validates the number's syntax, not its allocation or WhatsApp registration.
func normalizePremiumPhone(input string) (string, error) {
	phone := strings.TrimSpace(input)
	switch {
	case strings.HasPrefix(phone, "+"):
		phone = strings.TrimPrefix(phone, "+")
	case strings.HasPrefix(phone, "00"):
		phone = strings.TrimPrefix(phone, "00")
	case strings.HasPrefix(phone, "08"):
		phone = "62" + phone[1:]
	}
	invalid := func() (string, error) {
		return "", fmt.Errorf("Nomor tidak valid. Gunakan kode negara + nomor (maksimal 15 digit), tanpa spasi atau tanda baca; contoh +14155552671 atau 447700900123. Nomor Indonesia boleh memakai 08…")
	}
	if len(phone) < 2 || len(phone) > 15 || phone[0] == '0' {
		return invalid()
	}
	for _, digit := range phone {
		if digit < '0' || digit > '9' {
			return invalid()
		}
	}
	return phone, nil
}
