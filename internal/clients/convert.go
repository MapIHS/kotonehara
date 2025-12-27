package clients

func (c *Client) BotJID() string {
	// JID bot (session) yang sedang login
	return c.WA.Store.ID.ToNonAD().String()
}
