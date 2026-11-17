package devices

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/handlers"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	meowcaller "github.com/purpshell/meowcaller"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

const (
	// maxConcurrentEvents bounds handlers running at once (downloads, ffmpeg, ...).
	maxConcurrentEvents = 20
	// maxQueuedEvents bounds handlers accepted but not finished, so a burst
	// waits for a free slot instead of being silently dropped, while a flood
	// still cannot grow memory without limit.
	maxQueuedEvents = 500
)

func New(c *sqlstore.Container, cfg config.Config, ctx context.Context) *Devices {
	if ctx == nil {
		ctx = context.Background()
	}
	commands.SetCooldown(cfg.Cooldown)

	d := &Devices{
		container: c,
		log:       waLog.Stdout("devices", "INFO", true),
		timeout:   10 * time.Second,
		ctx:       ctx,
		cfg:       cfg,
		eventSem:  make(chan struct{}, maxConcurrentEvents),
		maxQueued: maxQueuedEvents,
		stopCh:    make(chan struct{}),
		drained:   make(chan struct{}),
	}
	go func() {
		select {
		case <-ctx.Done():
			d.beginShutdown()
		case <-d.drained:
		}
	}()
	return d
}

// Shutdown stops accepting command events and waits for active commands to finish.
// The wait is bounded by ctx, while active commands continue using the root context.
func (d *Devices) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	d.beginShutdown()
	select {
	case <-d.drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Devices) beginShutdown() {
	d.stopOnce.Do(func() {
		d.lifecycleMu.Lock()
		d.stopping = true
		close(d.stopCh)
		d.lifecycleMu.Unlock()
		go func() {
			d.active.Wait()
			close(d.drained)
		}()
	})
}

// tryStartCommand accepts run for asynchronous execution. It never blocks the
// whatsmeow event loop: accepted work waits in its own goroutine for one of the
// maxConcurrentEvents slots. It returns false (and logs) only when shutting
// down or when maxQueued handlers are already pending.
func (d *Devices) tryStartCommand(run func()) bool {
	if run == nil {
		return false
	}

	d.lifecycleMu.Lock()
	if d.stopping || d.ctx.Err() != nil {
		d.lifecycleMu.Unlock()
		return false
	}
	if d.queued >= d.maxQueued {
		d.lifecycleMu.Unlock()
		d.log.Warnf("event queue full (%d pending), dropping message event", d.maxQueued)
		return false
	}
	d.queued++
	d.active.Add(1)
	d.lifecycleMu.Unlock()

	go func() {
		defer func() {
			d.lifecycleMu.Lock()
			d.queued--
			d.lifecycleMu.Unlock()
			d.active.Done()
		}()

		select {
		case d.eventSem <- struct{}{}:
		case <-d.stopCh:
			return
		case <-d.ctx.Done():
			return
		}
		defer func() { <-d.eventSem }()

		// A slot may free up at the same moment shutdown begins; queued work
		// that has not started yet is dropped rather than run during drain.
		select {
		case <-d.stopCh:
			return
		default:
		}
		run()
	}()
	return true
}

func (d *Devices) GetDefaultDevice(ctx context.Context) (*store.Device, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	dev, err := d.container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("get first device: %w", err)
	}
	if dev == nil {
		return nil, errors.New("no device found (not paired yet)")
	}
	return dev, nil
}

func (d *Devices) NewClient(dev *store.Device) *whatsmeow.Client {
	clientLog := waLog.Stdout("whatsmeow", "ERROR", true)

	if d.cfg.DisableContactImport {
		d.log.Infof("contact import disabled")
		disableContactImportForDevice(dev)
	}

	client := whatsmeow.NewClient(dev, clientLog)
	callClient := meowcaller.NewClient(client)
	callClient.OnIncomingCall(func(call *meowcaller.Call) {
		d.log.Infof("incoming call from %s (%s)", call.Peer().String(), call.ID())
	})
	if d.cfg.DisableContactImport {
		disableContactImportForDevice(client.Store)
	}
	client.AddEventHandler(d.registerEventHandler(client, callClient))
	return client
}

func (d *Devices) registerEventHandler(client *whatsmeow.Client, callClient *meowcaller.Client) func(evt interface{}) {
	c := clients.New(client, d.cfg, callClient)
	c.Messages = d.Messages
	m := message.NewParser(c, d.cfg)
	m.ResolveIdentity = c.ResolveIdentity
	return func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			if d.ctx.Err() != nil {
				return
			}
			if d.Messages != nil {
				d.Messages.Capture(c.MessageStoreAccount(), v)
			}
			parse := m.Parse(d.ctx, v)

			d.tryStartCommand(func() {
				defer func() {
					if r := recover(); r != nil {
						d.log.Errorf("command panic: %v", r)
					}
				}()

				handlers.CheckAFK(d.ctx, c, parse, d.cfg)

				if !commands.CanHandle(parse.Body, d.cfg) {
					return
				}

				commands.CommandExec(d.ctx, c, parse, d.cfg)
			})

		case *events.UndecryptableMessage:
			if d.Messages != nil && v.IsUnavailable && v.UnavailableType == events.UnavailableTypeViewOnce {
				d.Messages.NoteUnavailableViewOnce()
			}
		case *events.Connected:
			if d.cfg.DisableContactImport {
				disableContactImportForDevice(client.Store)
			}
			d.log.Infof("connected")
		case *events.Disconnected:
			d.log.Warnf("disconnected")
		}
	}
}
