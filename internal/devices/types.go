package devices

import (
	"context"
	"time"

	"github.com/MapIHS/kotonehara/internal/infra/config"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type Devices struct {
	container *sqlstore.Container
	log       waLog.Logger
	timeout   time.Duration
	ctx       context.Context
	cfg       config.Config
}

type Device struct {
	Store  *store.Device
	Client *whatsmeow.Client
}
