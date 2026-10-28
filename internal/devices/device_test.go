package devices

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MapIHS/kotonehara/internal/infra/config"
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
	if d.tryStartCommand(func() { t.Error("over-capacity command ran") }) {
		t.Fatal("capacity exceeded")
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
