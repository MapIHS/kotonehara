package clients

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

func (c *Client) DeleteMessage(ctx context.Context, chat types.JID, sender types.JID, id string) (whatsmeow.SendResponse, error) {
	return c.WA.SendMessage(ctx, chat, c.WA.BuildRevoke(chat, sender, types.MessageID(id)))
}

func (c *Client) DeleteChat(ctx context.Context, chat types.JID, deleteMedia bool) error {
	return c.WA.SendAppState(ctx, appstate.BuildDeleteChat(chat, time.Time{}, nil, deleteMedia))
}
