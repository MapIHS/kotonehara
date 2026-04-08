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

	query := `INSERT INTO bot_messages (id, payload) VALUES ($1, $2) ON CONFLICT DO NOTHING;`
	_, err = s.db.Exec(query, id, data)
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
