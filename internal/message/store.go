package message

import (
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

var (
	MsgStore = &MessageStore{}
)

type MessageStore struct {
	db *sqlx.DB
}

// InitDBStore initializes the table and connects the DB to the store.
func InitDBStore(db *sqlx.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS bot_messages (
		id TEXT PRIMARY KEY,
		payload BYTEA NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("failed to create bot_messages table: %w", err)
	}

	// Add msg_type and sender for existing tables or new columns
	alterQuery1 := `ALTER TABLE bot_messages ADD COLUMN IF NOT EXISTS msg_type TEXT DEFAULT '';`
	db.Exec(alterQuery1) // ignore error as it might already exist

	alterQuery2 := `ALTER TABLE bot_messages ADD COLUMN IF NOT EXISTS sender TEXT DEFAULT '';`
	db.Exec(alterQuery2) // ignore error

	MsgStore.db = db
	return nil
}

// Add saves the incoming message's internal protobuf structure into Postgres.
func (s *MessageStore) Add(msg *events.Message) {
	if s.db == nil || msg.Message == nil {
		return
	}

	id := msg.Info.ID
	data, err := proto.Marshal(msg.Message)
	if err != nil {
		log.Printf("store message marshal error: %v", err)
		return
	}

	msgType := getMsgType(msg)
	sender := msg.Info.Sender.User

	query := `INSERT INTO bot_messages (id, payload, msg_type, sender) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING;`
	_, err = s.db.Exec(query, id, data, msgType, sender)
	if err != nil {
		log.Printf("store message insert error: %v", err)
	}
}

// Get fetches the protobuf message payload from DB and unmarshals it.
func (s *MessageStore) Get(id string) *waE2E.Message {
	if s.db == nil {
		return nil
	}

	var data []byte
	query := `SELECT payload FROM bot_messages WHERE id = $1 LIMIT 1;`
	err := s.db.Get(&data, query, id)
	if err != nil {
		// e.g. sql.ErrNoRows
		return nil
	}

	msgProto := &waE2E.Message{}
	if err := proto.Unmarshal(data, msgProto); err != nil {
		log.Printf("store message unmarshal error: %v", err)
		return nil
	}

	return msgProto
}

func getMsgType(msg *events.Message) string {
	if msg.Message == nil {
		return "unknown"
	}
	if msg.Message.ImageMessage != nil {
		return "image"
	} else if msg.Message.VideoMessage != nil {
		return "video"
	} else if msg.Message.AudioMessage != nil {
		return "audio"
	} else if msg.Message.DocumentMessage != nil {
		return "document"
	} else if msg.Message.StickerMessage != nil {
		return "sticker"
	} else if msg.Message.ExtendedTextMessage != nil {
		return "text"
	} else if msg.Message.Conversation != nil {
		return "text"
	}
	return "other"
}

// MediaItem represents a summary of a saved media message
type MediaItem struct {
	ID        string `db:"id"`
	MsgType   string `db:"msg_type"`
	Sender    string `db:"sender"`
	CreatedAt string `db:"created_at"`
}

// GetRecentMedia fetches recent media entries from the store.
func (s *MessageStore) GetRecentMedia(limit int) ([]MediaItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("db not initialized")
	}

	var items []MediaItem
	query := `
		SELECT id, msg_type, sender, TO_CHAR(created_at, 'YYYY-MM-DD HH24:MI:SS') AS created_at 
		FROM bot_messages 
		WHERE msg_type IN ('image', 'video', 'audio', 'sticker', 'document') 
		ORDER BY created_at DESC 
		LIMIT $1;
	`
	err := s.db.Select(&items, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}

	return items, nil
}
