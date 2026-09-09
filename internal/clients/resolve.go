package clients

import (
	"context"

	"github.com/MapIHS/kotonehara/internal/identity"
	"go.mau.fi/whatsmeow/types"
)

func (c *Client) ResolveIdentity(ctx context.Context, primary, alternate types.JID) (identity.Identity, error) {
	id := identity.New(primary, alternate)
	if c == nil || c.WA == nil || c.WA.Store == nil || c.WA.Store.LIDs == nil {
		return id, nil
	}

	if id.PN.IsEmpty() && !id.LID.IsEmpty() {
		pn, err := c.WA.Store.LIDs.GetPNForLID(ctx, id.LID)
		if err != nil {
			return id, err
		}
		id.Add(pn)
	}
	if id.LID.IsEmpty() && !id.PN.IsEmpty() {
		lid, err := c.WA.Store.LIDs.GetLIDForPN(ctx, id.PN)
		if err != nil {
			return id, err
		}
		id.Add(lid)
	}
	return id, nil
}
