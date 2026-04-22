package message

import (
	"context"
	"strings"

	"github.com/MapIHS/kotonehara/internal/infra/config"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type Parser struct {
	Client WAClient
	log    waLog.Logger
	cfg    config.Config
}

func NewParser(c WAClient, cfg config.Config) *Parser {
	return &Parser{
		Client: c,
		log:    waLog.Stdout("parser", "INFO", true),
		cfg:    cfg,
	}
}

func extractQuoteContext(mess *events.Message) *waE2E.ContextInfo {
	info := &waE2E.ContextInfo{
		StanzaID:      &mess.Info.ID,
		Participant:   proto.String(mess.Info.Sender.String()),
		QuotedMessage: mess.Message,
		MentionedJID:  []string{},
	}

	if ext := mess.Message.GetExtendedTextMessage(); ext != nil {
		if ctx := ext.GetContextInfo(); ctx != nil {
			info.MentionedJID = ctx.MentionedJID
			info.IsForwarded = ctx.IsForwarded
			info.ForwardingScore = ctx.ForwardingScore
		}
	} else if vid := mess.Message.GetVideoMessage(); vid != nil {
		if ctx := vid.GetContextInfo(); ctx != nil {
			info.MentionedJID = ctx.MentionedJID
			info.IsForwarded = ctx.IsForwarded
			info.ForwardingScore = ctx.ForwardingScore
		}
	} else if img := mess.Message.GetImageMessage(); img != nil {
		if ctx := img.GetContextInfo(); ctx != nil {
			info.MentionedJID = ctx.MentionedJID
			info.IsForwarded = ctx.IsForwarded
			info.ForwardingScore = ctx.ForwardingScore
		}
	}

	return info
}

func (p *Parser) Parse(ctx context.Context, mess *events.Message) *Message {
	sender := mess.Info.Sender.String()
	isOwner := false

	for _, own := range p.cfg.Owners {
		if strings.EqualFold(own, sender) {
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
	isGif := mess.Message.GetImageMessage().GetMimetype() == "image/gif"

	isQuotedImage := quotedMsg != nil && quotedMsg.GetImageMessage() != nil
	isQuotedVideo := quotedMsg != nil && quotedMsg.GetVideoMessage() != nil
	isQuotedSticker := quotedMsg != nil && quotedMsg.GetStickerMessage() != nil
	isQuotedGif := quotedMsg != nil && quotedMsg.GetImageMessage() != nil && quotedMsg.GetImageMessage().GetMimetype() == "image/gif"

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

	msgID := extractQuoteContext(mess)

	return &Message{
		From:        mess.Info.Chat,
		Sender:      mess.Info.Sender,
		PushName:    mess.Info.PushName,
		OwnerNumber: p.cfg.Owners,

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
		isGif:   isGif,

		IsAdmin:    isAdmin,
		IsBotAdmin: isBotAdmin,

		QuotedMsg: pickQuoted(mess),
		ID:        msgID,

		IsQuotedImage:   isQuotedImage,
		IsQuotedVideo:   isQuotedVideo,
		IsQuotedSticker: isQuotedSticker,
		isQuotedGif:     isQuotedGif,

		Reply: func(ctx context.Context, text string, opts ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			return p.Client.SendText(ctx, mess.Info.Chat, text, msgID, opts...)
		},
	}
}
