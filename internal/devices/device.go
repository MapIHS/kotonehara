package devices

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	meowcaller "github.com/purpshell/meowcaller"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
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
		eventSem:  make(chan struct{}, 20),
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
		d.lifecycleMu.Unlock()
		go func() {
			d.active.Wait()
			close(d.drained)
		}()
	})
}

func (d *Devices) tryStartCommand(run func()) bool {
	if run == nil {
		return false
	}

	d.lifecycleMu.Lock()
	if d.stopping || d.ctx.Err() != nil {
		d.lifecycleMu.Unlock()
		return false
	}
	select {
	case d.eventSem <- struct{}{}:
		d.active.Add(1)
	default:
		d.lifecycleMu.Unlock()
		return false
	}
	d.lifecycleMu.Unlock()

	go func() {
		defer func() {
			<-d.eventSem
			d.active.Done()
		}()
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
	m := message.NewParser(c, d.cfg)
	return func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			if !commands.CanHandle(message.ExtractBody(v), d.cfg) {
				return
			}

			d.tryStartCommand(func() {
				defer func() {
					if r := recover(); r != nil {
						d.log.Errorf("command panic: %v", r)
					}
				}()
				parse := m.Parse(d.ctx, v)
				commands.CommandExec(d.ctx, c, parse, d.cfg)
			})

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
