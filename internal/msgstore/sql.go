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
	} {
		if _, err = tx.ExecContext(ctx, q); err != nil {
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
	 WHERE expires_at<=? ORDER BY expires_at LIMIT ?)`
	_, err := s.db.ExecContext(ctx, s.db.Rebind(q), time.Now().UnixMilli(), s.opts.PruneBatch)
	return err
}
