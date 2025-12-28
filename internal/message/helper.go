package message

import (
	"strings"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

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
