package handlers

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/identity"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/menfess"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	for _, spec := range []struct {
		name, description string
		skipQuota         bool
	}{
		{"menfess", "Undangan anonim: <nomor> | <nama samaran> | <pesan>", false},
		{"terimamenfess", "Setujui undangan: <kode>", true},
		{"balas", "Balas Menfess: <kode> | <pesan>", false},
		{"stopmenfess", "Tutup sesi atau tolak undangan: [kode]", true},
		{"blokmenfess", "Tutup sesi dan blokir semua undangan Menfess", true},
		{"bukamenfess", "Izinkan kembali undangan Menfess", true},
	} {
		commands.Register(&commands.Command{Name: spec.name, Tags: "menfess", Description: spec.description, IsPrefix: true, IsPrivate: true, SkipQuota: spec.skipQuota,
			Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
				handleMenfess(ctx, client, m, cfg.Prefix, spec.name)
			},
		})
	}
}

var menfessPhone = regexp.MustCompile(`^\+?[1-9][0-9]{6,14}$`)

func parseMenfessInvite(query string) (types.JID, string, string, error) {
	fields := strings.SplitN(query, "|", 3)
	if len(fields) != 3 || !menfessPhone.MatchString(strings.TrimSpace(fields[0])) {
		return types.EmptyJID, "", "", fmt.Errorf("Gunakan menfess <nomor internasional> | <nama samaran> | <pesan>. Contoh nomor: 6281234567890.")
	}
	alias, text := strings.TrimSpace(fields[1]), strings.TrimSpace(fields[2])
	if err := menfess.ValidateText(text); err != nil {
		return types.EmptyJID, "", "", err
	}
	return types.NewJID(strings.TrimPrefix(strings.TrimSpace(fields[0]), "+"), types.DefaultUserServer), alias, text, nil
}

