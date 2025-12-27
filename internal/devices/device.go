package devices

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/message"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func New(c *sqlstore.Container) *Devices {
	return &Devices{
		container: c,
		log:       waLog.Stdout("devices", "INFO", true),
		timeout:   10 * time.Second,
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
	return func(evt interface{}) {
		c := clients.New(client)
		m := message.NewParser(c, nil)
		ctx := context.Background()
		switch v := evt.(type) {
		case *events.Message:
			parse := m.Parse(ctx, v)
			d.log.Infof("msg received: from=%v id=%v timestamp=%v",
				v.Info.Sender, v.Info.ID, v.Info.Timestamp)
			fmt.Println(v.Message)
			go commands.CommandExec(ctx, c, parse)

		case *events.Connected:
			d.log.Infof("connected")
		case *events.Disconnected:
			d.log.Warnf("disconnected")
		}
	}
}
