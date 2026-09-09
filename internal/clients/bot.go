package clients

import (
	"context"

	"github.com/MapIHS/kotonehara/internal/identity"
	"go.mau.fi/whatsmeow/types"
)

func (c *Client) BotJID() string {
	return c.BotIdentity().StateJID()
}

func (c *Client) BotIdentity() identity.Identity {
	if c == nil || c.WA == nil || c.WA.Store == nil {
		return identity.Identity{}
	}
	return identity.New(c.WA.Store.GetLID(), c.WA.Store.GetJID())
}

func (c *Client) SameUser(ctx context.Context, target types.JID, known identity.Identity) (bool, error) {
	if known.Matches(target) {
		return true, nil
	}
	resolved, err := c.ResolveIdentity(ctx, target, types.EmptyJID)
	if err != nil {
		return false, err
	}
	for _, alias := range resolved.Aliases() {
		if known.Matches(alias) {
			return true, nil
		}
	}
	return false, nil
}
