package msgstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

// Tests never read .env or DATABASE_URL. The optional PostgreSQL run shares the
// existing CI disposable rpg_test database, but owns a separate schema per test.
func fixture(t *testing.T, opts Options) (*sqlx.DB, *Store) {
	t.Helper()
	driver, dsn := "sqlite", filepath.Join(t.TempDir(), "messages.db")
	if raw := os.Getenv("MESSAGE_STORE_TEST_POSTGRES_URL"); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path != "/rpg_test" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") {
			t.Fatal("test URL must name a disposable loopback rpg_test database")
		}
		admin, err := sqlx.Connect("postgres", raw)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(dsn))
		schema := fmt.Sprintf("msgstore_test_%x", hash[:12])
		if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
				t.Error(err)
			}
			admin.Close()
		})
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		driver, dsn = "postgres", u.String()
	}
	db, err := sqlx.Connect(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if opts.FlushInterval == 0 {
		opts.FlushInterval = time.Hour
	}
	s, err := New(context.Background(), db, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shutdown(t, s) })
	return db, s
}

func shutdown(t *testing.T, s *Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func event(id string) *events.Message {
	msg := &waE2E.Message{Conversation: proto.String("original " + id)}
	return &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{
		Chat: types.NewJID("123", types.HiddenUserServer), Sender: types.NewJID("123", types.HiddenUserServer), SenderAlt: types.NewJID("628123", types.DefaultUserServer),
	}, ID: id, Timestamp: time.Now()}, RawMessage: msg, Message: msg}
}

func key(e *events.Message) Key {
	fromMe := e.Info.IsFromMe
	return Key{Account: "bot", Chat: e.Info.Chat, Sender: e.Info.Sender, ID: e.Info.ID, FromMe: &fromMe}
}

