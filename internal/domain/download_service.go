package domain

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/decko/go-user-tele-downloader/internal/model"
	"github.com/decko/go-user-tele-downloader/internal/repository"
)

// DownloadService manages download operations.
type DownloadService struct {
	repo          repository.DownloadRepository
	downloadDir   string
	maxConcurrent int
	sem           chan struct{}
}

// NewDownloadService creates a new download service.
func NewDownloadService(repo repository.DownloadRepository, downloadDir string, maxConcurrent int) *DownloadService {
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	return &DownloadService{
		repo:          repo,
		downloadDir:   downloadDir,
		maxConcurrent: maxConcurrent,
		sem:           make(chan struct{}, maxConcurrent),
	}
}

// Acquire blocks until a download slot is available, enforcing the configured
// maxConcurrent limit. It returns ctx.Err() if ctx is cancelled while waiting.
// Every successful Acquire must be paired with exactly one Release.
func (s *DownloadService) Acquire(ctx context.Context) error {
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release frees a download slot previously acquired with Acquire.
func (s *DownloadService) Release() {
	<-s.sem
}

// CreateDownload creates a new download request.
func (s *DownloadService) CreateDownload(ctx context.Context, chatID, userID int64, url string) (*model.Download, error) {
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}

	if err := os.MkdirAll(s.downloadDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating download directory: %w", err)
	}

	d := &model.Download{
		ID:        uuid.New().String(),
		ChatID:    chatID,
		UserID:    userID,
		URL:       url,
		Status:    model.DownloadStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.repo.Create(ctx, d); err != nil {
		return nil, fmt.Errorf("creating download: %w", err)
	}

	slog.Info("download created", "id", d.ID, "url", d.URL, "chat_id", chatID)
	return d, nil
}

// GetDownload retrieves a download by ID.
func (s *DownloadService) GetDownload(ctx context.Context, id string) (*model.Download, error) {
	return s.repo.GetByID(ctx, id)
}

// ListDownloads retrieves recent downloads for a chat.
func (s *DownloadService) ListDownloads(ctx context.Context, chatID int64, limit int) ([]*model.Download, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.repo.ListByChat(ctx, chatID, limit)
}

// CancelDownload cancels a pending download.
func (s *DownloadService) CancelDownload(ctx context.Context, id string) error {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting download: %w", err)
	}

	if d.Status != model.DownloadStatusPending {
		return fmt.Errorf("can only cancel pending downloads, current status: %s", d.Status)
	}

	if err := s.repo.UpdateStatus(ctx, id, model.DownloadStatusCancelled, ""); err != nil {
		return fmt.Errorf("cancelling download: %w", err)
	}

	slog.Info("download cancelled", "id", id)
	return nil
}

// DownloadFilePath returns the full file path for a download.
func (s *DownloadService) DownloadFilePath(filename string) string {
	return filepath.Join(s.downloadDir, filename)
}

// UpdateDownloadStatus updates the status of a download.
func (s *DownloadService) UpdateDownloadStatus(ctx context.Context, id string, status model.DownloadStatus, errMsg string) error {
	return s.repo.UpdateStatus(ctx, id, status, errMsg)
}

// UpdateDownloadProgress updates the progress of a download.
func (s *DownloadService) UpdateDownloadProgress(ctx context.Context, id string, progress float64) error {
	return s.repo.UpdateProgress(ctx, id, progress)
}
