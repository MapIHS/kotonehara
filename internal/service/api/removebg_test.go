package api

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRemoveBGRequestsSerializeAndRespectDelayAndCancellation(t *testing.T) {
	delay := 30 * time.Millisecond
	l := &removeBGLimiter{slot: make(chan struct{}, 1), delay: delay}
	release, err := l.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	_, err = l.acquire(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		release()
		t.Fatalf("allowed concurrent request: %v", err)
	}
	finished := time.Now()
	release()
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Millisecond)
	_, err = l.acquire(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cooldown did not honor cancellation: %v", err)
	}
	nextRelease, err := l.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer nextRelease()
	if time.Since(finished) < delay {
		t.Fatal("next request started before the pause ended")
	}
}
