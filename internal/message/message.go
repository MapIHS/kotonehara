package message

import (
	"context"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

type WAClient interface {
	SendText(ctx context.Context, to types.JID, txt string, opts *waE2E.ContextInfo, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
	GroupAdmins(ctx context.Context, j types.JID) ([]string, error)
	BotJID() string
}

type Message struct {
	From        types.JID
	Sender      types.JID
	PushName    string
	OwnerNumber []string

	IsOwner bool
	IsBot   bool
	IsGroup bool

	Body    string
	Command string
	Query   string

	Media    whatsmeow.DownloadableMessage
	Message  *waE2E.Message
	StanzaID string

	IsImage bool
	IsVideo bool

	IsAdmin    bool
	IsBotAdmin bool

	QuotedMsg *waE2E.ContextInfo
	ID        *waE2E.ContextInfo

	IsQuotedImage   bool
	IsQuotedVideo   bool
	IsQuotedSticker bool

	Reply func(ctx context.Context, text string, opts ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
}
