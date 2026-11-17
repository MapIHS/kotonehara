package clients

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/msgstore"
	"go.mau.fi/whatsmeow/types"
)

func (c *Client) MessageStoreAccount() string {
	if c == nil || c.WA == nil || c.WA.Store == nil {
		return ""
	}
	return c.BotIdentity().StateJID()
}

// RestoreQuotedMedia runs outside event dispatch, only for a recognized media
// command whose quote lacks the metadata needed by whatsmeow.Download.
func (c *Client) RestoreQuotedMedia(ctx context.Context, m *message.Message) {
	if m == nil {
		return
	}
	if m.Media != nil && m.Media.GetDirectPath() != "" && len(m.Media.GetMediaKey()) > 0 && len(m.Media.GetFileEncSHA256()) > 0 {
		return
	}
	c.RestoreQuotedMessage(ctx, m)
}

// RestoreQuotedMessage also restores flags that may be missing from the quote,
// even when it includes a downloadable media descriptor (e.g. view-once).
func (c *Client) RestoreQuotedMessage(ctx context.Context, m *message.Message) bool {
	if c == nil || c.Messages == nil || m == nil || m.ContextInfo.GetStanzaID() == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	participant, _ := types.ParseJID(m.ContextInfo.GetParticipant())
	chatIdentity, err := c.ResolveIdentity(ctx, m.From, types.EmptyJID)
	if err != nil {
		return false
	}
	key := msgstore.Key{Account: c.MessageStoreAccount(), Chat: m.From, ID: m.ContextInfo.GetStanzaID(), Sender: participant}
	if m.Msg != nil && !m.IsGroup {
		if m.Msg.Info.IsFromMe {
			key.ChatAlt = m.Msg.Info.RecipientAlt
		} else {
			key.ChatAlt = m.Msg.Info.SenderAlt
		}
	}
	for _, alias := range chatIdentity.Aliases() {
		if alias != m.From.ToNonAD() {
			key.ChatAlt = alias
		}
	}
	if !participant.IsEmpty() {
		senderIdentity, err := c.ResolveIdentity(ctx, participant, types.EmptyJID)
		if err != nil {
			return false
		}
		for _, alias := range senderIdentity.Aliases() {
			if alias != participant.ToNonAD() {
				key.SenderAlt = alias
			}
		}
		fromMe := c.BotIdentity().Matches(participant) || c.BotIdentity().Matches(key.SenderAlt)
		key.FromMe = &fromMe
	}
	stored, err := c.Messages.Get(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		stored, err = c.Messages.GetQuotedMedia(ctx, key)
	}
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("message store quote lookup: %v", err)
		}
		return false
	}
	message.SetQuotedMessage(m, stored.Raw)
	m.IsQuotedViewOnce = stored.IsViewOnce
	return true
}
