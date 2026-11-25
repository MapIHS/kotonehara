package clients

import (
	"context"
	"errors"

	"github.com/MapIHS/kotonehara/internal/message"
)

// DownloadMedia downloads the media selected by the parsed command. For a
// quoted view-once message, it transparently serves the short-lived in-memory
// copy captured when the original message arrived.
func (c *Client) DownloadMedia(ctx context.Context, m *message.Message) ([]byte, error) {
	if c == nil || c.WA == nil || m == nil || m.Media == nil {
		return nil, errors.New("media tidak tersedia")
	}
	if m.IsQuotedViewOnce && m.ContextInfo.GetStanzaID() != "" {
		if data, raw, ok := c.CachedViewOnce(ctx, m.From, m.ContextInfo.GetStanzaID()); ok {
			if len(data) > 0 {
				return data, nil
			}
			if original, _, _ := viewOnceDownloadable(message.UnwrapMessage(raw)); original != nil && original.GetDirectPath() != "" {
				return c.WA.Download(ctx, original)
			}
		}
	}
	return c.WA.Download(ctx, m.Media)
}
