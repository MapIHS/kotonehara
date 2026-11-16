package message

import (
	"net/url"
	"regexp"
	"strings"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

func ExtractBody(mess *events.Message) string {
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

func IsValidURL(str string) bool {
	u, err := url.ParseRequestURI(str)
	return err == nil && u.Scheme != "" && u.Host != ""
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
		if msg := quoted.GetLottieStickerMessage().GetMessage().GetStickerMessage(); msg != nil {
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

func pickQuoted(mess *events.Message) *waE2E.Message {
	if ext := mess.Message.GetExtendedTextMessage(); ext != nil {
		if ctx := ext.GetContextInfo(); ctx != nil {
			return ctx.GetQuotedMessage()
		}
	}

	if vid := mess.Message.GetVideoMessage(); vid != nil {
		if ctx := vid.GetContextInfo(); ctx != nil {
			return ctx.GetQuotedMessage()
		}
	}

	if img := mess.Message.GetImageMessage(); img != nil {
		if ctx := img.GetContextInfo(); ctx != nil {
			return ctx.GetQuotedMessage()
		}
	}

	if st := mess.Message.GetStickerMessage(); st != nil {
		if ctx := st.GetContextInfo(); ctx != nil {
			return ctx.GetQuotedMessage()
		}
	}

	return nil
}

// QuotedText returns the text or caption of the replied-to message.
func (m *Message) QuotedText() string {
	q := m.QuotedMsg
	if q == nil {
		return ""
	}
	for _, s := range []string{
		q.GetExtendedTextMessage().GetText(),
		q.GetConversation(),
		q.GetImageMessage().GetCaption(),
		q.GetVideoMessage().GetCaption(),
		q.GetDocumentMessage().GetCaption(),
	} {
		if s != "" {
			return s
		}
	}
	return ""
}

var urlPattern = regexp.MustCompile(`https?://[^\s<>"]+`)

// FirstURL returns the first http(s) link found in text, without trailing
// sentence punctuation, or "" when there is none.
func FirstURL(text string) string {
	for _, candidate := range urlPattern.FindAllString(text, -1) {
		candidate = strings.TrimRight(candidate, ".,;:!?)]}'*_~")
		if IsValidURL(candidate) {
			return candidate
		}
	}
	return ""
}
