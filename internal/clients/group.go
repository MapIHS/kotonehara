package clients

import (
	"context"

	"go.mau.fi/whatsmeow/types"
)

func (c *Client) GroupAdmins(ctx context.Context, j types.JID) ([]string, error) {
	key := j.String()

	// get cache
	if admins, ok := c.admins.get(key); ok {
		return admins, nil
	}

	info, err := c.WA.GetGroupInfo(ctx, j)
	if err != nil {
		return nil, err
	}

	admins := make([]string, 0, len(info.Participants))
	for _, p := range info.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			admins = append(admins, p.JID.String())
		}
	}

	// set cache
	c.admins.set(key, admins)

	return admins, nil
}
