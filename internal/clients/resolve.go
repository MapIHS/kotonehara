package clients

import (
	"context"
	"log"

	"go.mau.fi/whatsmeow/types"
)

// SenderPhone resolves a sender JID to a canonical phone-number JID string
// (e.g. "628xxx@s.whatsapp.net") regardless of whether the input is a
// traditional PN JID or a LID.  Device suffixes are always stripped.
//
// If the JID is a LID and the mapping is not found in the local store,
// the original LID (with device stripped) is returned as a fallback.
func (c *Client) SenderPhone(ctx context.Context, sender types.JID) string {
	clean := sender.ToNonAD()

	switch clean.Server {
	case types.DefaultUserServer:
		// Already a phone-number JID – just return it.
		return clean.String()

	case types.HiddenUserServer:
		// LID – try to resolve to phone number via local mapping table.
		pn, err := c.WA.Store.LIDs.GetPNForLID(ctx, clean)
		if err != nil {
			log.Printf("resolve LID %s: %v", clean, err)
			return clean.String() // fallback
		}
		if !pn.IsEmpty() {
			return pn.ToNonAD().String()
		}
		return clean.String() // mapping not found, fallback
	}

	return clean.String()
}
