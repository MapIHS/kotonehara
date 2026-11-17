package clients

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type messageSender interface {
	SendMessage(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
}

func (c *Client) SendMessage(ctx context.Context, to types.JID, msg *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	return c.sendStored(ctx, c.WA, to, msg, extra...)
}

func (c *Client) sendStored(ctx context.Context, sender messageSender, to types.JID, msg *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	response, err := sender.SendMessage(ctx, to, msg, extra...)
	if err == nil && c.Messages != nil && response.ID != "" {
		id := c.BotIdentity()
		from, alt := id.LID, id.PN
		if from.IsEmpty() {
			from, alt = id.PN, id.LID
		}
		stamp := response.Timestamp
		if stamp.IsZero() {
			stamp = time.Now()
		}
		evt := (&events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: to.ToNonAD(), Sender: from, SenderAlt: alt, IsFromMe: true, IsGroup: to.Server == types.GroupServer}, ID: response.ID, Timestamp: stamp}, RawMessage: msg}).UnwrapRaw()
		c.Messages.Capture(c.MessageStoreAccount(), evt)
	}
	return response, err
}

type storedViewOnceTransport struct {
	*whatsmeow.Client
	owner *Client
}

func (t storedViewOnceTransport) SendMessage(ctx context.Context, to types.JID, msg *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	return t.owner.SendMessage(ctx, to, msg, extra...)
}
