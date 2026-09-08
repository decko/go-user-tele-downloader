package domain

import (
	"context"
	"testing"
	"time"
)

// TestDownloadService_AcquireRelease_LimitsConcurrency verifies that the
// download service enforces maxConcurrent: Acquire blocks once every slot is
// held, and a blocked Acquire proceeds as soon as a slot is released.
func TestDownloadService_AcquireRelease_LimitsConcurrency(t *testing.T) {
	s := NewDownloadService(nil, "", 2) // maxConcurrent = 2
	ctx := context.Background()

	// Fill both slots.
	if err := s.Acquire(ctx); err != nil {
		t.Fatalf("acquire slot 1: %v", err)
	}
	if err := s.Acquire(ctx); err != nil {
		t.Fatalf("acquire slot 2: %v", err)
	}

	// A third acquire must block until a slot is released.
	third := make(chan error, 1)
	go func() {
		if err := s.Acquire(ctx); err != nil {
			third <- err
			return
		}
		s.Release()
		third <- nil
	}()

	select {
	case <-third:
		t.Fatal("third acquire returned while all slots were held")
	case <-time.After(100 * time.Millisecond):
		// expected: still blocked
	}

	s.Release()

	select {
	case err := <-third:
		if err != nil {
			t.Fatalf("third acquire after release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("third acquire did not unblock after a slot was released")
	}
}

// TestDownloadService_Acquire_ContextCancelled verifies that Acquire returns
// ctx.Err() instead of blocking forever when the context is cancelled.
func TestDownloadService_Acquire_ContextCancelled(t *testing.T) {
	s := NewDownloadService(nil, "", 1)
	if err := s.Acquire(context.Background()); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.Acquire(ctx); err == nil {
		t.Fatal("expected error when acquiring with a cancelled context")
	}
}
