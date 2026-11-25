package clients

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

const (
	viewOnceCacheTTL       = 30 * time.Minute
	viewOnceCacheMaxItems  = 12
	viewOnceCacheMaxBytes  = 48 << 20
	viewOnceCacheItemLimit = 24 << 20
	viewOnceCacheWait      = 15 * time.Second
)

var (
	ErrViewOnceNotCached   = errors.New("view-once media is not in the temporary cache")
	ErrViewOnceUnsupported = errors.New("view-once media type is unsupported")
	viewOnceDownloadSlot   = make(chan struct{}, 1)
)

type viewOnceKey struct {
	chat types.JID
	id   string
}

type viewOnceEntry struct {
	raw     *waE2E.Message
	data    []byte
	expires time.Time
}

type viewOnceCache struct {
	mu      sync.Mutex
	items   map[viewOnceKey]viewOnceEntry
	pending map[viewOnceKey]chan struct{}
	bytes   int
	order   []viewOnceKey
	now     func() time.Time
}

func newViewOnceCache() *viewOnceCache {
	return &viewOnceCache{items: make(map[viewOnceKey]viewOnceEntry), pending: make(map[viewOnceKey]chan struct{}), now: time.Now}
}

// RememberViewOnce downloads only view-once media while its WhatsApp URL is
// still fresh. It runs asynchronously from the event callback and retains only
// a small, short-lived in-memory cache; regular chat messages are never stored.
func (c *Client) RememberViewOnce(ctx context.Context, evt *events.Message) {
	if c == nil || c.WA == nil || c.viewOnce == nil || evt == nil || evt.Info.ID == "" || evt.Info.Chat.IsEmpty() {
		return
	}
	unwrapped := evt.UnwrapRaw()
	if unwrapped.Message == nil {
		log.Print("view-once received without message body")
		return
	}
	// WhatsApp may mark view-once directly on the media proto instead of
	// wrapping it in ViewOnceMessage. events.Message.IsViewOnce only tracks the
	// wrapper, so include the per-media flags too.
	isViewOnce := unwrapped.IsViewOnce ||
		unwrapped.Message.GetImageMessage().GetViewOnce() ||
		unwrapped.Message.GetVideoMessage().GetViewOnce() ||
		unwrapped.Message.GetAudioMessage().GetViewOnce()
	if !isViewOnce {
		return
	}
	media, kind, size := viewOnceDownloadable(unwrapped.Message)
	if media == nil {
		log.Printf("view-once received: type=unsupported")
		return
	}
	log.Printf("view-once received: type=%s direct_path=%t media_key_bytes=%d file_len=%d", kind, media.GetDirectPath() != "", len(media.GetMediaKey()), size)
	if size > viewOnceCacheItemLimit {
		log.Printf("view-once cache skipped: file exceeds %d MiB limit", viewOnceCacheItemLimit>>20)
		return
	}
	select {
	case viewOnceDownloadSlot <- struct{}{}:
	default:
		log.Print("view-once cache skipped: downloader busy")
		return
	}
	key := viewOnceKey{chat: evt.Info.Chat.ToNonAD(), id: string(evt.Info.ID)}
	if !c.viewOnce.startPending(key, evt.RawMessage) {
		<-viewOnceDownloadSlot
		return
	}
	go func() {
		defer func() { <-viewOnceDownloadSlot }()
		defer c.viewOnce.finishPending(key)
		dlCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		file, err := os.CreateTemp("", "kotonehara-view-once-*")
		if err != nil {
			log.Printf("view-once cache skipped: temp file unavailable (%T)", err)
			return
		}
		defer func() {
			_ = file.Close()
			_ = os.Remove(file.Name())
		}()
		bounded := &viewOnceCacheFile{File: file, max: viewOnceCacheItemLimit + 32}
		if err := c.WA.DownloadToFile(dlCtx, media, bounded); err != nil {
			log.Printf("view-once cache download failed: direct_path=%t media_key_bytes=%d error_type=%T", media.GetDirectPath() != "", len(media.GetMediaKey()), err)
			return
		}
		info, err := file.Stat()
		if err != nil || info.Size() <= 0 || info.Size() > viewOnceCacheItemLimit {
			return
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, viewOnceCacheItemLimit+1))
		if err != nil || len(data) == 0 || len(data) > viewOnceCacheItemLimit {
			log.Printf("view-once cache read failed: bytes=%d error_type=%T", len(data), err)
			return
		}
		c.viewOnce.put(key, evt.RawMessage, data)
		log.Printf("view-once cached: bytes=%d ttl_minutes=%d", len(data), int(viewOnceCacheTTL/time.Minute))
	}()
}

