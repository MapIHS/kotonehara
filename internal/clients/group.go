package clients

import (
	"context"

	"go.mau.fi/whatsmeow/types"
)

func (c *Client) GroupAdmins(ctx context.Context, j types.JID) ([]string, error) {
	var admin []string
	info, err := c.WA.GetGroupInfo(ctx, j)
	if err != nil {
		return admin, err
	}
	for _, p := range info.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			admin = append(admin, p.JID.String())
		}
	}
	return admin, nil
}
