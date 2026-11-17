package msgstore

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// Quotes are observations, not authenticated original messages. Keep their
// provenance separate and never insert them under the original message's key.
func downloadableViewOnceQuote(msg *waE2E.Message) (*waE2E.ContextInfo, *waE2E.Message) {
	var ci *waE2E.ContextInfo
	switch {
	case msg.GetExtendedTextMessage() != nil:
		ci = msg.GetExtendedTextMessage().GetContextInfo()
	case msg.GetImageMessage() != nil:
		ci = msg.GetImageMessage().GetContextInfo()
	case msg.GetVideoMessage() != nil:
		ci = msg.GetVideoMessage().GetContextInfo()
	case msg.GetAudioMessage() != nil:
		ci = msg.GetAudioMessage().GetContextInfo()
	case msg.GetStickerMessage() != nil:
		ci = msg.GetStickerMessage().GetContextInfo()
	case msg.GetDocumentMessage() != nil:
		ci = msg.GetDocumentMessage().GetContextInfo()
	default:
		return nil, nil
	}
	sender, err := types.ParseJID(ci.GetParticipant())
	if ci.GetStanzaID() == "" || err != nil || sender.IsEmpty() || ci.GetRemoteJID() != "" {
		return nil, nil
	}
	evt := (&events.Message{RawMessage: ci.GetQuotedMessage()}).UnwrapRaw()
	q := evt.Message
	var media whatsmeow.DownloadableMessage
	switch {
	case q.GetImageMessage() != nil:
		media = q.GetImageMessage()
	case q.GetVideoMessage() != nil:
		media = q.GetVideoMessage()
	case q.GetAudioMessage() != nil:
		media = q.GetAudioMessage()
	default:
		return nil, nil
	}
	if !evt.IsViewOnce && !q.GetImageMessage().GetViewOnce() && !q.GetVideoMessage().GetViewOnce() && !q.GetAudioMessage().GetViewOnce() {
		return nil, nil
	}
	if media.GetDirectPath() == "" || len(media.GetMediaKey()) != 32 || len(media.GetFileSHA256()) != 32 || len(media.GetFileEncSHA256()) != 32 {
		return nil, nil
	}
	return ci, ci.GetQuotedMessage()
}

func quoteMatches(r *row, k Key, now int64) bool {
	return r.Expires > now && r.Account == k.Account && r.QuoteID == k.ID &&
		(jid(k.Sender) == r.QuoteSender || (!k.SenderAlt.IsEmpty() && jid(k.SenderAlt) == r.QuoteSender))
}

func quoteSender(ci *waE2E.ContextInfo) string {
	sender, _ := types.ParseJID(ci.GetParticipant())
	return jid(sender)
}

func decodeQuote(r *row, k Key) (*Message, error) {
	var raw waE2E.Message
	if err := proto.Unmarshal(r.Payload, &raw); err != nil {
		return nil, err
	}
	m := (&events.Message{RawMessage: &raw}).UnwrapRaw().Message
	ci, q := downloadableViewOnceQuote(m)
	if ci == nil || ci.GetStanzaID() != k.ID {
		return nil, sql.ErrNoRows
	}
	sender, _ := types.ParseJID(ci.GetParticipant())
	if jid(sender) != jid(k.Sender) && (k.SenderAlt.IsEmpty() || jid(sender) != jid(k.SenderAlt)) {
		return nil, sql.ErrNoRows
	}
	chat, _ := types.ParseJID(r.Chat)
	fromMe := k.FromMe != nil && *k.FromMe
	return &Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: sender, IsFromMe: fromMe, IsGroup: r.IsGroup}, ID: k.ID}, Raw: q, IsViewOnce: true}, nil
}

