package quote

// Payload is the request body sent to the Quote API.
type Payload struct {
	Type            string    `json:"type"`
	Format          string    `json:"format"`
	BackgroundColor string    `json:"backgroundColor"`
	Width           int       `json:"width"`
	Height          int       `json:"height"`
	Scale           int       `json:"scale"`
	Messages        []Message `json:"messages"`
}

// Message represents a single message bubble in the quote image.
type Message struct {
	Entities     []Entity      `json:"entities"`
	Media        *Media        `json:"media,omitempty"`
	Avatar       bool          `json:"avatar"`
	From         From          `json:"from"`
	Text         string        `json:"text"`
	ReplyMessage *ReplyMessage `json:"replyMessage,omitempty"`
}

// Media is an optional image attachment inside the quote.
type Media struct {
	URL string `json:"url"`
}

// From identifies the sender shown in the quote bubble.
type From struct {
	ID    int   `json:"id"`
	Name  string `json:"name"`
	Photo Photo  `json:"photo"`
}

// Photo is the avatar shown next to the quote bubble.
type Photo struct {
	URL string `json:"url"`
}

// ReplyMessage is a nested reply shown inside the quote bubble.
type ReplyMessage struct {
	Name   string   `json:"name"`
	Text   string   `json:"text"`
	ChatID int      `json:"chatId"`
	Entities []Entity `json:"entities,omitempty"`
}

// Entity represents a Telegram text formatting entity.
type Entity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

// Response is the API response from the Quote API.
type Response struct {
	Ok     bool `json:"ok"`
	Result struct {
		Image string `json:"image"`
	} `json:"result"`
}
