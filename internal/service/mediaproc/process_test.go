package mediaproc

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCanceledContextDoesNotRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CommandContext(ctx, "true").Run(); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestWaitsForFreeSlot(t *testing.T) {
	if _, err := LookPath("true"); err != nil {
		t.Skip("true not available")
	}
	// Occupy every slot so the next command must wait.
	for i := 0; i < cap(slots); i++ {
		slots <- struct{}{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := CommandContext(ctx, "true").Run()
	for i := 0; i < cap(slots); i++ {
		<-slots
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("command ran without a free slot: %v", err)
	}
	if err := Command("true").Run(); err != nil {
		t.Fatalf("command failed once slots freed: %v", err)
	}
	if len(slots) != 0 {
		t.Fatal("slot leaked after run")
	}
}
