package model

import "time"

// DownloadStatus represents the state of a download.
type DownloadStatus string

const (
	// DownloadStatusPending means the download is queued.
	DownloadStatusPending DownloadStatus = "pending"
	// DownloadStatusDownloading means the download is in progress.
	DownloadStatusDownloading DownloadStatus = "downloading"
	// DownloadStatusCompleted means the download finished successfully.
	DownloadStatusCompleted DownloadStatus = "completed"
	// DownloadStatusFailed means the download encountered an error.
	DownloadStatusFailed DownloadStatus = "failed"
	// DownloadStatusCancelled means the download was cancelled by the user.
	DownloadStatusCancelled DownloadStatus = "cancelled"
)

// Download represents a file download tracked by the system.
type Download struct {
	ID          string
	ChatID      int64
	UserID      int64
	URL         string
	FilePath    string
	Status      DownloadStatus
	Progress    float64
	Error       string
	FileSize    int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
}

// User represents a Telegram user who interacted with the bot.
type User struct {
	ID        int64
	Username  string
	FirstName string
	LastName  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Chat represents a Telegram chat where the bot is active.
type Chat struct {
	ID        int64
	Title     string
	Type      string
	CreatedAt time.Time
	UpdatedAt time.Time
}