func viewOnceDownloadable(raw *waE2E.Message) (whatsmeow.DownloadableMessage, whatsmeow.MediaType, uint64) {
	msg := message.UnwrapMessage(raw)
	switch {
	case msg.GetImageMessage() != nil:
		media := msg.GetImageMessage()
		return media, whatsmeow.MediaImage, media.GetFileLength()
	case msg.GetVideoMessage() != nil:
		media := msg.GetVideoMessage()
		return media, whatsmeow.MediaVideo, media.GetFileLength()
	case msg.GetAudioMessage() != nil:
		media := msg.GetAudioMessage()
		return media, whatsmeow.MediaAudio, media.GetFileLength()
	default:
		return nil, "", 0
	}
}

func (c *viewOnceCache) put(key viewOnceKey, raw *waE2E.Message, data []byte) {
	if raw == nil || len(data) == 0 || len(data) > viewOnceCacheMaxBytes {
		return
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(now)
	if old, ok := c.items[key]; ok {
		c.deleteLocked(key, old)
	}
	for len(c.items) >= viewOnceCacheMaxItems || c.bytes+len(data) > viewOnceCacheMaxBytes {
		if len(c.order) == 0 {
			return
		}
		c.removeOldestLocked()
	}
	c.items[key] = viewOnceEntry{raw: proto.Clone(raw).(*waE2E.Message), data: append([]byte(nil), data...), expires: now.Add(viewOnceCacheTTL)}
	c.bytes += len(data)
	c.order = append(c.order, key)
}

func normalViewOnceMedia(source *waE2E.Message, up whatsmeow.UploadResponse, quote *waE2E.ContextInfo) *waE2E.Message {
	stamp := proto.Int64(time.Now().Unix())
	switch {
	case source.GetImageMessage() != nil:
		m := source.GetImageMessage()
		return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileSHA256: up.FileSHA256, FileEncSHA256: up.FileEncSHA256, FileLength: &up.FileLength, MediaKeyTimestamp: stamp, Caption: m.Caption, Mimetype: m.Mimetype, Width: m.Width, Height: m.Height, JPEGThumbnail: m.JPEGThumbnail, ContextInfo: quote}}
	case source.GetVideoMessage() != nil:
		m := source.GetVideoMessage()
		return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileSHA256: up.FileSHA256, FileEncSHA256: up.FileEncSHA256, FileLength: &up.FileLength, MediaKeyTimestamp: stamp, Caption: m.Caption, Mimetype: m.Mimetype, Width: m.Width, Height: m.Height, Seconds: m.Seconds, GifPlayback: m.GifPlayback, JPEGThumbnail: m.JPEGThumbnail, ContextInfo: quote}}
	default:
		m := source.GetAudioMessage()
		return &waE2E.Message{AudioMessage: &waE2E.AudioMessage{URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileSHA256: up.FileSHA256, FileEncSHA256: up.FileEncSHA256, FileLength: &up.FileLength, MediaKeyTimestamp: stamp, Mimetype: m.Mimetype, Seconds: m.Seconds, PTT: m.PTT, Waveform: m.Waveform, ContextInfo: quote}}
	}
}

func (c *viewOnceCache) get(ctx context.Context, chat types.JID, id string) (viewOnceEntry, bool) {
	if c == nil || id == "" {
		return viewOnceEntry{}, false
	}
	now := c.now()
	key := viewOnceKey{chat: chat.ToNonAD(), id: id}
	c.mu.Lock()
	c.pruneLocked(now)
	entry, ok := c.items[key]
	pending := c.pending[key]
	c.mu.Unlock()
	if !ok && pending != nil {
		timer := time.NewTimer(viewOnceCacheWait)
		defer timer.Stop()
		select {
		case <-pending:
		case <-timer.C:
		case <-ctx.Done():
			return viewOnceEntry{}, false
		}
		c.mu.Lock()
		c.pruneLocked(c.now())
		entry, ok = c.items[key]
		c.mu.Unlock()
	}
	if !ok {
		return viewOnceEntry{}, false
	}
	entry.raw = proto.Clone(entry.raw).(*waE2E.Message)
	entry.data = append([]byte(nil), entry.data...)
	return entry, true
}

