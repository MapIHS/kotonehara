package clients

import (
	"container/list"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// A quoted album parent carries no media, so commands such as .s need the
// children that pointed at it. Only download descriptors are kept (no media
// bytes, thumbnails, or captions), bounded by count and age, so memory stays
// in the low hundreds of kilobytes even for busy groups.
const (
	albumCacheMaxAlbums = 512
	albumCacheMaxItems  = 30
	albumCacheTTL       = time.Hour
)

type albumEntry struct {
	key     string
	expires time.Time
	ids     map[string]struct{}
	items   []whatsmeow.DownloadableMessage
}

type albumCache struct {
	mu    sync.Mutex
	now   func() time.Time
	order *list.List // oldest album first
	byKey map[string]*list.Element
}

func newAlbumCache() *albumCache {
	return &albumCache{now: time.Now, order: list.New(), byKey: make(map[string]*list.Element)}
}

func albumKey(chat types.JID, parentID string) string {
	return chat.ToNonAD().String() + "|" + parentID
}

// albumParentID returns the parent album ID when msg is a media album child.
func albumParentID(msg *waE2E.Message) string {
	assoc := msg.GetMessageContextInfo().GetMessageAssociation()
	if assoc.GetAssociationType() != waE2E.MessageAssociation_MEDIA_ALBUM {
		return ""
	}
	return assoc.GetParentMessageKey().GetID()
}

// albumDescriptor keeps only what is needed to download and convert the media.
func albumDescriptor(msg *waE2E.Message) whatsmeow.DownloadableMessage {
	if img := msg.GetImageMessage(); img != nil {
		return &waE2E.ImageMessage{
			URL: img.URL, DirectPath: img.DirectPath, MediaKey: img.MediaKey,
			FileSHA256: img.FileSHA256, FileEncSHA256: img.FileEncSHA256,
			FileLength: img.FileLength, Mimetype: img.Mimetype,
		}
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return &waE2E.VideoMessage{
			URL: vid.URL, DirectPath: vid.DirectPath, MediaKey: vid.MediaKey,
			FileSHA256: vid.FileSHA256, FileEncSHA256: vid.FileEncSHA256,
			FileLength: vid.FileLength, Mimetype: vid.Mimetype,
			Seconds: vid.Seconds, GifPlayback: vid.GifPlayback,
		}
	}
	return nil
}

func mediaURL(msg *waE2E.Message) string {
	if img := msg.GetImageMessage(); img != nil {
		return img.GetURL()
	}
	return msg.GetVideoMessage().GetURL()
}

func (c *albumCache) remember(chat types.JID, childID string, msg *waE2E.Message) {
	parentID := albumParentID(msg)
	if c == nil || parentID == "" || childID == "" {
		return
	}
	item := albumDescriptor(msg)
	if item == nil || len(item.GetMediaKey()) == 0 || item.GetDirectPath() == "" && mediaURL(msg) == "" {
		return
	}

	key := albumKey(chat, parentID)
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(now)

	var entry *albumEntry
	if el, ok := c.byKey[key]; ok {
		entry = el.Value.(*albumEntry)
	} else {
		entry = &albumEntry{key: key, expires: now.Add(albumCacheTTL), ids: make(map[string]struct{})}
		c.byKey[key] = c.order.PushBack(entry)
		for c.order.Len() > albumCacheMaxAlbums {
			c.removeLocked(c.order.Front())
		}
	}
	if _, dup := entry.ids[childID]; dup || len(entry.items) >= albumCacheMaxItems {
		return
	}
	entry.ids[childID] = struct{}{}
	entry.items = append(entry.items, item)
}

func (c *albumCache) get(chat types.JID, parentID string) []whatsmeow.DownloadableMessage {
	if c == nil || parentID == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(c.now())
	el, ok := c.byKey[albumKey(chat, parentID)]
	if !ok {
		return nil
	}
	items := el.Value.(*albumEntry).items
	return append([]whatsmeow.DownloadableMessage(nil), items...)
}

func (c *albumCache) pruneLocked(now time.Time) {
	for el := c.order.Front(); el != nil; el = c.order.Front() {
		if el.Value.(*albumEntry).expires.After(now) {
			return
		}
		c.removeLocked(el)
	}
}

func (c *albumCache) removeLocked(el *list.Element) {
	delete(c.byKey, el.Value.(*albumEntry).key)
	c.order.Remove(el)
}

// RememberAlbumChild records an incoming media album child so a later reply
// to the album parent can still reach its photos and videos.
func (c *Client) RememberAlbumChild(evt *events.Message) {
	if c == nil || evt == nil {
		return
	}
	c.albums.remember(evt.Info.Chat, string(evt.Info.ID), evt.Message)
}

// AlbumMedia returns the remembered media of an album, in arrival order.
func (c *Client) AlbumMedia(chat types.JID, parentID string) []whatsmeow.DownloadableMessage {
	if c == nil {
		return nil
	}
	return c.albums.get(chat, parentID)
}
