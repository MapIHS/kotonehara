package clients

import (
	"context"

	"github.com/MapIHS/kotonehara/internal/identity"
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
	admins = append(admins, groupAdminAliases(info.Participants)...)

	// set cache
	c.admins.set(key, admins)

	return admins, nil
}

func groupAdminAliases(participants []types.GroupParticipant) []string {
	admins := make([]string, 0, len(participants))
	seen := make(map[string]struct{})
	for _, participant := range participants {
		if !participant.IsAdmin && !participant.IsSuperAdmin {
			continue
		}
		for _, jid := range []types.JID{participant.JID, participant.PhoneNumber, participant.LID} {
			jid = identity.Normalize(jid)
			if jid.IsEmpty() {
				continue
			}
			value := jid.String()
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			admins = append(admins, value)
		}
	}
	return admins
}