func (c *viewOnceCache) startPending(key viewOnceKey, raw *waE2E.Message) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(c.now())
	if existing, ok := c.items[key]; ok && len(existing.data) > 0 {
		return false
	}
	if _, ok := c.pending[key]; ok {
		return false
	}
	c.pending[key] = make(chan struct{})
	if raw != nil {
		if existing, ok := c.items[key]; ok {
			existing.raw = proto.Clone(raw).(*waE2E.Message)
			existing.expires = c.now().Add(viewOnceCacheTTL)
			c.items[key] = existing
		} else {
			for len(c.items) >= viewOnceCacheMaxItems {
				c.removeOldestLocked()
			}
			c.items[key] = viewOnceEntry{raw: proto.Clone(raw).(*waE2E.Message), expires: c.now().Add(viewOnceCacheTTL)}
			c.order = append(c.order, key)
		}
	}
	return true
}

func (c *viewOnceCache) finishPending(key viewOnceKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if done := c.pending[key]; done != nil {
		delete(c.pending, key)
		close(done)
	}
}

func (c *viewOnceCache) pruneLocked(now time.Time) {
	for _, key := range append([]viewOnceKey(nil), c.order...) {
		entry, ok := c.items[key]
		if ok && !entry.expires.After(now) {
			c.deleteLocked(key, entry)
		}
	}
}

func (c *viewOnceCache) removeOldestLocked() {
	if len(c.order) == 0 {
		return
	}
	key := c.order[0]
	c.order = c.order[1:]
	if entry, ok := c.items[key]; ok {
		c.bytes -= len(entry.data)
		delete(c.items, key)
	}
}

func (c *viewOnceCache) deleteLocked(key viewOnceKey, entry viewOnceEntry) {
	c.bytes -= len(entry.data)
	delete(c.items, key)
	for i, candidate := range c.order {
		if candidate == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

// CachedViewOnce returns both available bytes and the original message in one
// lookup so a pending download is only waited on once.
func (c *Client) CachedViewOnce(ctx context.Context, chat types.JID, id string) ([]byte, *waE2E.Message, bool) {
	if c == nil || c.viewOnce == nil {
		return nil, nil, false
	}
	entry, ok := c.viewOnce.get(ctx, chat, id)
	return entry.data, entry.raw, ok
}

// CachedViewOnceMessage returns the original descriptor without waiting for the
// media download. The parser uses this to hydrate Message.Media immediately.
func (c *Client) CachedViewOnceMessage(chat types.JID, id string) (*waE2E.Message, bool) {
	if c == nil || c.viewOnce == nil || id == "" {
		return nil, false
	}
	key := viewOnceKey{chat: chat.ToNonAD(), id: id}
	c.viewOnce.mu.Lock()
	defer c.viewOnce.mu.Unlock()
	c.viewOnce.pruneLocked(c.viewOnce.now())
	entry, ok := c.viewOnce.items[key]
	if !ok || entry.raw == nil {
		return nil, false
	}
	return proto.Clone(entry.raw).(*waE2E.Message), true
}

// SendCachedViewOnceAsNormal uploads the already downloaded plaintext media as
// a normal image, video, or audio message, quoted to the original command.
func (c *Client) SendCachedViewOnceAsNormal(ctx context.Context, to types.JID, chat types.JID, id string, quote *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	entry, ok := c.viewOnce.get(ctx, chat, id)
	if !ok || len(entry.data) == 0 {
		return whatsmeow.SendResponse{}, ErrViewOnceNotCached
	}
	source := message.UnwrapMessage(entry.raw)
	_, kind, _ := viewOnceDownloadable(source)
	if kind == "" {
		return whatsmeow.SendResponse{}, ErrViewOnceUnsupported
	}
	up, err := c.WA.Upload(ctx, entry.data, kind)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}
	return c.WA.SendMessage(ctx, to, normalViewOnceMedia(source, up, quote))
}

type viewOnceCacheFile struct {
	*os.File
	max int64
}

func (f *viewOnceCacheFile) Write(p []byte) (int, error) {
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	if offset < 0 || offset > f.max || int64(len(p)) > f.max-offset {
		return 0, errors.New("view-once cache media too large")
	}
	return f.File.Write(p)
}

func (f *viewOnceCacheFile) WriteAt(p []byte, offset int64) (int, error) {
	if offset < 0 || offset > f.max || int64(len(p)) > f.max-offset {
		return 0, errors.New("view-once cache media too large")
	}
	return f.File.WriteAt(p, offset)
}

func (f *viewOnceCacheFile) Truncate(size int64) error {
	if size < 0 || size > f.max {
		return errors.New("view-once cache media too large")
	}
	return f.File.Truncate(size)
}

func (f *viewOnceCacheFile) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{f}, r)
}
