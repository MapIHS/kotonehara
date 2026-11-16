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