type menfessClient interface {
	ResolveIdentity(context.Context, types.JID, types.JID) (identity.Identity, error)
	BotIdentity() identity.Identity
	SendText(context.Context, types.JID, string, *waE2E.ContextInfo, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
}

func handleMenfess(ctx context.Context, client menfessClient, m *message.Message, prefix, action string) {
	if m.IsGroup {
		m.Reply(ctx, "Menfess hanya tersedia di chat pribadi bot.")
		return
	}
	service := menfess.Default()
	if service == nil {
		m.Reply(ctx, "Menfess belum tersedia.")
		return
	}
	if action == "menfess" && strings.TrimSpace(m.Query) == "" {
		m.Reply(ctx, "Kirim undangan anonim dengan "+prefix+"menfess <nomor> | <nama samaran> | <pesan>.\nPesan dibuka setelah penerima menyetujui undangan.\nUndangan berlaku 15 menit; sesi berlaku 24 jam. Nomor disembunyikan dari lawan bicara, tetapi diketahui bot.\nTutup dengan "+prefix+"stopmenfess; blokir undangan dengan "+prefix+"blokmenfess.")
		return
	}
	workCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	id, err := client.ResolveIdentity(workCtx, m.Sender, m.SenderAlt)
	if err != nil || id.PN.IsEmpty() {
		m.Reply(ctx, "Identitas nomor WhatsApp belum dapat dikenali. Coba lagi setelah sinkronisasi kontak.")
		return
	}
	actor := id.PN.ToNonAD().String()
	fail := func(err error) {
		if menfess.IsRejection(err) {
			m.Reply(ctx, err.Error())
			return
		}
		log.Printf("menfess %s: %v", action, err)
		m.Reply(ctx, "Menfess belum dapat diproses. Coba lagi nanti.")
	}
	// Always send a fresh text message without quotes, mentions, contact cards,
	// forwarding metadata, or the sender's original ContextInfo.
	send := func(to, text string) error {
		jid, err := types.ParseJID(to)
		if err != nil {
			return err
		}
		_, err = client.SendText(workCtx, jid, text, nil)
		return err
	}
	switch action {
	case "menfess":
		target, alias, text, err := parseMenfessInvite(m.Query)
		if err != nil {
			m.Reply(ctx, err.Error())
			return
		}
		if client.BotIdentity().Matches(target) {
			m.Reply(ctx, "Tujuan Menfess tidak boleh nomor bot.")
			return
		}
		if err := menfess.ValidateAlias(alias); err != nil {
			m.Reply(ctx, err.Error())
			return
		}
		session, err := service.Invite(workCtx, actor, target.String(), alias, text)
		if err != nil {
			if menfess.IsRejection(err) {
				fail(err)
			} else if actor == target.String() {
				m.Reply(ctx, "Nomor tujuan tidak boleh nomor sendiri.")
			} else {
				fail(err)
			}
			return
		}
		invitation := fmt.Sprintf("Undangan Menfess anonim dari %s.\nKode: %s\n\nPesan belum dibuka. Undangan berlaku 15 menit.\nTerima: %sterimamenfess %s\nTolak: %sstopmenfess %s\nBlokir semua undangan: %sblokmenfess\n\nNomor peserta disembunyikan dari lawan bicara, tetapi diketahui bot.", alias, session.Code, prefix, session.Code, prefix, session.Code, prefix)
		if err := send(session.Recipient, invitation); err != nil {
			// A failed delivery must not leave the sender stuck in a live session.
			cleanupCtx, cleanupCancel := context.WithTimeout(ctx, 5*time.Second)
			closeErr := service.Stop(cleanupCtx, actor, session.Code)
			cleanupCancel()
			if closeErr != nil {
				log.Printf("menfess close failed invitation: %v", closeErr)
			}
			m.Reply(ctx, "Undangan gagal dikirim. Periksa nomor tujuan lalu coba lagi.")
			return
		}
		m.Reply(ctx, "Undangan terkirim. Kode: "+session.Code+". Tunggu penerima menyetujui dalam 15 menit.\nBatalkan: "+prefix+"stopmenfess "+session.Code)
	case "terimamenfess":
		relay, err := service.Accept(workCtx, actor, m.Query)
		if err != nil {
			fail(err)
			return
		}
		// Send the initial message only to the consenting recipient.
		deliveryErr := send(relay.To, menfessRelayText(relay, prefix))
		if err := send(relay.Notify, "Undangan Menfess "+relay.Code+" sudah diterima. Sesi berlaku 24 jam.\nKirim pesan: "+prefix+"balas "+relay.Code+" | <pesan>"); err != nil {
			log.Printf("menfess consent notification failed: %v", err)
		}
		if deliveryErr != nil {
			m.Reply(ctx, "Sesi sudah diterima, tetapi pesan awal gagal dikirim. Minta pengirim mengirim ulang dengan command balas.")
			return
		}
	case "balas":
		fields := strings.SplitN(m.Query, "|", 2)
		if len(fields) != 2 {
			m.Reply(ctx, "Gunakan "+prefix+"balas <kode> | <pesan>.")
			return
		}
		if err := menfess.ValidateText(strings.TrimSpace(fields[1])); err != nil {
			m.Reply(ctx, err.Error())
			return
		}
		relay, err := service.Reply(workCtx, actor, fields[0], fields[1])
		if err != nil {
			fail(err)
			return
		}
		if err := send(relay.To, menfessRelayText(relay, prefix)); err != nil {
			m.Reply(ctx, "Balasan gagal dikirim. Coba lagi beberapa saat.")
			return
		}
		m.Reply(ctx, "Balasan Menfess terkirim.")
	case "stopmenfess":
		if err := service.Stop(workCtx, actor, m.Query); err != nil {
			fail(err)
			return
		}
		m.Reply(ctx, "Undangan atau sesi Menfess sudah ditutup.")
	case "blokmenfess", "bukamenfess":
		blocked := action == "blokmenfess"
		if err := service.Block(workCtx, actor, blocked); err != nil {
			fail(err)
			return
		}
		if blocked {
			m.Reply(ctx, "Semua undangan Menfess diblokir dan sesi ditutup. Izinkan kembali dengan "+prefix+"bukamenfess.")
		} else {
			m.Reply(ctx, "Kamu dapat menerima undangan Menfess lagi. Sesi sebelumnya tetap ditutup.")
		}
	}
}

func menfessRelayText(relay menfess.Relay, prefix string) string {
	return fmt.Sprintf("Menfess · %s\nDari: %s\n\n%s\n\nBalas: %sbalas %s | <pesan>\nTutup: %sstopmenfess %s\nBlokir: %sblokmenfess", relay.Code, relay.Alias, relay.Text, prefix, relay.Code, prefix, relay.Code, prefix)
}
