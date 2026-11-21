package commands

import (
	"context"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
)

type Command struct {
	Name        string
	As          []string
	Description string
	Tags        string
	Disable     bool
	IsPrefix    bool
	IsOwner     bool
	IsMedia     bool
	IsQuery     bool
	IsGroup     bool
	IsAdmin     bool
	IsBotAdmin  bool
	ShowWait    bool
	IsPrivate   bool
	SkipQuota   bool
	// OwnHelpFlag lets the command handle "--d" itself instead of the
	// generic description reply (used by menu to show all descriptions).
	OwnHelpFlag bool
	// AllowAlbum lets an IsMedia command run when the user replies to a
	// media album parent; the handler resolves the album's children itself.
	AllowAlbum bool
	After      func(ctx context.Context, client *clients.Client, m *message.Message)
	Exec       func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config)
}
