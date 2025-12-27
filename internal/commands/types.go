package commands

import (
	"context"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/message"
)

type Command struct {
	Name        string
	As          []string
	Description string
	Tags        string
	IsPrefix    bool
	IsOwner     bool
	IsMedia     bool
	IsQuery     bool
	IsGroup     bool
	IsAdmin     bool
	IsBotAdmin  bool
	ShowWait    bool
	IsPrivate   bool
	After       func(ctx context.Context, client *clients.Client, m *message.Message)
	Exec        func(ctx context.Context, client *clients.Client, m *message.Message)
}
