package devices

import (
	"context"
	"sync"
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

	lifecycleMu sync.Mutex
	stopping    bool
	active      sync.WaitGroup
	eventSem    chan struct{} // limits concurrently running handlers
	queued      int           // accepted handlers (running + waiting), guarded by lifecycleMu
	maxQueued   int           // cap on queued; beyond it events are rejected and logged
	stopCh      chan struct{} // closed when shutdown begins; waiting handlers are dropped
	stopOnce    sync.Once
	drained     chan struct{}
}

type Device struct {
	Store  *store.Device
	Client *whatsmeow.Client
}
