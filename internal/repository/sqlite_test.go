package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/decko/go-user-tele-downloader/internal/migration"
	"github.com/decko/go-user-tele-downloader/internal/model"
)

// TestSQLiteDownloadRepository_ConcurrentWrites ensures that many goroutines
// writing to the database concurrently do not collide with SQLITE_BUSY
// ("database is locked"). SQLite permits a single writer at a time; without
// serializing access, the download engine's concurrent status/progress writes
// fail and abort downloads.
func TestSQLiteDownloadRepository_ConcurrentWrites(t *testing.T) {
	ctx := context.Background()

	db, err := NewSQLiteDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := migration.Run(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	repo := NewSQLiteDownloadRepository(db)

	// 20 goroutines × 20 writes = 400 overlapping writes, which reliably
	// reproduces the SQLITE_BUSY collisions seen when an album is forwarded and
	// each download writes status/progress concurrently.
	const (
		goroutines    = 20
		writesPerGoro = 20
	)

	// start gates all goroutines so their writes overlap as much as possible.
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*writesPerGoro)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			<-start
			for i := 0; i < writesPerGoro; i++ {
				d := &model.Download{
					ID:        fmt.Sprintf("download-%d-%d", g, i),
					ChatID:    1,
					UserID:    1,
					URL:       "https://example.com/file.mkv",
					Status:    model.DownloadStatusPending,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				if err := repo.Create(ctx, d); err != nil {
					errs <- fmt.Errorf("create %s: %w", d.ID, err)
					return
				}
				if err := repo.UpdateStatus(ctx, d.ID, model.DownloadStatusDownloading, ""); err != nil {
					errs <- fmt.Errorf("update %s: %w", d.ID, err)
					return
				}
			}
		}(g)
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent write failed: %v", err)
	}
}
