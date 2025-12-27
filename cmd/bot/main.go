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
	dbInfra "github.com/MapIHS/kotonehara/internal/infra/db"
	"github.com/subosito/gotenv"

	_ "github.com/MapIHS/kotonehara/pkg"

	_ "github.com/lib/pq"

	"github.com/mdp/qrterminal"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func init() { _ = gotenv.Load() }

func main() {
	ctx := context.Background()

	db, err := dbInfra.Connect(ctx, dbInfra.Config{
		Driver:       "postgres",
		URL:          os.Getenv("DATABASE_URL"),
		MaxOpenConns: 20,
		MaxIdleConns: 10,
		ConnMaxIdle:  5 * time.Minute,
		ConnMaxLife:  30 * time.Minute,
	})

	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	dbLog := waLog.Stdout("Database", "INFO", true)
	container := sqlstore.NewWithDB(db.DB, "postgres", dbLog)

	upCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := container.Upgrade(upCtx); err != nil {
		log.Fatal("upgrade store: ", err)
	}

	d := devices.New(container)
	dev, err := d.GetDefaultDevice(ctx)
	if err != nil {
		panic(err)
	}

	client := d.NewClient(dev)

	if client.Store.ID == nil {
		// No ID stored, new login
		qrChan, _ := client.GetQRChannel(ctx)
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Render the QR code here
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("Scan QR ini via WhatsApp.")
				fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
}
