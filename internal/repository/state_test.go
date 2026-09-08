package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/decko/go-user-tele-downloader/internal/migration"
	"github.com/decko/go-user-tele-downloader/internal/model"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := NewSQLiteDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteDB: %v", err)
	}
	if err := migration.Run(db); err != nil {
		t.Fatalf("migration.Run: %v", err)
	}
	return db
}

func TestStateRepository_SetGetDelete(t *testing.T) {
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewSQLiteStateRepository(db)
	ctx := context.Background()

	// Missing key returns "".
	if got, err := repo.Get(ctx, "missing"); err != nil || got != "" {
		t.Fatalf("Get(missing) = %q, %v; want \"\", nil", got, err)
	}

	if err := repo.Set(ctx, "k", "v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, err := repo.Get(ctx, "k"); err != nil || got != "v1" {
		t.Fatalf("Get(k) = %q, %v; want v1, nil", got, err)
	}

	// Upsert overwrites.
	if err := repo.Set(ctx, "k", "v2"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, _ := repo.Get(ctx, "k"); got != "v2" {
		t.Fatalf("Get(k) = %q, want v2", got)
	}

	if err := repo.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, "k"); got != "" {
		t.Fatalf("Get(k) after delete = %q, want \"\"", got)
	}
}

func TestSQLiteDownloadRepository_StatusMessageIDAndListByStatus(t *testing.T) {
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewSQLiteDownloadRepository(db)
	ctx := context.Background()

	d := &model.Download{
		ID:     "dl-1",
		ChatID: 1,
		URL:    "file.mkv",
		Status: model.DownloadStatusDownloading,
	}
	if err := repo.Create(ctx, d); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Persist a status message ID.
	if err := repo.UpdateStatusMessageID(ctx, "dl-1", 12345); err != nil {
		t.Fatalf("UpdateStatusMessageID: %v", err)
	}

	got, err := repo.GetByID(ctx, "dl-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.StatusMessageID != 12345 {
		t.Fatalf("StatusMessageID = %d, want 12345", got.StatusMessageID)
	}

	// ListByStatus finds the downloading download.
	list, err := repo.ListByStatus(ctx, model.DownloadStatusDownloading)
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(list) != 1 || list[0].ID != "dl-1" {
		t.Fatalf("ListByStatus(downloading) = %+v, want [dl-1]", list)
	}

	// ListByStatus for a different status finds nothing.
	list, err = repo.ListByStatus(ctx, model.DownloadStatusCompleted)
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("ListByStatus(completed) = %d, want 0", len(list))
	}
}
