package clients

import "strings"

func (c *Client) BotJID() string {
	return stripDeviceFromLID(c.WA.Store.LID.String())
}

func stripDeviceFromLID(jid string) string {
	userServer, server, ok := strings.Cut(jid, "@")
	if !ok {
		return jid
	}
	user, _, _ := strings.Cut(userServer, ":")
	return user + "@" + server
}