func TestSnapshotLookupAliasesAndPersistence(t *testing.T) {
	db, s := fixture(t, Options{})
	e := event("image")
	e.RawMessage = &waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		DirectPath: proto.String("/media/original"), MediaKey: []byte("secret"), FileEncSHA256: []byte("hash"), JPEGThumbnail: []byte("thumbnail"),
	}}}}
	e.UnwrapRaw()
	if !s.Capture("bot", e) {
		t.Fatal("capture rejected")
	}
	e.Message.ImageMessage.MediaKey[0] = 'X'
	k := key(e)
	k.Chat = e.Info.SenderAlt
	k.Sender = e.Info.SenderAlt
	m, err := s.Get(context.Background(), k)
	if err != nil {
		t.Fatal(err)
	}
	got := (&events.Message{RawMessage: m.Raw}).UnwrapRaw()
	if !m.IsViewOnce || !got.IsViewOnce || string(got.Message.GetImageMessage().GetMediaKey()) != "secret" || string(got.Message.GetImageMessage().GetJPEGThumbnail()) != "thumbnail" {
		t.Fatalf("snapshot incomplete: %+v", m)
	}
	got.Message.ImageMessage.MediaKey[0] = 'Y'
	again, err := s.Get(context.Background(), k)
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Raw.GetViewOnceMessageV2().GetMessage().GetImageMessage().GetMediaKey()) != "secret" {
		t.Fatal("caller mutated cache")
	}
	shutdown(t, s)
	if s.Capture("bot", event("after-stop")) {
		t.Fatal("capture after shutdown")
	}
	restarted, err := New(context.Background(), db, Options{CacheBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown(t, restarted)
	m, err = restarted.Get(context.Background(), k)
	if err != nil || !m.IsViewOnce {
		t.Fatalf("restart lookup: %v, %+v", err, m)
	}
	k.Account = "other-bot"
	if _, err := restarted.Get(context.Background(), k); err != sql.ErrNoRows {
		t.Fatalf("cross-account lookup: %v", err)
	}
	k.Account = "bot"
	k.Chat = types.NewJID("other", types.HiddenUserServer)
	if _, err := restarted.Get(context.Background(), k); err != sql.ErrNoRows {
		t.Fatalf("cross-chat lookup: %v", err)
	}
	// A PN with the same number as a LID is never an inferred alias.
	k.Chat = types.NewJID("123", types.DefaultUserServer)
	if _, err := restarted.Get(context.Background(), k); err != sql.ErrNoRows {
		t.Fatalf("cross-namespace lookup: %v", err)
	}
}

func TestQueueLimitsAndPendingReadAfterCacheEviction(t *testing.T) {
	_, s := fixture(t, Options{QueueEntries: 2, CacheEntries: 1})
	e := event("first")
	if !s.Capture("bot", e) || !s.Capture("bot", event("second")) || s.Capture("bot", event("overflow")) {
		t.Fatal("queue entry limit")
	}
	if _, err := s.Get(context.Background(), key(e)); err != nil {
		t.Fatalf("pending message lost on cache eviction: %v", err)
	}
	if stats := s.Stats(); stats.PendingEntries != 2 || stats.CacheEntries != 1 || stats.Dropped != 1 {
		t.Fatalf("stats: %+v", stats)
	}
	shutdown(t, s)
	if stats := s.Stats(); stats.PendingEntries != 0 || stats.PendingBytes != 0 || stats.Written != 2 {
		t.Fatalf("flush: %+v", stats)
	}
}

func TestByteAndMessageLimits(t *testing.T) {
	_, s := fixture(t, Options{QueueBytes: 1400, CacheBytes: 1, MaxMessageBytes: 100})
	large := event("large")
	large.Message.Conversation = proto.String(string(make([]byte, 101)))
	if s.Capture("bot", large) {
		t.Fatal("oversize admitted")
	}
	if !s.Capture("bot", event("one")) || s.Capture("bot", event("two")) {
		t.Fatal("byte budget not enforced")
	}
	if st := s.Stats(); st.PendingBytes > 1400 || st.CacheBytes != 0 {
		t.Fatalf("budgets: %+v", st)
	}
}

func TestOriginalSurvivesRevokeEditDuplicateAndFilters(t *testing.T) {
	_, s := fixture(t, Options{})
	e := event("same")
	s.Capture("bot", e)
	duplicate := event("same")
	duplicate.Message.Conversation = proto.String("replacement")
	s.Capture("bot", duplicate)
	edit := event("same")
	edit.IsEdit = true
	revoke := event("same")
	revoke.Message = &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum()}}
	revoke.RawMessage = revoke.Message
	status := event("status")
	status.Info.Chat = types.StatusBroadcastJID
	nl := event("newsletter")
	nl.Info.Chat = types.NewJID("123", types.NewsletterServer)
	old := event("old")
	old.Info.Timestamp = time.Now().Add(-25 * time.Hour)
	reaction := event("reaction")
	reaction.Message = &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{}}
	for _, filtered := range []*events.Message{edit, revoke, status, nl, old, reaction} {
		if s.Capture("bot", filtered) {
			t.Fatalf("filtered event admitted: %s", filtered.Info.ID)
		}
	}
	m, err := s.Get(context.Background(), key(e))
	if err != nil || m.Raw.GetConversation() != "original same" {
		t.Fatalf("original overwritten: %v", err)
	}
	shutdown(t, s)
	if st := s.Stats(); st.Written != 1 || st.Skipped != 6 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestSenderKeyDistributionDoesNotOccupyRealMessageID(t *testing.T) {
	db, s := fixture(t, Options{})
	e := event("shared-id")
	e.Message = &waE2E.Message{SenderKeyDistributionMessage: &waE2E.SenderKeyDistributionMessage{AxolotlSenderKeyDistributionMessage: []byte("group key")}, MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: []byte("metadata")}}
	e.RawMessage = e.Message
	if s.Capture("bot", e) {
		t.Fatal("encryption-only event occupied a message ID")
	}
	actual := event("shared-id")
	actual.Message.SenderKeyDistributionMessage = &waE2E.SenderKeyDistributionMessage{AxolotlSenderKeyDistributionMessage: []byte("group key")}
	if !s.Capture("bot", actual) {
		t.Fatal("real content with sender-key sidecar was dropped")
	}
	shutdown(t, s)
	restarted, err := New(context.Background(), db, Options{CacheBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown(t, restarted)
	m, err := restarted.Get(context.Background(), key(actual))
	if err != nil || m.Raw.GetConversation() != "original shared-id" {
		t.Fatalf("message body lost after key event: %+v %v", m, err)
	}
	if st := s.Stats(); st.Skipped != 1 || st.Written != 1 {
		t.Fatalf("unexpected store stats: %+v", st)
	}
}

func TestAmbiguousAndDirectionIsolation(t *testing.T) {
	_, s := fixture(t, Options{})
	e := event("same")
	s.Capture("bot", e)
	other := event("same")
	other.Info.IsFromMe = true
	other.Message.Conversation = proto.String("outgoing")
	s.Capture("bot", other)
	k := key(e)
	k.FromMe = nil
	if _, err := s.Get(context.Background(), k); err != ErrAmbiguous {
		t.Fatalf("ambiguous key: %v", err)
	}
	m, err := s.Get(context.Background(), key(other))
	if err != nil || m.Raw.GetConversation() != "outgoing" {
		t.Fatalf("direction lookup: %v", err)
	}
}

func TestAlbumMergesPendingAndDatabaseInOrder(t *testing.T) {
	_, s := fixture(t, Options{CacheEntries: 1})
	makeChild := func(id string, index int32) *events.Message {
		e := event(id)
		e.Message.MessageContextInfo = &waE2E.MessageContextInfo{MessageAssociation: &waE2E.MessageAssociation{AssociationType: waE2E.MessageAssociation_MEDIA_ALBUM.Enum(), ParentMessageKey: &waCommon.MessageKey{ID: proto.String("album")}, MessageIndex: proto.Int32(index)}}
		return e
	}
	e := makeChild("second", 2)
	s.Capture("bot", e)
	s.flush() // Commit one child; keep the other pending.
	s.Capture("bot", makeChild("first", 1))
	k := key(e)
	k.Chat = e.Info.SenderAlt
	k.Sender = e.Info.SenderAlt
	album, err := s.Album(context.Background(), k, "album", 100)
	if err != nil || len(album) != 2 || album[0].Info.ID != "first" || album[1].Info.ID != "second" {
		t.Fatalf("album merge: %+v, %v", album, err)
	}
}

func TestRetentionFiltersReadsAndPrunesBoundedBatch(t *testing.T) {
	db, s := fixture(t, Options{PruneBatch: 1, CacheBytes: 1})
	for _, id := range []string{"one", "two", "three"} {
		s.Capture("bot", event(id))
	}
	shutdown(t, s)
	if _, err := db.Exec("UPDATE bot_messages SET expires_at=0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), key(event("one"))); err != sql.ErrNoRows {
		t.Fatalf("expired read: %v", err)
	}
	if err := s.prune(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM bot_messages"); err != nil || count != 2 {
		t.Fatalf("bounded prune: %d, %v", count, err)
	}
}

func TestCaptureDoesNotWaitForDatabaseAndFailureIsBounded(t *testing.T) {
	db, s := fixture(t, Options{BatchSize: 1, DBTimeout: 30 * time.Millisecond, FlushInterval: time.Hour})
	// Hold the only pool connection: worker cannot write, Capture must still return.
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e := event("blocked")
	done := make(chan bool, 1)
	go func() { done <- s.Capture("bot", e) }()
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("capture rejected")
		}
	case <-time.After(time.Second):
		conn.Close()
		t.Fatal("Capture waited for DB")
	}
	deadline := time.Now().Add(time.Second)
	for s.Stats().WriteErrors == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if st := s.Stats(); st.WriteErrors != 1 || st.PendingEntries != 0 {
		conn.Close()
		t.Fatalf("unbounded failed write: %+v", st)
	}
	if _, err := s.Get(context.Background(), key(e)); err != nil {
		conn.Close()
		t.Fatalf("cache lost on DB failure: %v", err)
	}
	conn.Close()
	if !s.Capture("bot", event("recovered")) {
		t.Fatal("store did not recover")
	}
	shutdown(t, s)
	if s.Stats().Written != 1 {
		t.Fatal("recovered batch not written")
	}
}

