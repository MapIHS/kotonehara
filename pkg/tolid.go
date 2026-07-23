package pkg

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "tolid",
		As:          []string{"lid"},
		Tags:        "tools",
		Description: "Ubah nomor WhatsApp ke LID yang tersimpan",
		IsPrefix:    true,
		IsQuery:     true,
		Exec:        toLID,
	})
}

func toLID(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	pn, alreadyLID, err := parseLIDTarget(m.Query)
	if err != nil {
		_, _ = m.Reply(ctx, "Nomor tidak valid. Contoh: .tolid 6281234567890")
		return
	}
	if alreadyLID {
		_, _ = m.Reply(ctx, "Itu sudah LID:\n"+pn.String())
		return
	}

	lid, err := client.WA.Store.LIDs.GetLIDForPN(ctx, pn)
	if err != nil {
		_, _ = m.Reply(ctx, "Gagal mencari LID: "+err.Error())
		return
	}
	if lid.IsEmpty() {
		_, _ = m.Reply(ctx, "LID belum tersimpan untuk nomor "+pn.User+". Bot hanya dapat menampilkan mapping yang sudah diterima WhatsApp dari riwayat chat, pesan, atau sinkronisasi.")
		return
	}

	_, _ = m.Reply(ctx, fmt.Sprintf("*Nomor:* %s\n*LID:* %s", pn.String(), lid.ToNonAD().String()))
}

func parseLIDTarget(input string) (types.JID, bool, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	if jid, err := types.ParseJID(input); err == nil && !jid.IsEmpty() {
		if jid.Server == types.HiddenUserServer {
			return jid.ToNonAD(), true, nil
		}
		if jid.Server == types.DefaultUserServer {
			return jid.ToNonAD(), false, nil
		}
	}

	phone := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, input)
	if len(phone) < 7 || len(phone) > 15 {
		return types.JID{}, false, fmt.Errorf("invalid phone number")
	}
	return types.NewJID(phone, types.DefaultUserServer), false, nil
}
