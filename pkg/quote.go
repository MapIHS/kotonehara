package pkg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/quote"
	"github.com/MapIHS/kotonehara/internal/service/s3"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

var quoteColors = map[string]string{
	"default": "#FFFFFF",
	"dark":    "#1F2C34",
}

func init() {
	commands.Register(&commands.Command{
		Name:        "qc",
		As:          []string{"quote"},
		Tags:        "convert",
		Description: "Buat stiker quote dari pesan",
		IsPrefix:    true,
		ShowWait:    true,
		Exec:        quoteCmd,
	})
	commands.Register(&commands.Command{
		Name:        "fqc",
		As:          []string{"fquote", "fakequote"},
		Tags:        "convert",
		Description: "Buat stiker fake quote dari pesan",
		IsPrefix:    true,
		ShowWait:    true,
		Exec:        quoteCmd,
	})
}

func quoteCmd(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	if cfg.QuoteAPIURL == "" {
		m.Reply(ctx, "QUOTE_API_URL belum diatur, yaa.")
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	text := strings.TrimSpace(m.Query)
	// Handle colors
	bgColor := quoteColors["default"]
	parts := strings.SplitN(text, "|", 2)
	if len(parts) == 2 {
		c := strings.ToLower(strings.TrimSpace(parts[0]))
		if val, ok := quoteColors[c]; ok {
			bgColor = val
			text = strings.TrimSpace(parts[1])
		}
	} else if len(parts) == 1 {
		c := strings.ToLower(strings.TrimSpace(parts[0]))
		if val, ok := quoteColors[c]; ok {
			bgColor = val
			text = ""
		}
	}

	var qMsg *waE2E.Message
	var senderJID types.JID
	var senderName string
	var replyMsg *quote.ReplyMessage

	cmdName := strings.ToLower(m.Command)
	if len(cmdName) > 0 && (cmdName[0] == '.' || cmdName[0] == '!' || cmdName[0] == '/') {
		cmdName = cmdName[1:]
	}
	isFakeQuote := cmdName == "fqc" || cmdName == "fquote" || cmdName == "fakequote"

	if m.QuotedMsg != nil && text == "" {
		qMsg = m.QuotedMsg
		text = getMsgText(m.QuotedMsg)

		participant := ""
		if m.ContextInfo != nil {
			participant = m.ContextInfo.GetParticipant()
		}
		if participant != "" {
			senderJID, _ = types.ParseJID(participant)
		}
		if senderJID.IsEmpty() {
			senderJID = m.Sender
		}
		senderName = resolveContactName(opCtx, client, senderJID)
	} else if m.QuotedMsg != nil && text != "" && isFakeQuote {
		qMsg = m.QuotedMsg

		participant := ""
		if m.ContextInfo != nil {
			participant = m.ContextInfo.GetParticipant()
		}
		if participant != "" {
			senderJID, _ = types.ParseJID(participant)
		}
		if senderJID.IsEmpty() {
			senderJID = m.Sender
		}
		senderName = resolveContactName(opCtx, client, senderJID)
	} else if m.QuotedMsg != nil && text != "" && !isFakeQuote {
		qMsg = m.Message
		senderJID = m.Sender
		senderName = m.PushName

		nestedText := getMsgText(m.QuotedMsg)
		if nestedText != "" {
			nestedCleanText, nestedEntities := quote.ParseEntities(nestedText)

			participant := ""
			if m.ContextInfo != nil {
				participant = m.ContextInfo.GetParticipant()
			}
			var nestedJID types.JID
			if participant != "" {
				nestedJID, _ = types.ParseJID(participant)
			} else {
				nestedJID = m.Sender // fallback
			}

			nestedSenderName := resolveContactName(opCtx, client, nestedJID)
			if nestedSenderName == "" {
				nestedSenderName = jidToDisplayName(nestedJID)
			}

			replyMsg = &quote.ReplyMessage{
				Name:     nestedSenderName,
				Text:     nestedCleanText,
				Entities: nestedEntities,
			}
		}
	} else {
		qMsg = m.Message
		senderJID = m.Sender
		senderName = m.PushName
	}

	if text == "" && qMsg.GetImageMessage() == nil {
		m.Reply(ctx, "Usage:\n.qc <text>\n.qc <color> | <text>\n.qc <reply>\n.fqc <text> (fake quote)")
		return
	}

	// Parse WhatsApp formatting to Telegram entities
	cleanText, entities := quote.ParseEntities(text)

	// Resolve sender name fallback
	if senderName == "" {
		senderName = jidToDisplayName(senderJID)
	}

	// Prepare avatar
	avatarURL := ""
	pPic, err := client.WA.GetProfilePictureInfo(opCtx, senderJID, &whatsmeow.GetProfilePictureParams{})
	if err == nil && pPic != nil && pPic.URL != "" {
		avatarURL = pPic.URL
	} else {
		avatarURL = "https://telegra.ph/file/89c1638d9620584e6e140.png"
	}

	// Media handling
	var media *quote.Media
	if img := qMsg.GetImageMessage(); img != nil {
		data, err := client.WA.Download(opCtx, img)
		if err == nil && len(data) > 0 {
			mimeType := img.GetMimetype()
			if mimeType == "" {
				mimeType = "image/jpeg"
			}

			var publicURL string
			var upErr error

			if cfg.BASES3URL != "" {
				ext := ".jpg"
				if strings.Contains(mimeType, "png") {
					ext = ".png"
				}
				filename := fmt.Sprintf("quote-%d%s", time.Now().UnixNano(), ext)
				publicURL, upErr = s3.New(cfg.BASES3URL, 30*time.Second).Upload(filename, data)
			}

			if upErr == nil && publicURL != "" {
				media = &quote.Media{
					URL: publicURL,
				}
			} else if upErr != nil {
				fmt.Println("Gagal upload media ke S3:", upErr)
			}
		}
	}

	// Build payload and call Quote API
	payload := quote.Payload{
		Type:            "quote",
		Format:          "png",
		BackgroundColor: bgColor,
		Width:           512,
		Height:          512,
		Scale:           2,
		Messages: []quote.Message{
			{
				Entities: entities,
				Media:    media,
				Avatar:   true,
				From: quote.From{
					ID:   hashJID(senderJID.String()),
					Name: senderName,
					Photo: quote.Photo{
						URL: avatarURL,
					},
				},
				Text:         cleanText,
				ReplyMessage: replyMsg,
			},
		},
	}

	qc := quote.New(cfg.QuoteAPIURL, 30*time.Second)
	imgBytes, err := qc.Generate(opCtx, payload)
	if err != nil {
		m.Reply(ctx, fmt.Sprintf("Gagal membuat quote: %s", err))
		return
	}

	// Convert to sticker
	stcBytes, err := sticker.BuildSticker(opCtx, imgBytes, m.PushName, false, false)
	if err != nil {
		m.Reply(ctx, "Gagal membuat stiker.")
		return
	}

	_, _ = client.SendSticker(opCtx, m.From, stcBytes, false, false, m.ID)
}

func getMsgText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if conv := msg.GetConversation(); conv != "" {
		return conv
	}
	if ext := msg.GetExtendedTextMessage().GetText(); ext != "" {
		return ext
	}
	if img := msg.GetImageMessage().GetCaption(); img != "" {
		return img
	}
	if vid := msg.GetVideoMessage().GetCaption(); vid != "" {
		return vid
	}
	if doc := msg.GetDocumentMessage().GetCaption(); doc != "" {
		return doc
	}
	return ""
}

