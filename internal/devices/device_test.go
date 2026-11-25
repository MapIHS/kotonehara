package devices

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
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

func TestClientBrowserProfileBeforeAndAfterLogin(t *testing.T) {
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
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	container := sqlstore.NewWithDB(db.DB, "sqlite", nil)
	if err := container.Upgrade(context.Background()); err != nil {
		t.Fatal(err)
	}

	dev := container.NewDevice()
	d := New(container, config.Config{}, context.Background())
	client := d.NewClient(dev)

	// Before login (QR pairing): Android 13 with DESKTOP platform type so Linked Devices shows "Android 13".
	regPayload := client.GetClientPayload()
	if regPayload.GetUserAgent().GetPlatform() != waWa6.ClientPayload_UserAgent_WEB {
		t.Fatalf("unpaired platform = %v, want WEB", regPayload.GetUserAgent().GetPlatform())
	}
	if regPayload.GetUserAgent().GetDevice() != "Android 13" {
		t.Fatalf("unpaired device = %q, want Android 13", regPayload.GetUserAgent().GetDevice())
	}
	if regPayload.GetUserAgent().GetOsVersion() != "13.0.0" {
		t.Fatalf("unpaired osVersion = %q, want 13.0.0", regPayload.GetUserAgent().GetOsVersion())
	}
	var regProps waCompanionReg.DeviceProps
	if err := proto.Unmarshal(regPayload.GetDevicePairingData().GetDeviceProps(), &regProps); err != nil {
		t.Fatalf("unmarshal registration DeviceProps: %v", err)
	}
	if regProps.GetPlatformType() != waCompanionReg.DeviceProps_DESKTOP {
		t.Fatalf("unpaired PlatformType = %v, want DESKTOP", regProps.GetPlatformType())
	}
	if regProps.GetOs() != "Android 13" {
		t.Fatalf("unpaired Os = %q, want Android 13", regProps.GetOs())
	}

	// Simulate completed pairing / existing login session.
	jid := types.NewJID("6281234567890", types.DefaultUserServer)
	jid.Device = 1
	dev.ID = &jid

	loginPayload := client.GetClientPayload()
	if loginPayload.GetDevicePairingData() != nil {
		t.Fatal("logged-in payload should not include DevicePairingData")
	}
	if loginPayload.GetUserAgent().GetPlatform() != waWa6.ClientPayload_UserAgent_WEB {
		t.Fatalf("logged-in platform = %v, want WEB", loginPayload.GetUserAgent().GetPlatform())
	}
	if loginPayload.GetUserAgent().GetDevice() != "Android 13" {
		t.Fatalf("logged-in device = %q, want Android 13", loginPayload.GetUserAgent().GetDevice())
	}
	if loginPayload.GetUserAgent().GetOsVersion() != "13.0.0" {
		t.Fatalf("logged-in osVersion = %q, want 13.0.0", loginPayload.GetUserAgent().GetOsVersion())
	}
	if loginPayload.GetUserAgent().GetOsBuildNumber() != "13.0.0" {
		t.Fatalf("logged-in osBuildNumber = %q, want 13.0.0", loginPayload.GetUserAgent().GetOsBuildNumber())
	}
	if store.DeviceProps.GetPlatformType() != waCompanionReg.DeviceProps_DESKTOP {
		t.Fatalf("logged-in PlatformType = %v, want DESKTOP", store.DeviceProps.GetPlatformType())
	}
	if store.DeviceProps.GetOs() != "Android 13" {
		t.Fatalf("logged-in DeviceProps.Os = %q, want Android 13", store.DeviceProps.GetOs())
	}

	// New client created for an already-logged-in device starts in Android 13 profile immediately.
	client2 := d.NewClient(dev)
	loginPayload2 := client2.GetClientPayload()
	if loginPayload2.GetUserAgent().GetDevice() != "Android 13" || loginPayload2.GetUserAgent().GetOsVersion() != "13.0.0" {
		t.Fatalf("restarted logged-in client got device=%q os=%q", loginPayload2.GetUserAgent().GetDevice(), loginPayload2.GetUserAgent().GetOsVersion())
	}
}
