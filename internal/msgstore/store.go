// Package msgstore keeps a bounded, best-effort history of live message events.
// It never downloads attachments or waits for database I/O on Capture.
package msgstore

import (
	"container/list"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

var ErrAmbiguous = errors.New("message key matches multiple senders or directions")

type Options struct {
	Retention, FlushInterval, DBTimeout, PruneInterval  time.Duration
	CacheBytes, QueueBytes, MaxMessageBytes, BatchBytes int
	CacheEntries, QueueEntries, BatchSize, PruneBatch   int
}

func (o Options) defaults() Options {
	if o.Retention <= 0 {
		o.Retention = 24 * time.Hour
	}
	if o.FlushInterval <= 0 {
		o.FlushInterval = time.Second
	}
	if o.DBTimeout <= 0 {
		o.DBTimeout = 2 * time.Second
	}
	if o.PruneInterval <= 0 {
		o.PruneInterval = 5 * time.Minute
	}
	if o.CacheBytes <= 0 {
		o.CacheBytes = 4 << 20
	}
	if o.QueueBytes <= 0 {
		o.QueueBytes = 4 << 20
	}
	if o.MaxMessageBytes <= 0 {
		o.MaxMessageBytes = 128 << 10
	}
	if o.BatchBytes <= 0 {
		o.BatchBytes = 512 << 10
	}
	if o.CacheEntries <= 0 {
		o.CacheEntries = 2000
	}
	if o.QueueEntries <= 0 {
		o.QueueEntries = 1000
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 50
	}
	if o.BatchSize > 50 {
		o.BatchSize = 50
	} // Also fits SQLite's historical bind limit.
	if o.PruneBatch <= 0 {
		o.PruneBatch = 1000
	}
	return o
}

// Key permits verified PN/LID alternatives. Empty Sender/FromMe mean unknown;
// an ambiguous result is an error, never an arbitrary message from another user.
type Key struct {
	Account                          string
	Chat, ChatAlt, Sender, SenderAlt types.JID
	ID                               string
	FromMe                           *bool
}

type Message struct {
	Info       types.MessageInfo
	Raw        *waE2E.Message
	IsViewOnce bool
	AlbumID    string
	AlbumIndex int
}

type row struct {
	Account     string `db:"account"`
	Chat        string `db:"chat"`
	ChatAlt     string `db:"chat_alt"`
	ID          string `db:"message_id"`
	Sender      string `db:"sender"`
	SenderAlt   string `db:"sender_alt"`
	FromMe      bool   `db:"from_me"`
	IsGroup     bool   `db:"is_group"`
	Timestamp   int64  `db:"message_time"`
	Expires     int64  `db:"expires_at"`
	ViewOnce    bool   `db:"view_once"`
	AlbumID     string `db:"album_id"`
	AlbumIndex  int    `db:"album_index"`
	Payload     []byte `db:"payload"`
	QuoteID     string `db:"-"`
	QuoteSender string `db:"-"`
	lru         *list.Element
	pending     bool
}

type rowKey struct {
	account, chat, id, sender string
	fromMe                    bool
}
type lookupKey struct{ account, chat, id string }

func (r *row) key() rowKey { return rowKey{r.Account, r.Chat, r.ID, r.Sender, r.FromMe} }

// Include a conservative allowance for metadata/index/list allocations. This is
// an accounting budget, not a claim about the Go runtime's exact heap usage.
func (r *row) size() int {
	return len(r.Payload) + len(r.Account) + len(r.Chat) + len(r.ChatAlt) + len(r.ID) + len(r.Sender) + len(r.SenderAlt) + len(r.AlbumID) + len(r.QuoteID) + len(r.QuoteSender) + 1024
}

type Stats struct {
	UnavailableViewOnce                                                             uint64
	Accepted, Skipped, Dropped, Written, WriteErrors, ReadErrors, CacheHits, DBHits uint64
	PendingEntries, PendingBytes, CacheEntries, CacheBytes                          int
	LastWriteDuration                                                               time.Duration
	DatabaseBytes, TableBytes                                                       int64 // PostgreSQL samples; zero until first sample.
}

type Store struct {
	db               *sqlx.DB
	opts             Options
	mu               sync.Mutex
	index            map[lookupKey]map[rowKey]*row
	quotes           map[lookupKey]map[rowKey]*row
	lru              list.List
	queue            []*row
	stats            Stats
	closing          bool
	wake, stop, done chan struct{}
	ctx              context.Context
	cancel           context.CancelFunc
	retryAfter       time.Time // Worker-owned; backs off after a failed batch.
}

func New(ctx context.Context, db *sqlx.DB, opts Options) (*Store, error) {
	if db.DriverName() != "postgres" && db.DriverName() != "sqlite" {
		return nil, fmt.Errorf("unsupported message store driver")
	}
	s := &Store{db: db, opts: opts.defaults(), index: make(map[lookupKey]map[rowKey]*row), wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
	s.quotes = make(map[lookupKey]map[rowKey]*row)
	if err := s.migrate(ctx); err != nil {
		return nil, err
	}
	// Independent of the application context: SIGTERM must still allow a flush.
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return s, nil
}

func jid(j types.JID) string {
	if j.IsEmpty() {
		return ""
	}
	return j.ToNonAD().String()
}

func chatAlt(info types.MessageInfo) types.JID {
	if info.Chat.Server != types.DefaultUserServer && info.Chat.Server != types.HiddenUserServer {
		return types.EmptyJID
	}
	if info.IsFromMe {
		return info.RecipientAlt
	}
	return info.SenderAlt
}

// Capture snapshots protobuf bytes before any command can mutate the event.
// A false return means filtered, oversized, expired, closing, or overloaded.
// Pending limits include the batch currently being written to the database.
func (s *Store) Capture(account string, evt *events.Message) bool {
	if s == nil || evt == nil {
		return false
	}
	msg := evt.Message
	raw := evt.RawMessage
	if raw == nil {
		raw = msg
	}
	now := time.Now()
	timestamp := evt.Info.Timestamp
	if timestamp.IsZero() {
		timestamp = now
	}
	if timestamp.After(now) {
		timestamp = now
	}
	if account == "" || evt.Info.ID == "" || evt.Info.Chat.IsEmpty() || evt.Info.Sender.IsEmpty() || raw == nil || msg == nil ||
		evt.Info.Chat.Server == types.BroadcastServer || evt.Info.Chat.Server == types.NewsletterServer ||
		evt.IsEdit || msg.GetProtocolMessage() != nil || msg.GetReactionMessage() != nil || msg.GetEncReactionMessage() != nil ||
		!timestamp.Add(s.opts.Retention).After(now) || proto.Size(raw) == 0 || proto.Size(raw) > s.opts.MaxMessageBytes {
		s.mu.Lock()
		s.stats.Skipped++
		s.mu.Unlock()
		return false
	}
	r := &row{Account: account, Chat: jid(evt.Info.Chat), ChatAlt: jid(chatAlt(evt.Info)), ID: evt.Info.ID, Sender: jid(evt.Info.Sender), SenderAlt: jid(evt.Info.SenderAlt), FromMe: evt.Info.IsFromMe, IsGroup: evt.Info.IsGroup, Timestamp: timestamp.UnixMilli(), Expires: timestamp.Add(s.opts.Retention).UnixMilli(), ViewOnce: evt.IsViewOnce || msg.GetImageMessage().GetViewOnce() || msg.GetVideoMessage().GetViewOnce() || msg.GetAudioMessage().GetViewOnce()}
	if ci, _ := downloadableViewOnceQuote(msg); ci != nil {
		r.QuoteID = ci.GetStanzaID()
		r.QuoteSender = quoteSender(ci)
	}
	association := msg.GetMessageContextInfo().GetMessageAssociation()
	if association.GetAssociationType() == waE2E.MessageAssociation_MEDIA_ALBUM {
		r.AlbumID = association.GetParentMessageKey().GetID()
		r.AlbumIndex = int(association.GetMessageIndex())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		s.stats.Dropped++
		return false
	}
	if old := s.index[lookupKey{account, r.Chat, r.ID}][r.key()]; old != nil {
		return true
	}
	// Reserve capacity before allocating a serialized copy. No per-message goroutine.
	if s.stats.PendingEntries >= s.opts.QueueEntries || s.stats.PendingBytes+proto.Size(raw)+r.size() > s.opts.QueueBytes {
		s.stats.Dropped++
		return false
	}
	var err error
	r.Payload, err = proto.Marshal(raw)
	if err != nil {
		s.stats.Dropped++
		return false
	}
	r.pending = true
	s.addIndex(r)
	s.queue = append(s.queue, r)
	s.stats.PendingEntries++
	s.stats.PendingBytes += r.size()
	s.stats.Accepted++
	s.cache(r)
	if len(s.queue) >= s.opts.BatchSize || s.stats.PendingBytes >= s.opts.BatchBytes {
		s.signal()
	}
	return true
}

func (s *Store) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Store) addIndex(r *row) {
	for _, chat := range []string{r.Chat, r.ChatAlt} {
		if chat == "" {
			continue
		}
		k := lookupKey{r.Account, chat, r.ID}
		if s.index[k] == nil {
			s.index[k] = make(map[rowKey]*row)
		}
		s.index[k][r.key()] = r
		if r.QuoteID != "" {
			qk := lookupKey{r.Account, chat, r.QuoteID}
			if s.quotes[qk] == nil {
				s.quotes[qk] = make(map[rowKey]*row)
			}
			s.quotes[qk][r.key()] = r
		}
	}
}
func (s *Store) forget(r *row) {
	if r.pending || r.lru != nil {
		return
	}
	for _, chat := range []string{r.Chat, r.ChatAlt} {
		k := lookupKey{r.Account, chat, r.ID}
		delete(s.index[k], r.key())
		if len(s.index[k]) == 0 {
			delete(s.index, k)
		}
		qk := lookupKey{r.Account, chat, r.QuoteID}
		delete(s.quotes[qk], r.key())
		if len(s.quotes[qk]) == 0 {
			delete(s.quotes, qk)
		}
	}
}
func (s *Store) cache(r *row) {
	if r.size() > s.opts.CacheBytes {
		return
	}
	r.lru = s.lru.PushFront(r)
	s.stats.CacheEntries++
	s.stats.CacheBytes += r.size()
	for s.stats.CacheEntries > s.opts.CacheEntries || s.stats.CacheBytes > s.opts.CacheBytes {
		old := s.lru.Back().Value.(*row)
		s.lru.Remove(old.lru)
		old.lru = nil
		s.stats.CacheEntries--
		s.stats.CacheBytes -= old.size()
		s.forget(old)
	}
}

func matches(r *row, k Key, now int64) bool {
	if r.Expires <= now || r.Account != k.Account || r.ID != k.ID {
		return false
	}
	if k.FromMe != nil && *k.FromMe != r.FromMe {
		return false
	}
	if !k.Sender.IsEmpty() || !k.SenderAlt.IsEmpty() {
		a, b := jid(k.Sender), jid(k.SenderAlt)
		if !(a != "" && (a == r.Sender || a == r.SenderAlt)) && !(b != "" && (b == r.Sender || b == r.SenderAlt)) {
			return false
		}
	}
	return true
}

func decode(r *row) (*Message, error) {
	raw := &waE2E.Message{}
	if err := proto.Unmarshal(r.Payload, raw); err != nil {
		return nil, err
	}
	chat, _ := types.ParseJID(r.Chat)
	sender, _ := types.ParseJID(r.Sender)
	alt, _ := types.ParseJID(r.SenderAlt)
	info := types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: sender, SenderAlt: alt, IsFromMe: r.FromMe, IsGroup: r.IsGroup}, ID: r.ID, Timestamp: time.UnixMilli(r.Timestamp)}
	return &Message{Info: info, Raw: raw, IsViewOnce: r.ViewOnce, AlbumID: r.AlbumID, AlbumIndex: r.AlbumIndex}, nil
}

