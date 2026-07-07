package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MapIHS/kotonehara/internal/devices"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	dbInfra "github.com/MapIHS/kotonehara/internal/infra/db"
	"github.com/mdp/qrterminal"
	"github.com/subosito/gotenv"

	_ "github.com/MapIHS/kotonehara/pkg"

	_ "github.com/lib/pq"

	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func init() { _ = gotenv.Load() }

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	status := newBotStatus()
	cfg := config.Load()
	startWebServer(ctx, status, cfg)
	status.setStage("connecting database")

	db, err := dbInfra.Connect(ctx, dbInfra.Config{
		Driver:       "postgres",
		URL:          cfg.DBURL,
		MaxOpenConns: 20,
		MaxIdleConns: 10,
		ConnMaxIdle:  5 * time.Minute,
		ConnMaxLife:  30 * time.Minute,
	})

	if err != nil {
		status.setError(err)
		log.Fatal(err)
	}

	defer db.Close()

	dbLog := waLog.Stdout("Database", "INFO", true)
	container := sqlstore.NewWithDB(db.DB, "postgres", dbLog)

	upCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := container.Upgrade(upCtx); err != nil {
		status.setError(err)
		log.Fatal("upgrade store: ", err)
	}

	status.setStage("loading device")
	d := devices.New(container, cfg, ctx)
	dev, err := d.GetDefaultDevice(ctx)
	if err != nil {
		status.setError(err)
		panic(err)
	}

	client := d.NewClient(dev)
	status.updateClient(client)
	client.AddEventHandler(func(evt interface{}) {
		switch evt.(type) {
		case *events.Connected:
			status.setStage("running")
			status.updateClient(client)
		case *events.Disconnected:
			status.setStage("disconnected")
			status.updateClient(client)
		}
	})

	client.PrePairCallback = func(jid types.JID, platform, businessName string) bool {
		fmt.Printf("Pairing request from %s (platform: %s, business: %s)\n", jid, platform, businessName)
		return true
	}
	if client.Store.ID == nil {
		// No ID stored, new login
		status.setStage("waiting for qr login")
		qrChan, _ := client.GetQRChannel(ctx)
		err = client.Connect()
		if err != nil {
			status.setError(err)
			panic(err)
		}
		status.updateClient(client)
		// code, err := client.PairPhone(ctx, "", true, whatsmeow.PairClientChrome, "Chrome (Linux)")
		// if err != nil {
		// 	panic(err)
		// }

		// fmt.Println("ini code kamu : " + code)

		for evt := range qrChan {
			status.updateClient(client)
			if evt.Event == "code" {
				// Render the QR code here
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("Scan QR ini via WhatsApp.")
				fmt.Println("QR code:", evt.Code)
				status.setQR(evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
				if evt.Event == "success" {
					status.clearQR()
				}
			}
		}
	} else {
		// Already logged in, just connect
		status.setStage("connecting whatsapp")
		err = client.Connect()
		if err != nil {
			status.setError(err)
			panic(err)
		}
		status.updateClient(client)
	}

	status.setStage("running")
	status.updateClient(client)

	<-ctx.Done()

	status.setStage("shutting down")
	client.Disconnect()
	status.updateClient(client)
}
