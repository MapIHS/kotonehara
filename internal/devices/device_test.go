package devices

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

func TestConcurrencyLimitAndShutdownDrain(t *testing.T) {
	d := New(nil, config.Config{}, context.Background())
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{}, cap(d.eventSem))
	for i := 0; i < cap(d.eventSem); i++ {
		if !d.tryStartCommand(func() { started <- struct{}{}; <-release }) {
			t.Fatal("command rejected before capacity")
		}
	}
	for i := 0; i < cap(d.eventSem); i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("command did not start")
		}
	}
	// Over the concurrency limit the event is queued, not dropped, and it
	// must not run while every slot is busy.
	queuedRan := make(chan struct{}, 1)
	if !d.tryStartCommand(func() { queuedRan <- struct{}{} }) {
		t.Fatal("event over concurrency limit was dropped instead of queued")
	}
	select {
	case <-queuedRan:
		t.Fatal("queued command ran while all slots were busy")
	case <-time.After(50 * time.Millisecond):
	}
	timeout, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.Shutdown(timeout); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown should wait for running commands: %v", err)
	}
	if d.tryStartCommand(func() { t.Error("post-shutdown command ran") }) {
		t.Fatal("accepted command after shutdown")
	}
	// Let all accepted commands finish without relying on a timing delay.
	for i := 0; i < cap(d.eventSem); i++ {
		release <- struct{}{}
	}
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := d.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := d.Shutdown(ctx); err != nil {
		t.Fatalf("repeated shutdown: %v", err)
	}
	select {
	case <-queuedRan:
		t.Fatal("command still waiting at shutdown was run instead of dropped")
	default:
	}
}

func TestQueuedCommandRunsWhenSlotFrees(t *testing.T) {
	d := New(nil, config.Config{}, context.Background())
	release := make(chan struct{})
	started := make(chan struct{}, cap(d.eventSem))
	for i := 0; i < cap(d.eventSem); i++ {
		d.tryStartCommand(func() { started <- struct{}{}; <-release })
	}
	for i := 0; i < cap(d.eventSem); i++ {
		<-started
	}
	queuedRan := make(chan struct{})
	if !d.tryStartCommand(func() { close(queuedRan) }) {
		t.Fatal("queued command rejected")
	}
	release <- struct{}{}
	select {
	case <-queuedRan:
	case <-time.After(time.Second):
		t.Fatal("queued command did not run after a slot freed")
	}
	close(release)
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := d.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestQueueLimitRejects(t *testing.T) {
	d := New(nil, config.Config{}, context.Background())
	d.maxQueued = 2
	release := make(chan struct{})
	for i := 0; i < d.maxQueued; i++ {
		if !d.tryStartCommand(func() { <-release }) {
			t.Fatal("rejected below queue limit")
		}
	}
	if d.tryStartCommand(func() { t.Error("over-limit command ran") }) {
		t.Fatal("queue limit not enforced")
	}
	close(release)
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := d.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCanceledRootRejectsCommandsAndDrains(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	d := New(nil, config.Config{}, ctx)
	cancel()
	if d.tryStartCommand(func() { t.Error("command ran after cancellation") }) {
		t.Fatal("accepted command on canceled root")
	}
	deadline, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := d.Shutdown(deadline); err != nil {
		t.Fatal(err)
	}
	if d.tryStartCommand(nil) {
		t.Fatal("accepted nil command")
	}
}

func TestBusinessProfileRegistrationLoginAndRePair(t *testing.T) {
	origProps := proto.Clone(store.DeviceProps).(*waCompanionReg.DeviceProps)
	origPayload := proto.Clone(store.BaseClientPayload).(*waWa6.ClientPayload)
	t.Cleanup(func() {
		clientPayloadMu.Lock()
		defer clientPayloadMu.Unlock()
		store.DeviceProps = origProps
		store.BaseClientPayload = origPayload
	})
	db, err := sqlx.Connect("sqlite", filepath.Join(t.TempDir(), "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	container := sqlstore.NewWithDB(db.DB, "sqlite", nil)
	if err = container.Upgrade(context.Background()); err != nil {
		t.Fatal(err)
	}
	dev := container.NewDevice()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := New(container, config.Config{}, ctx)
	client := d.NewClient(dev)
	if client.QRClientType != whatsmeow.PairClientChrome {
		t.Fatalf("QR type = %v", client.QRClientType)
	}
	check := func(payload *waWa6.ClientPayload) {
		t.Helper()
		ua := payload.GetUserAgent()
		if ua.GetPlatform() != waWa6.ClientPayload_UserAgent_SMB_ANDROID || ua.GetDevice() != "Android 13" || ua.GetOsVersion() != "13.0.0" {
			t.Fatalf("wrong Business profile: %v", ua)
		}
		if payload.GetWebInfo() == nil || payload.GetWebInfo().GetWebSubPlatform() != waWa6.ClientPayload_WebInfo_WEB_BROWSER {
			t.Fatalf("Business profile lost webInfo: %v", payload.GetWebInfo())
		}
	}
	registration := client.GetClientPayload()
	check(registration)
	var props waCompanionReg.DeviceProps
	if err := proto.Unmarshal(registration.GetDevicePairingData().GetDeviceProps(), &props); err != nil {
		t.Fatal(err)
	}
	if props.GetPlatformType() != waCompanionReg.DeviceProps_ANDROID_PHONE || props.GetOs() != "Android 13" {
		t.Fatalf("registration props = %v", &props)
	}
	jid := types.NewJID("6281234567890", types.DefaultUserServer)
	jid.Device = 1
	dev.ID = &jid
	// A saved session does not need to wait for a LID or reset its connection.
	login := d.NewClient(dev).GetClientPayload()
	check(login)
	if login.GetDevicePairingData() != nil || login.GetUsername() != 6281234567890 || login.GetDevice() != 1 {
		t.Fatalf("login payload identity changed")
	}
	handler := d.registerEventHandler(client, nil)
	handler(&events.LoggedOut{})
	dev.ID = nil
	check(client.GetClientPayload())
	if client.GetClientPayload().GetDevicePairingData() == nil || client.QRClientType != whatsmeow.PairClientChrome {
		t.Fatal("re-pair lost registration data or Chrome QR type")
	}
}