// Get returns sql.ErrNoRows for a miss. Returned messages are independent copies.
func (s *Store) Get(ctx context.Context, k Key) (*Message, error) {
	now := time.Now().UnixMilli()
	s.mu.Lock()
	var found *row
	for _, chat := range []string{jid(k.Chat), jid(k.ChatAlt)} {
		for _, r := range s.index[lookupKey{k.Account, chat, k.ID}] {
			if matches(r, k, now) {
				if found != nil && found != r {
					s.mu.Unlock()
					return nil, ErrAmbiguous
				}
				found = r
			}
		}
	}
	// Incomplete keys need a DB check as well: a second sender/direction might
	// have been evicted from the cache while another match remains resident.
	if found != nil && !k.Sender.IsEmpty() && k.FromMe != nil {
		if found.lru != nil {
			s.lru.MoveToFront(found.lru)
		}
		s.stats.CacheHits++
		s.mu.Unlock()
		return decode(found)
	}
	s.mu.Unlock()
	query, args := selectKey(k, now)
	query += " LIMIT 2"
	readCtx, cancel := context.WithTimeout(ctx, s.opts.DBTimeout)
	defer cancel()
	var rows []*row
	if err := s.db.SelectContext(readCtx, &rows, s.db.Rebind(query), args...); err != nil {
		s.mu.Lock()
		s.stats.ReadErrors++
		s.mu.Unlock()
		return nil, err
	}
	if len(rows) == 0 {
		if found != nil {
			return decode(found)
		}
		return nil, sql.ErrNoRows
	}
	if len(rows) != 1 {
		return nil, ErrAmbiguous
	}
	if found != nil && found.key() != rows[0].key() {
		return nil, ErrAmbiguous
	}
	s.mu.Lock()
	s.stats.DBHits++
	s.mu.Unlock()
	return decode(rows[0])
}