func TestConcurrentCaptureLookupAndShutdown(t *testing.T) {
	_, s := fixture(t, Options{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			for j := 0; j < 20; j++ {
				e := event(fmt.Sprintf("%d-%d", i, j))
				if s.Capture("bot", e) {
					_, _ = s.Get(context.Background(), key(e))
				}
			}
		})
	}
	wg.Wait()
	shutdown(t, s)
	if st := s.Stats(); st.Accepted != 160 || st.Written != 160 || st.PendingEntries != 0 {
		t.Fatalf("concurrent loss: %+v", st)
	}
}

func TestShutdownDeadlineCancelsWriterAndAccountsForPending(t *testing.T) {
	db, s := fixture(t, Options{BatchSize: 1, QueueEntries: 2, DBTimeout: time.Hour})
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if !s.Capture("bot", event("one")) || !s.Capture("bot", event("two")) || s.Capture("bot", event("three")) {
		t.Fatal("pending/in-flight bound")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Shutdown(ctx); err != context.Canceled {
		t.Fatalf("deadline: %v", err)
	}
	select {
	case <-s.done:
	case <-time.After(time.Second):
		t.Fatal("writer did not stop after query cancellation")
	}
	if st := s.Stats(); st.PendingEntries != 0 || st.PendingBytes != 0 || st.Dropped != 3 {
		t.Fatalf("abandoned messages unaccounted: %+v", st)
	}
}
