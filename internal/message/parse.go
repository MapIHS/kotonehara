package message

import (
	"context"
	"os"
	"strings"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

type Parser struct {
	Client WAClient
	Owners []string
}

func NewParser(c WAClient, owners []string) *Parser {
	if len(owners) == 0 {
		if o := strings.TrimSpace(os.Getenv("Owner_Number")); o != "" {
			owners = append(owners, o)
		}
		owners = append(owners, "161469647855806@lid")
	}
	return &Parser{Client: c, Owners: owners}
}

func (p *Parser) Parse(ctx context.Context, mess *events.Message) *Message {
	senderNonAD := mess.Info.Sender.ToNonAD().String()
	isOwner := false

	for _, own := range p.Owners {
		if strings.Contains(own, senderNonAD) {
			isOwner = true
			break
		}
	}

	body := extractBody(mess)
	cmd, query := splitCommand(body)
	var quotedMsg *waE2E.Message
	var quotedInfo *waE2E.ContextInfo
	ext := mess.Message.GetExtendedTextMessage()
	if ext != nil {
		quotedInfo = ext.GetContextInfo()
		if quotedInfo != nil {
			quotedMsg = quotedInfo.GetQuotedMessage()
		}
	}

	media := pickMedia(quotedMsg, mess)

	isImage := mess.Message.GetImageMessage() != nil
	isVideo := mess.Message.GetVideoMessage() != nil

	isQuotedImage := quotedMsg != nil && quotedMsg.GetImageMessage() != nil
	isQuotedVideo := quotedMsg != nil && quotedMsg.GetVideoMessage() != nil
	isQuotedSticker := quotedMsg != nil && quotedMsg.GetStickerMessage() != nil

	isAdmin := false
	isBotAdmin := false
	if mess.Info.IsGroup {
		admins, err := p.Client.GroupAdmins(ctx, mess.Info.Chat)
		if err == nil && len(admins) > 0 {
			senderStr := mess.Info.Sender.String()
			botStr := p.Client.BotJID()
			for _, a := range admins {
				if a == senderStr {
					isAdmin = true
				}
				if a == botStr {
					isBotAdmin = true
				}
				if isAdmin && isBotAdmin {
					break
				}
			}
		}
	}

	replyFn := func(ctx context.Context, text string, opts ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		return p.Client.SendText(ctx, mess.Info.Chat, text, &waE2E.ContextInfo{
			StanzaID:      &mess.Info.ID,
			Participant:   proto.String(mess.Info.Sender.String()),
			QuotedMessage: mess.Message,
		}, opts...)
	}

	return &Message{
		From:        mess.Info.Chat,
		Sender:      mess.Info.Sender,
		PushName:    mess.Info.PushName,
		OwnerNumber: p.Owners,

		IsOwner: isOwner,
		IsBot:   mess.Info.IsFromMe,
		IsGroup: mess.Info.IsGroup,

		Body:    body,
		Command: cmd,
		Query:   query,

		Media:    media,
		Message:  mess.Message,
		StanzaID: mess.Info.ID,

		IsImage: isImage,
		IsVideo: isVideo,

		IsAdmin:    isAdmin,
		IsBotAdmin: isBotAdmin,

		QuotedMsg: quotedInfo,
		ID: &waE2E.ContextInfo{
			StanzaID:      &mess.Info.ID,
			Participant:   proto.String(mess.Info.Sender.String()),
			QuotedMessage: mess.Message,
		},

		IsQuotedImage:   isQuotedImage,
		IsQuotedVideo:   isQuotedVideo,
		IsQuotedSticker: isQuotedSticker,

		Reply: replyFn,
	}
}

func extractBody(mess *events.Message) string {
	if s := mess.Message.GetExtendedTextMessage().GetText(); s != "" {
		return s
	}
	if s := mess.Message.GetImageMessage().GetCaption(); s != "" {
		return s
	}
	if s := mess.Message.GetVideoMessage().GetCaption(); s != "" {
		return s
	}
	if s := mess.Message.GetConversation(); s != "" {
		return s
	}
	return ""
}

func splitCommand(body string) (cmd, query string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ""
	}
	parts := strings.Fields(body)
	cmd = strings.ToLower(parts[0])
	if len(parts) > 1 {
		query = strings.Join(parts[1:], " ")
	}
	return cmd, query
}

func pickMedia(quoted *waE2E.Message, mess *events.Message) whatsmeow.DownloadableMessage {
	if quoted != nil {
		if msg := quoted.GetImageMessage(); msg != nil {
			return msg
		}
		if msg := quoted.GetVideoMessage(); msg != nil {
			return msg
		}
		if msg := quoted.GetStickerMessage(); msg != nil {
			return msg
		}
	}

	if msg := mess.Message.GetImageMessage(); msg != nil {
		return msg
	}
	if msg := mess.Message.GetVideoMessage(); msg != nil {
		return msg
	}
	return nil
}
