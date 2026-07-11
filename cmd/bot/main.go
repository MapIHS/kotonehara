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
	_ "modernc.org/sqlite"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func init() { _ = gotenv.Load() }

func main() {
	ctx := context.Background()
	cfg := config.Load()

	maxOpenConns, maxIdleConns := 20, 10
	if cfg.DBDriver == "sqlite" {
		// SQLite serializes writes; a single connection avoids "database is locked" errors.
		maxOpenConns, maxIdleConns = 1, 1
	}

	db, err := dbInfra.Connect(ctx, dbInfra.Config{
		Driver:       cfg.DBDriver,
		URL:          cfg.DBURL,
		MaxOpenConns: maxOpenConns,
		MaxIdleConns: maxIdleConns,
		ConnMaxIdle:  5 * time.Minute,
		ConnMaxLife:  30 * time.Minute,
	})

	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	dbLog := waLog.Stdout("Database", "INFO", true)
	container := sqlstore.NewWithDB(db.DB, cfg.DBDriver, dbLog)

	upCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := container.Upgrade(upCtx); err != nil {
		log.Fatal("upgrade store: ", err)
	}

	d := devices.New(container, cfg, ctx)
	dev, err := d.GetDefaultDevice(ctx)
	if err != nil {
		panic(err)
	}

	client := d.NewClient(dev)

	client.PrePairCallback = func(jid types.JID, platform, businessName string) bool {
		fmt.Printf("Pairing request from %s (platform: %s, business: %s)\n", jid, platform, businessName)
		return true
	}
	if client.Store.ID == nil {
		// No ID stored, new login
		if cfg.LoginMethod == "pairing" {
			if cfg.PairingPhoneNumber == "" {
				log.Fatal("LOGIN_METHOD=pairing butuh PAIRING_PHONE_NUMBER (nomor internasional tanpa '+', contoh 6281234567890)")
			}

			qrChan, _ := client.GetQRChannel(ctx)
			if err = client.Connect(); err != nil {
				panic(err)
			}

			// Tunggu event pertama dari channel QR untuk memastikan koneksi websocket
			// sudah siap sebelum meminta kode pairing.
			first, ok := <-qrChan
			if ok && first.Event == "code" {
				code, err := client.PairPhone(ctx, cfg.PairingPhoneNumber, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
				if err != nil {
					panic(err)
				}
				fmt.Println("Masukkan kode ini di HP: WhatsApp > Perangkat Tertaut > Tautkan dengan nomor telepon.")
				fmt.Println("Kode pairing:", code)
			}

			for evt := range qrChan {
				if evt.Event != "code" {
					fmt.Println("Login event:", evt.Event)
				}
			}
		} else {
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
