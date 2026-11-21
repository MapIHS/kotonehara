package clients

import (
	"fmt"
	"testing"
	"time"

	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func albumChild(parentID string, video bool) *waE2E.Message {
	msg := &waE2E.Message{MessageContextInfo: &waE2E.MessageContextInfo{MessageAssociation: &waE2E.MessageAssociation{
		AssociationType:  waE2E.MessageAssociation_MEDIA_ALBUM.Enum(),
		ParentMessageKey: &waCommon.MessageKey{ID: proto.String(parentID)},
	}}}
	if video {
		msg.VideoMessage = &waE2E.VideoMessage{DirectPath: proto.String("/v"), MediaKey: []byte("k"), Mimetype: proto.String("video/mp4"), JPEGThumbnail: make([]byte, 4096)}
	} else {
		msg.ImageMessage = &waE2E.ImageMessage{DirectPath: proto.String("/i"), MediaKey: []byte("k"), Mimetype: proto.String("image/jpeg"), JPEGThumbnail: make([]byte, 4096), Caption: proto.String("secret")}
	}
	return msg
}

func TestAlbumCacheKeepsChildrenInOrderWithoutThumbnailsOrDuplicates(t *testing.T) {
	c := newAlbumCache()
	chat := types.NewJID("123", types.GroupServer)
	c.remember(chat, "c1", albumChild("p", false))
	c.remember(chat, "c2", albumChild("p", true))
	c.remember(chat, "c1", albumChild("p", false))
	c.remember(chat, "x", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{DirectPath: proto.String("/i"), MediaKey: []byte("k")}})

	items := c.get(chat, "p")
	if len(items) != 2 {
		t.Fatalf("items=%d", len(items))
	}
	img, ok := items[0].(*waE2E.ImageMessage)
	if !ok || len(img.GetJPEGThumbnail()) != 0 || img.GetCaption() != "" || img.GetDirectPath() != "/i" {
		t.Fatalf("image descriptor not trimmed: %#v", items[0])
	}
	if _, ok := items[1].(*waE2E.VideoMessage); !ok {
		t.Fatalf("second item = %T", items[1])
	}
	if got := c.get(types.NewJID("456", types.GroupServer), "p"); got != nil {
		t.Fatalf("album leaked across chats: %d", len(got))
	}
}

func TestAlbumCacheExpiresAndBoundsAlbums(t *testing.T) {
	c := newAlbumCache()
	now := time.Unix(0, 0)
	c.now = func() time.Time { return now }
	chat := types.NewJID("123", types.GroupServer)

	c.remember(chat, "c", albumChild("old", false))
	now = now.Add(albumCacheTTL + time.Second)
	if got := c.get(chat, "old"); got != nil {
		t.Fatalf("expired album still returned")
	}

	for i := 0; i < albumCacheMaxAlbums+5; i++ {
		c.remember(chat, "c", albumChild(fmt.Sprintf("p%d", i), false))
	}
	if c.order.Len() != albumCacheMaxAlbums || len(c.byKey) != albumCacheMaxAlbums {
		t.Fatalf("albums=%d keys=%d", c.order.Len(), len(c.byKey))
	}
	if c.get(chat, "p0") != nil || c.get(chat, fmt.Sprintf("p%d", albumCacheMaxAlbums+4)) == nil {
		t.Fatal("oldest album should be evicted first")
	}

	for i := 0; i < albumCacheMaxItems+5; i++ {
		c.remember(chat, fmt.Sprintf("c%d", i), albumChild("big", false))
	}
	if got := len(c.get(chat, "big")); got != albumCacheMaxItems {
		t.Fatalf("items=%d", got)
	}
}

func TestAlbumCacheUnwrapsAssociatedChildAndDeviceSent(t *testing.T) {
	c := newAlbumCache()
	chat := types.NewJID("123", types.GroupServer)

	// AssociatedChildMessage with association on the outer message.
	outerAssoc := &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{MessageAssociation: &waE2E.MessageAssociation{
			AssociationType:  waE2E.MessageAssociation_MEDIA_ALBUM.Enum(),
			ParentMessageKey: &waCommon.MessageKey{ID: proto.String("p-wrap")},
		}},
		AssociatedChildMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{DirectPath: proto.String("/w1"), MediaKey: []byte("k1")},
		}},
	}
	// DeviceSentMessage wrapping EphemeralMessage + AssociatedChildMessage, with association inside.
	nested := &waE2E.Message{
		DeviceSentMessage: &waE2E.DeviceSentMessage{Message: &waE2E.Message{
			EphemeralMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
				AssociatedChildMessage: &waE2E.FutureProofMessage{Message: albumChild("p-wrap", true)},
			}},
		}},
	}
	// Association only on raw outer envelope while inner message also has its own MessageContextInfo.
	innerOnly := &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: []byte("s")},
		ImageMessage:       &waE2E.ImageMessage{DirectPath: proto.String("/w3"), MediaKey: []byte("k3")},
	}
	rawOuter := &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{MessageAssociation: &waE2E.MessageAssociation{
			AssociationType:  waE2E.MessageAssociation_MEDIA_ALBUM.Enum(),
			ParentMessageKey: &waCommon.MessageKey{ID: proto.String("p-wrap")},
		}},
	}
	// View-once media must never be cached.
	viewOnce := albumChild("p-wrap", false)
	viewOnce.ImageMessage.ViewOnce = proto.Bool(true)

	c.remember(chat, "c1", outerAssoc)
	c.remember(chat, "c2", nested)
	c.remember(chat, "c3", innerOnly, rawOuter)
	c.remember(chat, "c4", viewOnce)

	items := c.get(chat, "p-wrap")
	if len(items) != 3 {
		t.Fatalf("items=%d, want 3", len(items))
	}
	if items[0].GetDirectPath() != "/w1" || items[2].GetDirectPath() != "/w3" {
		t.Fatalf("unexpected direct paths: %q, %q", items[0].GetDirectPath(), items[2].GetDirectPath())
	}
	if _, ok := items[1].(*waE2E.VideoMessage); !ok {
		t.Fatalf("second item = %T, want *waE2E.VideoMessage", items[1])
	}
}
