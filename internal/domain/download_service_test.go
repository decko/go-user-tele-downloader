package domain

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/decko/go-user-tele-downloader/internal/model"
	"github.com/decko/go-user-tele-downloader/internal/repository"
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

// fakeDownloadRepository is a minimal in-memory DownloadRepository for tests.
type fakeDownloadRepository struct{}

var _ repository.DownloadRepository = fakeDownloadRepository{}

func (fakeDownloadRepository) Create(_ context.Context, _ *model.Download) error { return nil }
func (fakeDownloadRepository) GetByID(_ context.Context, _ string) (*model.Download, error) {
	return nil, nil
}
func (fakeDownloadRepository) ListByChat(_ context.Context, _ int64, _ int) ([]*model.Download, error) {
	return nil, nil
}
func (fakeDownloadRepository) ListAll(_ context.Context) ([]*model.Download, error) { return nil, nil }
func (fakeDownloadRepository) UpdateStatus(_ context.Context, _ string, _ model.DownloadStatus, _ string) error {
	return nil
}
func (fakeDownloadRepository) UpdateProgress(_ context.Context, _ string, _ float64) error {
	return nil
}
func (fakeDownloadRepository) ListPending(_ context.Context, _ int) ([]*model.Download, error) {
	return nil, nil
}

// TestDownloadService_QueueTracking verifies the queued counter: each created
// download is queued until a slot is acquired.
func TestDownloadService_QueueTracking(t *testing.T) {
	s := NewDownloadService(fakeDownloadRepository{}, t.TempDir(), 2)

	if got := s.Queued(); got != 0 {
		t.Fatalf("initial queued = %d, want 0", got)
	}

	for i := 0; i < 2; i++ {
		if _, err := s.CreateDownload(context.Background(), 1, 0, fmt.Sprintf("url-%d", i)); err != nil {
			t.Fatalf("CreateDownload %d: %v", i, err)
		}
	}
	if got := s.Queued(); got != 2 {
		t.Fatalf("queued after 2 creates = %d, want 2", got)
	}

	if err := s.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if got := s.Queued(); got != 1 {
		t.Fatalf("queued after 1 acquire = %d, want 1", got)
	}

	if err := s.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if got := s.Queued(); got != 0 {
		t.Fatalf("queued after 2 acquires = %d, want 0", got)
	}
}

// TestDownloadService_QueueChanges verifies that QueueChanges emits the queued
// count on every change, in order.
func TestDownloadService_QueueChanges(t *testing.T) {
	s := NewDownloadService(fakeDownloadRepository{}, t.TempDir(), 2)
	changes := s.QueueChanges()

	if _, err := s.CreateDownload(context.Background(), 1, 0, "url"); err != nil {
		t.Fatalf("CreateDownload: %v", err)
	}
	if got := <-changes; got != 1 {
		t.Fatalf("first change = %d, want 1", got)
	}

	if err := s.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if got := <-changes; got != 0 {
		t.Fatalf("second change = %d, want 0", got)
	}
}
