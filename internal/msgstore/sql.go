package msgstore

import (
	"context"
	"strings"
	"time"
)

func (s *Store) migrate(ctx context.Context) error {
	payloadType := "BYTEA"
	if s.db.DriverName() == "sqlite" {
		payloadType = "BLOB"
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var quotesExist bool
	check := `SELECT to_regclass('bot_message_quotes') IS NOT NULL`
	if s.db.DriverName() == "sqlite" {
		check = `SELECT COUNT(*)>0 FROM sqlite_master WHERE type='table' AND name='bot_message_quotes'`
	}
	if err := tx.GetContext(ctx, &quotesExist, check); err != nil {
		return err
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS bot_messages (
		 account TEXT NOT NULL, chat TEXT NOT NULL, chat_alt TEXT NOT NULL,
		 message_id TEXT NOT NULL, sender TEXT NOT NULL, sender_alt TEXT NOT NULL,
		 from_me BOOLEAN NOT NULL, is_group BOOLEAN NOT NULL,
		 message_time BIGINT NOT NULL, expires_at BIGINT NOT NULL,
		 view_once BOOLEAN NOT NULL, album_id TEXT NOT NULL, album_index INTEGER NOT NULL,
		 payload ` + payloadType + ` NOT NULL,
		 PRIMARY KEY(account,chat,message_id,sender,from_me))`,
		`CREATE INDEX IF NOT EXISTS bot_messages_chat_alt ON bot_messages(account,chat_alt,message_id)`,
		`CREATE INDEX IF NOT EXISTS bot_messages_expiry ON bot_messages(expires_at)`,
		`CREATE INDEX IF NOT EXISTS bot_messages_album ON bot_messages(account,chat,album_id,album_index)`,
		`CREATE INDEX IF NOT EXISTS bot_messages_album_alt ON bot_messages(account,chat_alt,album_id,album_index)`,
		`CREATE TABLE IF NOT EXISTS bot_message_quotes (
		 account TEXT NOT NULL, chat TEXT NOT NULL, source_id TEXT NOT NULL, source_sender TEXT NOT NULL, source_from_me BOOLEAN NOT NULL,
		 target_id TEXT NOT NULL, target_sender TEXT NOT NULL,
		 PRIMARY KEY(account,chat,source_id,source_sender,source_from_me),
		 FOREIGN KEY(account,chat,source_id,source_sender,source_from_me) REFERENCES bot_messages(account,chat,message_id,sender,from_me) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS bot_message_quotes_target ON bot_message_quotes(account,target_id,target_sender)`,
	} {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	if !quotesExist {
		if err := backfillQuotes(ctx, tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) write(ctx context.Context, batch []*row) (int64, error) {
	values := make([]string, 0, len(batch))
	args := make([]any, 0, len(batch)*14)
	for _, r := range batch {
		values = append(values, "(?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
		args = append(args, r.Account, r.Chat, r.ChatAlt, r.ID, r.Sender, r.SenderAlt, r.FromMe, r.IsGroup, r.Timestamp, r.Expires, r.ViewOnce, r.AlbumID, r.AlbumIndex, r.Payload)
	}
	q := `INSERT INTO bot_messages(account,chat,chat_alt,message_id,sender,sender_alt,from_me,is_group,message_time,expires_at,view_once,album_id,album_index,payload) VALUES ` + strings.Join(values, ",") + ` ON CONFLICT(account,chat,message_id,sender,from_me) DO NOTHING`
	var refs []quoteRef
	for _, r := range batch {
		if r.QuoteID != "" {
			refs = append(refs, ref(r))
		}
	}
	if len(refs) > 0 {
		tx, err := s.db.BeginTxx(ctx, nil)
		if err != nil {
			return 0, err
		}
		defer tx.Rollback()
		result, err := tx.ExecContext(ctx, tx.Rebind(q), args...)
		if err != nil {
			return 0, err
		}
		if err := writeQuoteRefs(ctx, tx, refs); err != nil {
			return 0, err
		}
		written, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		return written, tx.Commit()
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(q), args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) prune(ctx context.Context) error {
	// The subquery and delete use the same full primary key on both drivers.
	q := `DELETE FROM bot_messages WHERE (account,chat,message_id,sender,from_me) IN (
	 SELECT account,chat,message_id,sender,from_me FROM bot_messages
	 WHERE expires_at<=? ORDER BY expires_at,account,chat,message_id,sender,from_me LIMIT ?)`
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cutoff := time.Now().UnixMilli()
	// Explicit child cleanup also supports SQLite connections with FK checks off.
	children := `DELETE FROM bot_message_quotes WHERE (account,chat,source_id,source_sender,source_from_me) IN (
	 SELECT account,chat,message_id,sender,from_me FROM bot_messages WHERE expires_at<=? ORDER BY expires_at,account,chat,message_id,sender,from_me LIMIT ?)`
	if _, err := tx.ExecContext(ctx, tx.Rebind(children), cutoff, s.opts.PruneBatch); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(q), cutoff, s.opts.PruneBatch); err != nil {
		return err
	}
	return tx.Commit()
}
