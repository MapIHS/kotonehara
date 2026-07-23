package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
		loginCtx, cancelLogin := context.WithTimeout(ctx, 5*time.Minute)
		defer cancelLogin()

		qrChan, qrErr := client.GetQRChannel(loginCtx)
		if qrErr != nil {
			log.Fatal("buat channel login: ", qrErr)
		}
		if err = client.Connect(); err != nil {
			log.Fatal("connect WhatsApp: ", err)
		}

		if cfg.LoginMethod == "pairing" {
			if cfg.PairingPhoneNumber == "" {
				log.Fatal("LOGIN_METHOD=pairing butuh PAIRING_PHONE_NUMBER (nomor internasional tanpa '+', contoh 6281234567890)")
			}
			err = runPairingLogin(loginCtx, client, qrChan, cfg.PairingPhoneNumber)
		} else {
			err = runQRLogin(loginCtx, qrChan)
		}
		if err != nil && ctx.Err() == nil {
			log.Fatal("login WhatsApp: ", err)
		}
	} else if err = client.Connect(); err != nil {
		log.Fatal("connect WhatsApp: ", err)
	}

	<-ctx.Done()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()
	if err := d.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown command: %v", err)
	}
	client.Disconnect()
}

func runPairingLogin(ctx context.Context, client *whatsmeow.Client, qrChan <-chan whatsmeow.QRChannelItem, phoneNumber string) error {
	pairRequested := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt, ok := <-qrChan:
			if !ok {
				return fmt.Errorf("channel login tertutup sebelum pairing berhasil")
			}
			switch evt.Event {
			case whatsmeow.QRChannelSuccess.Event:
				return nil
			case whatsmeow.QRChannelTimeout.Event:
				return fmt.Errorf("kode pairing kedaluwarsa")
			case whatsmeow.QRChannelEventError:
				return fmt.Errorf("pairing ditolak: %w", evt.Error)
			case whatsmeow.QRChannelEventCode:
				if pairRequested {
					continue
				}
			default:
				if strings.HasPrefix(evt.Event, "err-") {
					return fmt.Errorf("pairing gagal: %s", evt.Event)
				}
				fmt.Println("Login event:", evt.Event)
				continue
			}

			code, err := client.PairPhone(ctx, phoneNumber, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
			if err != nil {
				return fmt.Errorf("minta kode pairing: %w", err)
			}
			pairRequested = true
			fmt.Println("Masukkan kode ini di HP: WhatsApp > Perangkat Tertaut > Tautkan dengan nomor telepon.")
			fmt.Println("Kode pairing:", code)
		}
	}
}

func runQRLogin(ctx context.Context, qrChan <-chan whatsmeow.QRChannelItem) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt, ok := <-qrChan:
			if !ok {
				return fmt.Errorf("channel login tertutup sebelum QR berhasil dipindai")
			}
			switch evt.Event {
			case whatsmeow.QRChannelSuccess.Event:
				return nil
			case whatsmeow.QRChannelTimeout.Event:
				return fmt.Errorf("QR login kedaluwarsa")
			case whatsmeow.QRChannelEventError:
				return fmt.Errorf("QR login ditolak: %w", evt.Error)
			case whatsmeow.QRChannelEventCode:
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("Scan QR ini via WhatsApp.")
				fmt.Println(evt.Code)
			default:
				if strings.HasPrefix(evt.Event, "err-") {
					return fmt.Errorf("QR login gagal: %s", evt.Event)
				}
				fmt.Println("Login event:", evt.Event)
			}
		}
	}
}