// GetQuotedMedia finds a complete view-once descriptor inside another received
// reply in the same chat. It cannot recover data WhatsApp never delivered.
// A known target sender is required; expiry follows the observing reply.
func (s *Store) GetQuotedMedia(ctx context.Context, k Key) (*Message, error) {
	if k.Account == "" || k.Chat.IsEmpty() || k.ID == "" || k.Sender.IsEmpty() {
		return nil, sql.ErrNoRows
	}
	now := time.Now().UnixMilli()
	s.mu.Lock()
	var found *row
	for _, chat := range []string{jid(k.Chat), jid(k.ChatAlt)} {
		for _, r := range s.quotes[lookupKey{k.Account, chat, k.ID}] {
			if quoteMatches(r, k, now) && (found == nil || r.Timestamp > found.Timestamp) {
				found = r
			}
		}
	}
	if found != nil {
		s.stats.CacheHits++
		s.mu.Unlock()
		return decodeQuote(found, k)
	}
	s.mu.Unlock()
	a, b := jid(k.Sender), jid(k.SenderAlt)
	if b == "" {
		b = "!"
	}
	chat, alt := jid(k.Chat), jid(k.ChatAlt)
	if alt == "" {
		alt = "!"
	}
	q := `SELECT m.* FROM bot_message_quotes q JOIN bot_messages m
	 ON m.account=q.account AND m.chat=q.chat AND m.message_id=q.source_id AND m.sender=q.source_sender AND m.from_me=q.source_from_me
	 WHERE q.account=? AND q.target_id=? AND q.target_sender IN (?,?) AND m.expires_at>?
	 AND (m.chat IN (?,?) OR m.chat_alt IN (?,?)) ORDER BY m.message_time DESC LIMIT 4`
	readCtx, cancel := context.WithTimeout(ctx, s.opts.DBTimeout)
	defer cancel()
	var rows []*row
	if err := s.db.SelectContext(readCtx, &rows, s.db.Rebind(q), k.Account, k.ID, a, b, now, chat, alt, chat, alt); err != nil {
		s.mu.Lock()
		s.stats.ReadErrors++
		s.mu.Unlock()
		return nil, err
	}
	for _, r := range rows {
		m, err := decodeQuote(r, k)
		if err == nil {
			s.mu.Lock()
			s.stats.DBHits++
			s.mu.Unlock()
			return m, nil
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
	}
	return nil, sql.ErrNoRows
}

type quoteRef struct {
	account, chat, sourceID, sourceSender, targetID, targetSender string
	fromMe                                                        bool
}

func ref(r *row) quoteRef {
	return quoteRef{r.Account, r.Chat, r.ID, r.Sender, r.QuoteID, r.QuoteSender, r.FromMe}
}

func writeQuoteRefs(ctx context.Context, tx *sqlx.Tx, refs []quoteRef) error {
	for len(refs) > 0 {
		n := len(refs)
		if n > 50 {
			n = 50
		}
		var values []string
		var args []any
		for _, r := range refs[:n] {
			values = append(values, "(?,?,?,?,?,?,?)")
			args = append(args, r.account, r.chat, r.sourceID, r.sourceSender, r.fromMe, r.targetID, r.targetSender)
		}
		q := `INSERT INTO bot_message_quotes(account,chat,source_id,source_sender,source_from_me,target_id,target_sender) VALUES ` + strings.Join(values, ",") + ` ON CONFLICT(account,chat,source_id,source_sender,source_from_me) DO NOTHING`
		if _, err := tx.ExecContext(ctx, tx.Rebind(q), args...); err != nil {
			return err
		}
		refs = refs[n:]
	}
	return nil
}

// One-time, bounded upgrade: index at most the latest 256 unexpired stored
// events so recent complete quotes are useful immediately after deployment.
func backfillQuotes(ctx context.Context, tx *sqlx.Tx) error {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`SELECT * FROM bot_messages WHERE expires_at>? ORDER BY expires_at DESC LIMIT 256`), time.Now().UnixMilli())
	if err != nil {
		return err
	}
	var refs []quoteRef
	for rows.Next() {
		var r row
		if err := rows.StructScan(&r); err != nil {
			rows.Close()
			return err
		}
		var raw waE2E.Message
		if proto.Unmarshal(r.Payload, &raw) != nil {
			continue
		}
		m := (&events.Message{RawMessage: &raw}).UnwrapRaw().Message
		if ci, _ := downloadableViewOnceQuote(m); ci != nil {
			r.QuoteID = ci.GetStanzaID()
			r.QuoteSender = quoteSender(ci)
			refs = append(refs, ref(&r))
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	return writeQuoteRefs(ctx, tx, refs)
}
