package domain

import (
	"context"
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

// TestDownloadService_QueueTracking verifies that "queued" counts only
// downloads blocked waiting for a slot. Downloads that grab a free slot
// immediately are never queued.
func TestDownloadService_QueueTracking(t *testing.T) {
	s := NewDownloadService(fakeDownloadRepository{}, t.TempDir(), 2)
	ctx := context.Background()
	changes := s.QueueChanges()

	if got := s.Queued(); got != 0 {
		t.Fatalf("initial queued = %d, want 0", got)
	}

	// Creating a download does not queue it.
	if _, err := s.CreateDownload(ctx, 1, 0, "url"); err != nil {
		t.Fatalf("CreateDownload: %v", err)
	}
	if got := s.Queued(); got != 0 {
		t.Fatalf("queued after create = %d, want 0", got)
	}

	// Acquires with free slots are not queued (fast path).
	if err := s.Acquire(ctx); err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	if err := s.Acquire(ctx); err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	if got := s.Queued(); got != 0 {
		t.Fatalf("queued after filling slots = %d, want 0", got)
	}

	// The third acquire blocks (no free slot): queued goes to 1.
	acquired := make(chan error, 1)
	go func() { acquired <- s.Acquire(ctx) }()

	select {
	case n := <-changes:
		if n != 1 {
			t.Fatalf("queued change = %d, want 1", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected queued change to 1")
	}

	// Releasing a slot lets the blocked download proceed: queued returns to 0.
	s.Release()
	if err := <-acquired; err != nil {
		t.Fatalf("third Acquire: %v", err)
	}
	select {
	case n := <-changes:
		if n != 0 {
			t.Fatalf("queued change = %d, want 0", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected queued change to 0")
	}
}

// TestDownloadService_QueueChanges verifies QueueChanges emits only on blocked
// transitions, in order, and stays silent on the fast path.
func TestDownloadService_QueueChanges(t *testing.T) {
	s := NewDownloadService(fakeDownloadRepository{}, t.TempDir(), 2)
	ctx := context.Background()
	changes := s.QueueChanges()

	// Fill both slots via the fast path: no change should be emitted.
	if err := s.Acquire(ctx); err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	if err := s.Acquire(ctx); err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	select {
	case n := <-changes:
		t.Fatalf("unexpected fast-path change = %d", n)
	default:
		// expected: silent
	}

	// Third acquire blocks: emits 1.
	acquired := make(chan error, 1)
	go func() { acquired <- s.Acquire(ctx) }()
	if got := <-changes; got != 1 {
		t.Fatalf("first change = %d, want 1", got)
	}

	// Release: emits 0.
	s.Release()
	if err := <-acquired; err != nil {
		t.Fatalf("third Acquire: %v", err)
	}
	if got := <-changes; got != 0 {
		t.Fatalf("second change = %d, want 0", got)
	}
}