func selectKey(k Key, now int64) (string, []any) {
	query := "SELECT * FROM bot_messages WHERE account=? AND message_id=? AND expires_at>? AND (chat IN (?,?) OR chat_alt IN (?,?))"
	args := []any{k.Account, k.ID, now, jid(k.Chat), jid(k.ChatAlt), jid(k.Chat), jid(k.ChatAlt)}
	// Empty alternatives must never match the default empty chat_alt column.
	for _, i := range []int{3, 4, 5, 6} {
		if args[i] == "" {
			args[i] = "!"
		}
	}
	if !k.Sender.IsEmpty() || !k.SenderAlt.IsEmpty() {
		a, b := jid(k.Sender), jid(k.SenderAlt)
		if a == "" {
			a = "!"
		}
		if b == "" {
			b = "!"
		}
		query += " AND (sender IN (?,?) OR sender_alt IN (?,?))"
		args = append(args, a, b, a, b)
	}
	if k.FromMe != nil {
		query += " AND from_me=?"
		args = append(args, *k.FromMe)
	}
	return query, args
}

// Album merges committed rows and pending/cache messages. Results may be partial
// while WhatsApp is still delivering the album. limit is clamped to 100.
func (s *Store) Album(ctx context.Context, k Key, albumID string, limit int) ([]*Message, error) {
	if albumID == "" {
		return nil, fmt.Errorf("album ID is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	k.ID = "" // Select the album's children, not an individual message ID.
	query, args := selectKey(k, time.Now().UnixMilli())
	query = strings.Replace(query, "message_id=?", "album_id=?", 1)
	args[1] = albumID
	query += " ORDER BY album_index,message_time,message_id LIMIT ?"
	args = append(args, limit)
	readCtx, cancel := context.WithTimeout(ctx, s.opts.DBTimeout)
	defer cancel()
	var rows []*row
	if err := s.db.SelectContext(readCtx, &rows, s.db.Rebind(query), args...); err != nil {
		return nil, err
	}
	merged := make(map[rowKey]*row)
	for _, r := range rows {
		merged[r.key()] = r
	}
	s.mu.Lock()
	for key, group := range s.index {
		if key.account != k.Account || (key.chat != jid(k.Chat) && key.chat != jid(k.ChatAlt)) {
			continue
		}
		for _, r := range group {
			candidate := k
			candidate.ID = r.ID
			if r.AlbumID == albumID && matches(r, candidate, time.Now().UnixMilli()) {
				merged[r.key()] = r
			}
		}
	}
	s.mu.Unlock()
	rows = rows[:0]
	for _, r := range merged {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AlbumIndex != rows[j].AlbumIndex {
			return rows[i].AlbumIndex < rows[j].AlbumIndex
		}
		if rows[i].Timestamp != rows[j].Timestamp {
			return rows[i].Timestamp < rows[j].Timestamp
		}
		return rows[i].ID < rows[j].ID
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	result := make([]*Message, 0, len(rows))
	for _, r := range rows {
		m, err := decode(r)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, nil
}

func (s *Store) Stats() Stats { s.mu.Lock(); defer s.mu.Unlock(); return s.stats }

func (s *Store) NoteUnavailableViewOnce() { s.mu.Lock(); s.stats.UnavailableViewOnce++; s.mu.Unlock() }

func (s *Store) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.closing {
		s.closing = true
		close(s.stop)
	}
	s.mu.Unlock()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		s.cancel()
		return ctx.Err()
	}
}

func (s *Store) takeBatch() []*row {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, size := 0, 0
	for n < len(s.queue) && n < s.opts.BatchSize {
		next := s.queue[n].size()
		if n > 0 && size+next > s.opts.BatchBytes {
			break
		}
		size += next
		n++
	}
	batch := append([]*row(nil), s.queue[:n]...)
	copy(s.queue, s.queue[n:])
	clear(s.queue[len(s.queue)-n:])
	s.queue = s.queue[:len(s.queue)-n]
	return batch
}

func (s *Store) flush() bool {
	batch := s.takeBatch()
	if len(batch) == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(s.ctx, s.opts.DBTimeout)
	start := time.Now()
	written, err := s.write(ctx, batch)
	cancel()
	s.mu.Lock()
	s.stats.LastWriteDuration = time.Since(start)
	if err != nil {
		s.stats.WriteErrors++
		s.stats.Dropped += uint64(len(batch))
	} else {
		s.stats.Written += uint64(written)
	}
	for _, r := range batch {
		r.pending = false
		s.stats.PendingEntries--
		s.stats.PendingBytes -= r.size()
		s.forget(r)
	}
	if err == nil && len(s.queue) >= s.opts.BatchSize {
		s.signal()
	}
	s.mu.Unlock()
	if err != nil && s.ctx.Err() == nil {
		s.retryAfter = time.Now().Add(s.opts.FlushInterval)
		log.Printf("message store: batch of %d not persisted: %v", len(batch), err)
	}
	return true
}

func (s *Store) run() {
	defer close(s.done)
	defer s.cancel()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, r := range s.queue {
			r.pending = false
			s.stats.Dropped++
			s.stats.PendingEntries--
			s.stats.PendingBytes -= r.size()
			s.forget(r)
		}
		s.queue = nil
	}()
	ticker := time.NewTicker(s.opts.FlushInterval)
	defer ticker.Stop()
	prune := time.NewTicker(s.opts.PruneInterval)
	defer prune.Stop()
	metrics := time.NewTicker(time.Minute)
	defer metrics.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.stop:
			for s.ctx.Err() == nil && s.flush() {
			}
			return
		case <-ticker.C:
			if !time.Now().Before(s.retryAfter) {
				s.flush()
			}
		case <-s.wake:
			if !time.Now().Before(s.retryAfter) {
				s.flush()
			}
		case <-prune.C:
			ctx, cancel := context.WithTimeout(s.ctx, s.opts.DBTimeout)
			err := s.prune(ctx)
			cancel()
			if err != nil && s.ctx.Err() == nil {
				log.Printf("message store cleanup: %v", err)
			}
		case <-metrics.C:
			if s.db.DriverName() == "postgres" {
				ctx, cancel := context.WithTimeout(s.ctx, s.opts.DBTimeout)
				var dbBytes, tableBytes int64
				err := s.db.QueryRowContext(ctx, `SELECT pg_database_size(current_database()), pg_total_relation_size('bot_messages'::regclass)+pg_total_relation_size('bot_message_quotes'::regclass)`).Scan(&dbBytes, &tableBytes)
				cancel()
				if err == nil {
					s.mu.Lock()
					s.stats.DatabaseBytes = dbBytes
					s.stats.TableBytes = tableBytes
					s.mu.Unlock()
				}
			}
			st := s.Stats()
			if st.UnavailableViewOnce > 0 {
				log.Printf("message store view-once: unavailable=%d", st.UnavailableViewOnce)
			}
			pool := s.db.Stats()
			log.Printf("message store: accepted=%d skipped=%d dropped=%d written=%d write_errors=%d pending=%d pending_bytes=%d cache_bytes=%d cache_hits=%d db_hits=%d read_errors=%d last_write=%s db_in_use=%d db_wait_count=%d db_wait_duration=%s", st.Accepted, st.Skipped, st.Dropped, st.Written, st.WriteErrors, st.PendingEntries, st.PendingBytes, st.CacheBytes, st.CacheHits, st.DBHits, st.ReadErrors, st.LastWriteDuration, pool.InUse, pool.WaitCount, pool.WaitDuration)
			if s.db.DriverName() == "postgres" {
				log.Printf("message store storage: database_bytes=%d table_and_indexes_bytes=%d", st.DatabaseBytes, st.TableBytes)
			}
		}
	}
}