// resolveContactName tries to get a human-readable name for the given JID using
// whatsmeow's contact store. Falls back to extracting a readable name from the JID.
func resolveContactName(ctx context.Context, client *clients.Client, jid types.JID) string {
	if client == nil || client.WA == nil || client.WA.Store == nil || client.WA.Store.Contacts == nil {
		return jidToDisplayName(jid)
	}

	// Check if it's the bot itself (by primary ID or LID)
	if client.WA.Store.ID != nil && jid.User == client.WA.Store.ID.User {
		if client.WA.Store.PushName != "" {
			return client.WA.Store.PushName
		}
		return "Kotonehara"
	}
	if !client.WA.Store.LID.IsEmpty() && jid.User == client.WA.Store.LID.User {
		if client.WA.Store.PushName != "" {
			return client.WA.Store.PushName
		}
		return "Kotonehara"
	}

	contact, err := client.WA.Store.Contacts.GetContact(ctx, jid)
	if err == nil && contact.Found {
		if contact.PushName != "" {
			return contact.PushName
		}
		if contact.FullName != "" {
			return contact.FullName
		}
		if contact.FirstName != "" {
			return contact.FirstName
		}
		if contact.BusinessName != "" {
			return contact.BusinessName
		}
	}

	return jidToDisplayName(jid)
}

func jidToDisplayName(jid types.JID) string {
	if jid.IsEmpty() {
		return "Unknown"
	}

	user := jid.User
	if user == "" {
		return "Unknown"
	}

	if idx := strings.Index(user, ":"); idx > 0 {
		user = user[:idx]
	}

	return user
}

func hashJID(s string) int {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return int(h & 0x7fffffff)
}
