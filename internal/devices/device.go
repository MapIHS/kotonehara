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

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func New(c *sqlstore.Container, cfg config.Config, ctx context.Context) *Devices {
	return &Devices{
		container: c,
		log:       waLog.Stdout("devices", "INFO", true),
		timeout:   10 * time.Second,
		ctx:       ctx,
		cfg:       cfg,
	}
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

	client := whatsmeow.NewClient(dev, clientLog)
	client.AddEventHandler(d.registerEventHandler(client))
	return client
}

func (d *Devices) registerEventHandler(client *whatsmeow.Client) func(evt interface{}) {
	c := clients.New(client, d.cfg)
	m := message.NewParser(c, d.cfg)
	sem := make(chan struct{}, 20)
	return func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			parse := m.Parse(d.ctx, v)

			sem <- struct{}{}
			go func() {
				defer func() {
					if r := recover(); r != nil {
						d.log.Errorf("command panic: %v", r)
					}
					<-sem
				}()
				commands.CommandExec(d.ctx, c, parse, d.cfg)
			}()

		case *events.Connected:
			d.log.Infof("connected")
		case *events.Disconnected:
			d.log.Warnf("disconnected")
		}
	}
}
