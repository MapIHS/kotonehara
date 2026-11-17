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

	if mess == nil {
		return nil
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
	return ExtractContextInfo(mess.Message).GetQuotedMessage()
}

// ExtractContextInfo returns reply metadata for the message's content type.
func ExtractContextInfo(raw *waE2E.Message) *waE2E.ContextInfo {
	m := UnwrapMessage(raw)
	switch {
	case m.GetExtendedTextMessage() != nil:
		return m.GetExtendedTextMessage().GetContextInfo()
	case m.GetImageMessage() != nil:
		return m.GetImageMessage().GetContextInfo()
	case m.GetVideoMessage() != nil:
		return m.GetVideoMessage().GetContextInfo()
	case m.GetAudioMessage() != nil:
		return m.GetAudioMessage().GetContextInfo()
	case m.GetDocumentMessage() != nil:
		return m.GetDocumentMessage().GetContextInfo()
	case m.GetStickerMessage() != nil:
		return m.GetStickerMessage().GetContextInfo()
	case m.GetContactMessage() != nil:
		return m.GetContactMessage().GetContextInfo()
	case m.GetContactsArrayMessage() != nil:
		return m.GetContactsArrayMessage().GetContextInfo()
	case m.GetLocationMessage() != nil:
		return m.GetLocationMessage().GetContextInfo()
	case m.GetLiveLocationMessage() != nil:
		return m.GetLiveLocationMessage().GetContextInfo()
	default:
		return nil
	}
}

// UnwrapMessage handles wrappers on quotes as well as top-level messages.
func UnwrapMessage(raw *waE2E.Message) *waE2E.Message {
	if raw == nil {
		return nil
	}
	return (&events.Message{RawMessage: raw}).UnwrapRaw().Message
}

// IsViewOnceMessage retains the view-once signal before a quote is unwrapped.
func IsViewOnceMessage(raw *waE2E.Message) bool {
	if raw == nil {
		return false
	}
	evt := (&events.Message{RawMessage: raw}).UnwrapRaw()
	return evt.IsViewOnce || evt.Message.GetImageMessage().GetViewOnce() || evt.Message.GetVideoMessage().GetViewOnce() || evt.Message.GetAudioMessage().GetViewOnce()
}

// SetQuotedMessage keeps command media flags consistent after store lookup.
func SetQuotedMessage(m *Message, quoted *waE2E.Message) {
	m.IsQuotedViewOnce = IsViewOnceMessage(quoted)
	q := UnwrapMessage(quoted)
	m.QuotedMsg = q
	m.Media = pickMedia(q, m.Msg)
	if strings.EqualFold(m.Command, "tomp3") && q.GetAudioMessage() != nil {
		m.Media = q.GetAudioMessage()
	}
	m.IsQuotedImage = q.GetImageMessage() != nil
	m.IsQuotedVideo = q.GetVideoMessage() != nil
	m.IsQuotedSticker = q.GetStickerMessage() != nil
	m.IsQuotedGif = q.GetImageMessage().GetMimetype() == "image/gif"
	m.IsQuotedStickerGif = q.GetStickerMessage().GetIsAnimated()
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
