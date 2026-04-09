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

	msgType := getMsgType(msg.Message)
	sender := msg.Info.PushName
	if sender == "" {
		sender = msg.Info.Sender.User
	}

	query := `INSERT INTO bot_messages (id, payload, msg_type, sender) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING;`
	_, err = s.db.Exec(query, id, data, msgType, sender)
	if err != nil {
		log.Printf("store message insert error: %v", err)
	}

	// Extract and save the quoted message if any (solves ViewOnce issue that are only visible when replied)
	ctxInfo := extractContextInfo(msg.Message)
	if ctxInfo != nil && ctxInfo.QuotedMessage != nil && ctxInfo.StanzaID != nil && *ctxInfo.StanzaID != "" {
		quotedID := *ctxInfo.StanzaID
		quotedData, err := proto.Marshal(ctxInfo.QuotedMessage)
		if err == nil {
			quotedMsgType := getMsgType(ctxInfo.QuotedMessage)
			quotedSender := "unknown_quoted"
			if ctxInfo.Participant != nil {
				quotedSender = *ctxInfo.Participant
			}
			qQuery := `INSERT INTO bot_messages (id, payload, msg_type, sender) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING;`
			s.db.Exec(qQuery, quotedID, quotedData, quotedMsgType, quotedSender)
		}
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

func getMsgType(realMsg *waE2E.Message) string {
	if realMsg == nil {
		return "unknown"
	}

	// Unwrap EphemeralMessage if sent in a disappearing chat
	if realMsg.EphemeralMessage != nil && realMsg.EphemeralMessage.Message != nil {
		realMsg = realMsg.EphemeralMessage.Message
	}
	// Unwrap DocumentWithCaptionMessage
	if realMsg.DocumentWithCaptionMessage != nil && realMsg.DocumentWithCaptionMessage.Message != nil {
		realMsg = realMsg.DocumentWithCaptionMessage.Message
	}

	if realMsg.ViewOnceMessage != nil || realMsg.ViewOnceMessageV2 != nil || realMsg.ViewOnceMessageV2Extension != nil {
		return "viewonce"
	}
	if realMsg.ImageMessage != nil {
		return "image"
	} else if realMsg.VideoMessage != nil {
		return "video"
	} else if realMsg.AudioMessage != nil {
		return "audio"
	} else if realMsg.DocumentMessage != nil {
		return "document"
	} else if realMsg.StickerMessage != nil {
		return "sticker"
	} else if realMsg.ExtendedTextMessage != nil {
		return "text"
	} else if realMsg.Conversation != nil {
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
		WHERE msg_type IN ('image', 'video', 'audio', 'sticker', 'document', 'viewonce') 
		ORDER BY created_at DESC 
		LIMIT $1;
	`
	err := s.db.Select(&items, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}

	return items, nil
}

func extractContextInfo(msg *waE2E.Message) *waE2E.ContextInfo {
	if msg == nil {
		return nil
	}
	if msg.ExtendedTextMessage != nil {
		return msg.ExtendedTextMessage.ContextInfo
	} else if msg.ImageMessage != nil {
		return msg.ImageMessage.ContextInfo
	} else if msg.VideoMessage != nil {
		return msg.VideoMessage.ContextInfo
	} else if msg.DocumentMessage != nil {
		return msg.DocumentMessage.ContextInfo
	} else if msg.AudioMessage != nil {
		return msg.AudioMessage.ContextInfo
	} else if msg.StickerMessage != nil {
		return msg.StickerMessage.ContextInfo
	} else if msg.LocationMessage != nil {
		return msg.LocationMessage.ContextInfo
	} else if msg.ContactMessage != nil {
		return msg.ContactMessage.ContextInfo
	} else if msg.ContactsArrayMessage != nil {
		return msg.ContactsArrayMessage.ContextInfo
	} else if msg.TemplateMessage != nil {
		return msg.TemplateMessage.ContextInfo
	} else if msg.ButtonsMessage != nil {
		return msg.ButtonsMessage.ContextInfo
	} else if msg.ListMessage != nil {
		return msg.ListMessage.ContextInfo
	} else if msg.PtvMessage != nil {
		return msg.PtvMessage.ContextInfo
	} else if msg.ViewOnceMessage != nil {
		return extractContextInfo(msg.ViewOnceMessage.Message)
	} else if msg.ViewOnceMessageV2 != nil {
		return extractContextInfo(msg.ViewOnceMessageV2.Message)
	} else if msg.ViewOnceMessageV2Extension != nil {
		return extractContextInfo(msg.ViewOnceMessageV2Extension.Message)
	} else if msg.DocumentWithCaptionMessage != nil {
		return extractContextInfo(msg.DocumentWithCaptionMessage.Message)
	} else if msg.EphemeralMessage != nil {
		return extractContextInfo(msg.EphemeralMessage.Message)
	}
	return nil
}
