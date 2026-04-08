package pkg

import (
	"context"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "q",
		Description: "Replies with the message quoted by the one you replied to.",
		Tags:        "main",
		IsPrefix:    true,
		As:          []string{"q"},
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if m.QuotedMsg == nil || m.QuotedMsg.GetQuotedMessage() == nil {
				m.Reply(ctx, "Sialahkan reply pesan yg ada reply an :3")
				return
			}

			bMsgId := m.QuotedMsg.GetStanzaID()
			if bMsgId == "" {
				m.Reply(ctx, "Pesan itu tidak meng-quote apapun.")
				return
			}

			bFullMsg := message.MsgStore.Get(bMsgId)
			if bFullMsg == nil {
				m.Reply(ctx, "Pesan aslinya sudah tidak ada di memori bot (terlalu lama atau bot baru di-restart).")
				return
			}

			ctxInfo := getContextInfo(bFullMsg)
			if ctxInfo == nil || ctxInfo.GetQuotedMessage() == nil {
				m.Reply(ctx, "Pesan itu tidak meng-quote apapun.")
				return
			}

			targetMsg := ctxInfo.GetQuotedMessage()

			if targetMsg.ProtocolMessage != nil {
				m.Reply(ctx, "Sorry, the quoted message could not be found.")
				return
			}

			forwardedMsg := proto.Clone(targetMsg).(*waE2E.Message)

			client.WA.SendMessage(ctx, m.From, forwardedMsg)
		},
	})
}

func getContextInfo(msg *waE2E.Message) *waE2E.ContextInfo {
	if msg == nil {
		return nil
	}
	if msg.ExtendedTextMessage != nil {
		return msg.ExtendedTextMessage.ContextInfo
	} else if msg.ImageMessage != nil {
		return msg.ImageMessage.ContextInfo
	} else if msg.VideoMessage != nil {
		return msg.VideoMessage.ContextInfo
	} else if msg.DocumentMessage != nil {
		return msg.DocumentMessage.ContextInfo
	} else if msg.AudioMessage != nil {
		return msg.AudioMessage.ContextInfo
	} else if msg.StickerMessage != nil {
		return msg.StickerMessage.ContextInfo
	} else if msg.LocationMessage != nil {
		return msg.LocationMessage.ContextInfo
	} else if msg.ContactMessage != nil {
		return msg.ContactMessage.ContextInfo
	} else if msg.ContactsArrayMessage != nil {
		return msg.ContactsArrayMessage.ContextInfo
	} else if msg.TemplateMessage != nil {
		return msg.TemplateMessage.ContextInfo
	} else if msg.ButtonsMessage != nil {
		return msg.ButtonsMessage.ContextInfo
	} else if msg.ListMessage != nil {
		return msg.ListMessage.ContextInfo
	} else if msg.PtvMessage != nil {
		return msg.PtvMessage.ContextInfo
	} else if msg.ViewOnceMessage != nil {
		return getContextInfo(msg.ViewOnceMessage.Message)
	} else if msg.ViewOnceMessageV2 != nil {
		return getContextInfo(msg.ViewOnceMessageV2.Message)
	} else if msg.ViewOnceMessageV2Extension != nil {
		return getContextInfo(msg.ViewOnceMessageV2Extension.Message)
	} else if msg.DocumentWithCaptionMessage != nil {
		return getContextInfo(msg.DocumentWithCaptionMessage.Message)
	} else if msg.EphemeralMessage != nil {
		return getContextInfo(msg.EphemeralMessage.Message)
	}
	return nil
}
