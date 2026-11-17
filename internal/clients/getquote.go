package clients

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

var (
	ErrQuoteDifferentChat      = errors.New("quoted message belongs to another chat")
	ErrForwardUnsupported      = errors.New("message content cannot be forwarded")
	ErrForwardMediaUnavailable = errors.New("forwarded media metadata is incomplete")
)

// GetQuote follows exactly two reply keys: command -> replied-to message ->
// that message's quote. It never searches another chat or follows further replies.
func (c *Client) GetQuote(ctx context.Context, m *message.Message) (*waE2E.Message, error) {
	if m == nil || m.ContextInfo.GetStanzaID() == "" {
		return nil, sql.ErrNoRows
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	parent := m.QuotedMsg
	stored, err := c.lookupQuotedMessage(ctx, m, m.ContextInfo)
	if err == nil {
		parent = stored.Raw
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	ci := message.ExtractContextInfo(parent)
	if ci.GetStanzaID() == "" {
		return nil, sql.ErrNoRows
	}
	// Enforce scope even when falling back to an embedded quote without a store.
	if ci.GetRemoteJID() != "" {
		remote, err := types.ParseJID(ci.GetRemoteJID())
		if err != nil {
			return nil, ErrQuoteDifferentChat
		}
		alt := types.EmptyJID
		if m.Msg != nil && !m.IsGroup {
			if m.Msg.Info.IsFromMe {
				alt = m.Msg.Info.RecipientAlt
			} else {
				alt = m.Msg.Info.SenderAlt
			}
		}
		chatIdentity, err := c.ResolveIdentity(ctx, m.From, alt)
		if err != nil {
			return nil, err
		}
		if !chatIdentity.Matches(remote) {
			return nil, ErrQuoteDifferentChat
		}
	}
	target, err := c.lookupQuotedMessage(ctx, m, ci)
	if err == nil {
		return target.Raw, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if ci.GetQuotedMessage() == nil {
		return nil, sql.ErrNoRows
	}
	return proto.Clone(ci.GetQuotedMessage()).(*waE2E.Message), nil
}

func (c *Client) SendForwarded(ctx context.Context, to types.JID, raw *waE2E.Message) (whatsmeow.SendResponse, error) {
	out, err := forwardedMessage(raw)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}
	return c.SendMessage(ctx, to, out)
}

// Only content is copied. Protocol messages, sender-key data, reply routing,
// mentions, and message secrets are never replayed as part of a forward.
func forwardedMessage(raw *waE2E.Message) (*waE2E.Message, error) {
	if raw == nil {
		return nil, sql.ErrNoRows
	}
	source := message.UnwrapMessage(proto.Clone(raw).(*waE2E.Message))
	score := message.ExtractContextInfo(source).GetForwardingScore()
	if score < 0xffffffff {
		score++
	}
	forward := &waE2E.ContextInfo{IsForwarded: proto.Bool(true), ForwardingScore: proto.Uint32(score)}
	out := &waE2E.Message{}
	var media whatsmeow.DownloadableMessage
	switch {
	case source.GetConversation() != "":
		out.ExtendedTextMessage = &waE2E.ExtendedTextMessage{Text: source.Conversation, ContextInfo: forward}
	case source.GetExtendedTextMessage().GetText() != "":
		out.ExtendedTextMessage = source.ExtendedTextMessage
		out.ExtendedTextMessage.ContextInfo = forward
	case source.GetImageMessage() != nil:
		out.ImageMessage = source.ImageMessage
		out.ImageMessage.ContextInfo = forward
		media = out.ImageMessage
	case source.GetVideoMessage() != nil:
		out.VideoMessage = source.VideoMessage
		out.VideoMessage.ContextInfo = forward
		media = out.VideoMessage
	case source.GetAudioMessage() != nil:
		out.AudioMessage = source.AudioMessage
		out.AudioMessage.ContextInfo = forward
		media = out.AudioMessage
	case source.GetDocumentMessage() != nil:
		out.DocumentMessage = source.DocumentMessage
		out.DocumentMessage.ContextInfo = forward
		media = out.DocumentMessage
	case source.GetStickerMessage() != nil:
		out.StickerMessage = source.StickerMessage
		out.StickerMessage.ContextInfo = forward
		media = out.StickerMessage
	case source.GetContactMessage() != nil:
		out.ContactMessage = source.ContactMessage
		out.ContactMessage.ContextInfo = forward
	case source.GetContactsArrayMessage() != nil:
		out.ContactsArrayMessage = source.ContactsArrayMessage
		out.ContactsArrayMessage.ContextInfo = forward
	case source.GetLocationMessage() != nil:
		out.LocationMessage = source.LocationMessage
		out.LocationMessage.ContextInfo = forward
	default:
		return nil, ErrForwardUnsupported
	}
	if media != nil && (media.GetDirectPath() == "" || len(media.GetMediaKey()) != 32 || len(media.GetFileSHA256()) != 32 || len(media.GetFileEncSHA256()) != 32) {
		return nil, ErrForwardMediaUnavailable
	}
	// Preserve view-once semantics; getquote does not implicitly run RVO.
	if message.IsViewOnceMessage(raw) {
		if out.ImageMessage != nil {
			out.ImageMessage.ViewOnce = proto.Bool(true)
		}
		if out.VideoMessage != nil {
			out.VideoMessage.ViewOnce = proto.Bool(true)
		}
		if out.AudioMessage != nil {
			out.AudioMessage.ViewOnce = proto.Bool(true)
		}
	}
	return out, nil
}
