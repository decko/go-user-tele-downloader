package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/decko/go-user-tele-downloader/internal/model"
)

// DownloadRepository defines the interface for download persistence.
type DownloadRepository interface {
	Create(ctx context.Context, d *model.Download) error
	GetByID(ctx context.Context, id string) (*model.Download, error)
	ListByChat(ctx context.Context, chatID int64, limit int) ([]*model.Download, error)
	ListAll(ctx context.Context) ([]*model.Download, error)
	UpdateStatus(ctx context.Context, id string, status model.DownloadStatus, errMsg string) error
	UpdateProgress(ctx context.Context, id string, progress float64) error
	UpdateStatusMessageID(ctx context.Context, id string, messageID int) error
	ListPending(ctx context.Context, limit int) ([]*model.Download, error)
	ListByStatus(ctx context.Context, status model.DownloadStatus) ([]*model.Download, error)
}

// SQLiteDownloadRepository implements DownloadRepository using SQLite.
type SQLiteDownloadRepository struct {
	db *sql.DB
}

// NewSQLiteDownloadRepository creates a new SQLite download repository.
func NewSQLiteDownloadRepository(db *sql.DB) *SQLiteDownloadRepository {
	return &SQLiteDownloadRepository{db: db}
}

// downloadColumns is the ordered SELECT column list shared by the read queries.
const downloadColumns = `id, chat_id, user_id, url, file_path, status, progress, error, file_size, status_message_id, created_at, updated_at, completed_at`

// scanDownload scans a row into d, handling the nullable completed_at.
func scanDownload(scanner interface{ Scan(dest ...any) error }, d *model.Download) error {
	var completedAt sql.NullTime
	if err := scanner.Scan(
		&d.ID, &d.ChatID, &d.UserID, &d.URL, &d.FilePath, &d.Status, &d.Progress, &d.Error,
		&d.FileSize, &d.StatusMessageID, &d.CreatedAt, &d.UpdatedAt, &completedAt,
	); err != nil {
		return err
	}
	if completedAt.Valid {
		d.CompletedAt = &completedAt.Time
	}
	return nil
}

// Create inserts a new download record.
func (r *SQLiteDownloadRepository) Create(ctx context.Context, d *model.Download) error {
	query := `INSERT INTO downloads (id, chat_id, user_id, url, file_path, status, progress, error, file_size, status_message_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query,
		d.ID, d.ChatID, d.UserID, d.URL, d.FilePath, d.Status, d.Progress, d.Error, d.FileSize, d.StatusMessageID, d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating download: %w", err)
	}
	return nil
}

// GetByID retrieves a download by its ID.
func (r *SQLiteDownloadRepository) GetByID(ctx context.Context, id string) (*model.Download, error) {
	query := `SELECT ` + downloadColumns + ` FROM downloads WHERE id = ?`
	d := &model.Download{}
	if err := scanDownload(r.db.QueryRowContext(ctx, query, id), d); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("download not found: %s", id)
		}
		return nil, fmt.Errorf("getting download: %w", err)
	}
	return d, nil
}

// ListByChat retrieves downloads for a specific chat.
func (r *SQLiteDownloadRepository) ListByChat(ctx context.Context, chatID int64, limit int) ([]*model.Download, error) {
	query := `SELECT ` + downloadColumns + ` FROM downloads WHERE chat_id = ? ORDER BY created_at DESC LIMIT ?`
	return r.queryDownloads(ctx, query, chatID, limit)
}

// ListAll retrieves all downloads.
func (r *SQLiteDownloadRepository) ListAll(ctx context.Context) ([]*model.Download, error) {
	query := `SELECT ` + downloadColumns + ` FROM downloads ORDER BY created_at DESC`
	return r.queryDownloads(ctx, query)
}

// ListPending retrieves downloads with pending status.
func (r *SQLiteDownloadRepository) ListPending(ctx context.Context, limit int) ([]*model.Download, error) {
	query := `SELECT ` + downloadColumns + ` FROM downloads WHERE status = ? ORDER BY created_at ASC LIMIT ?`
	return r.queryDownloads(ctx, query, model.DownloadStatusPending, limit)
}

// ListByStatus retrieves downloads with the given status.
func (r *SQLiteDownloadRepository) ListByStatus(ctx context.Context, status model.DownloadStatus) ([]*model.Download, error) {
	query := `SELECT ` + downloadColumns + ` FROM downloads WHERE status = ? ORDER BY created_at ASC`
	return r.queryDownloads(ctx, query, status)
}

// queryDownloads runs a SELECT and scans all rows into downloads.
func (r *SQLiteDownloadRepository) queryDownloads(ctx context.Context, query string, args ...any) ([]*model.Download, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing downloads: %w", err)
	}
	defer rows.Close()

	var downloads []*model.Download
	for rows.Next() {
		d := &model.Download{}
		if err := scanDownload(rows, d); err != nil {
			return nil, fmt.Errorf("scanning download: %w", err)
		}
		downloads = append(downloads, d)
	}
	return downloads, rows.Err()
}

// UpdateStatus updates the status and optional error message of a download.
func (r *SQLiteDownloadRepository) UpdateStatus(ctx context.Context, id string, status model.DownloadStatus, errMsg string) error {
	query := `UPDATE downloads SET status = ?, error = ?, updated_at = ?`
	args := []any{status, errMsg, time.Now()}
	if status == model.DownloadStatusCompleted || status == model.DownloadStatusFailed {
		query += `, completed_at = ?`
		now := sql.NullTime{Time: time.Now(), Valid: true}
		args = append(args, now)
	}
	query += ` WHERE id = ?`
	args = append(args, id)
	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updating download status: %w", err)
	}
	return nil
}

// UpdateProgress updates the progress percentage of a download.
func (r *SQLiteDownloadRepository) UpdateProgress(ctx context.Context, id string, progress float64) error {
	query := `UPDATE downloads SET progress = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, progress, time.Now(), id)
	if err != nil {
		return fmt.Errorf("updating download progress: %w", err)
	}
	return nil
}

// UpdateStatusMessageID persists the Telegram message ID of a download's status
// message so it can be edited after a restart.
func (r *SQLiteDownloadRepository) UpdateStatusMessageID(ctx context.Context, id string, messageID int) error {
	query := `UPDATE downloads SET status_message_id = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, messageID, time.Now(), id)
	if err != nil {
		return fmt.Errorf("updating status message id: %w", err)
	}
	return nil
}
