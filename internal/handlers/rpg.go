package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/identity"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/rpg"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	commands.Register(&commands.Command{Name: "rpg", Tags: "game", Description: "HARA: Gema Arunika — profil, battle web, koleksi dan gacha", IsPrefix: true, SkipQuota: true, Exec: func(ctx context.Context, c *clients.Client, m *message.Message, cfg config.Config) {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		handleRPG(ctx, m, rpg.Default(), cfg.RPGPublicURL, func(to types.JID, body string) error { _, err := c.SendText(ctx, to, body, nil); return err })
	}})
}
func handleRPG(ctx context.Context, m *message.Message, s *rpg.Service, publicURL string, sendPrivate func(types.JID, string) error) {
	reply := func(text string) { _, _ = m.Reply(ctx, text) }
	if s == nil {
		reply("RPG belum aktif. Pemilik bot perlu mengatur koneksi RPG di VPS terlebih dahulu.")
		return
	}
	id := m.Identity
	if len(id.Aliases()) == 0 {
		id = identity.New(m.Sender, m.SenderAlt)
	}
	p, err := s.EnsurePlayer(ctx, id.AliasStrings(), m.PushName)
	if err != nil {
		reply(rpgError(err))
		return
	}
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(m.Query)))
	sub := ""
	if len(parts) > 0 {
		sub = parts[0]
	}
	switch sub {
	case "", "mulai", "jelajah", "lanjut":
		ticket, err := s.IssueTicket(ctx, p.ID)
		if err != nil {
			reply(rpgError(err))
			return
		}
		text := rpgSummary(p, s.Catalog()) + "\n\n🗺️ Buka game:\n" + strings.TrimRight(publicURL, "/") + "/rpg/#ticket=" + ticket + "\n\nLink pribadi ini berlaku 2 menit dan hanya sekali pakai. Jangan bagikan. Battle dan koleksi disimpan di akunmu."
		if err = sendPrivate(m.Sender.ToNonAD(), text); err != nil {
			log.Printf("RPG private link delivery failed: %v", err)
			reply("Link game gagal dikirim ke chat pribadi. Coba kirim .rpg lanjut langsung ke bot.")
			return
		}
		if m.IsGroup {
			reply("Link RPG sudah dikirim ke chat pribadimu.")
		}
	case "profil", "tim":
		reply(rpgSummary(p, s.Catalog()) + "\n\n.rpg lanjut — buka peta, battle, dan atur tim")
	case "gacha":
		if len(parts) != 2 || (parts[1] != "1" && parts[1] != "10") {
			reply("Gunakan .rpg gacha 1 atau .rpg gacha 10. Biaya 160 Embun Bintang per tarikan.")
			return
		}
		if m.StanzaID == "" {
			reply("ID pesan tidak tersedia. Kirim ulang command dalam pesan baru.")
			return
		}
		count := 1
		if parts[1] == "10" {
			count = 10
		}
		hash := sha256.Sum256([]byte(m.From.ToNonAD().String() + ":" + m.StanzaID))
		request := "wa:" + hex.EncodeToString(hash[:])
		out, err := s.Summon(ctx, p.ID, request, rpg.BannerID, count)
		if err != nil {
			reply(rpgError(err))
			return
		}
		lines := []string{"✨ *Hasil pemanggilan*"}
		for _, pull := range out.Results {
			suffix := " · baru"
			if pull.Duplicate {
				suffix = " · duplikat"
			}
			lines = append(lines, strings.Repeat("★", pull.Character.Rarity)+" "+pull.Character.Name+suffix)
		}
		lines = append(lines, fmt.Sprintf("\nEmbun: %d · Pity ★5: %d/80", out.Profile.Shards, out.Profile.Pity5))
		reply(strings.Join(lines, "\n"))
	case "peluang":
		reply("*Peluang dasar:* ★1 40% · ★2 30% · ★3 20% · ★4 8% · ★5 2%.\n★4+ paling lambat tarikan ke-10; ★5 ke-80. Mulai tarikan ke-61 peluang ★5 naik 5 poin persentase tiap tarikan. ★5 mereset dua pity.\n★5: 50% karakter unggulan; setelah gagal, ★5 berikutnya pasti unggulan.\n1 tarikan = 160 Embun Bintang. Tanpa pembelian uang asli.")
	case "riwayat":
		history, err := s.History(ctx, p.ID)
		if err != nil {
			reply(rpgError(err))
			return
		}
		if len(history) == 0 {
			reply("Belum ada riwayat pemanggilan.")
			return
		}
		lines := []string{"*Pemanggilan terakhir*"}
		shown := 0
		for _, entry := range history {
			for i := len(entry.Results) - 1; i >= 0; i-- {
				pull := entry.Results[i]
				lines = append(lines, strings.Repeat("★", pull.Character.Rarity)+" "+pull.Character.Name)
				shown++
				if shown == 10 {
					break
				}
			}
			if shown == 10 {
				break
			}
		}
		reply(strings.Join(lines, "\n"))
	default:
		reply("*HARA: Gema Arunika*\n.rpg mulai — buat profil dan buka game\n.rpg profil — saldo dan progres\n.rpg lanjut — link pribadi untuk bermain\n.rpg tim — lihat tim\n.rpg gacha 1 / 10 — panggil karakter\n.rpg peluang — aturan pemanggilan\n.rpg riwayat — hasil terakhir")
	}
}
func rpgSummary(p rpg.Profile, c rpg.Catalog) string {
	names := []string{}
	for _, id := range p.Party {
		for _, ch := range c.Characters {
			if ch.ID == id {
				names = append(names, ch.Name)
				break
			}
		}
	}
	return fmt.Sprintf("🌄 *HARA: Gema Arunika*\nPenjaga: %s\nJalur terbuka: level %d / 999\nTim: %s\nKarakter: %d / 60\nEmbun: %d · Koin: %d · Debu: %d\nPity ★5: %d/80 · ★4+: %d/10", p.Name, p.Unlocked+1, strings.Join(names, " · "), len(p.Collection), p.Shards, p.Coins, p.Dust, p.Pity5, p.Pity4)
}
func rpgError(err error) string {
	var e *rpg.Error
	if errors.As(err, &e) {
		return e.Message
	}
	log.Printf("RPG bot command failed: %v", err)
	return "RPG belum bisa diproses. Coba lagi sebentar."
}
