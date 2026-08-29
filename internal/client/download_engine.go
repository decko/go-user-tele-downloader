package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"

	"github.com/decko/go-user-tele-downloader/internal/model"
)

// DownloadEngine handles enhanced file downloads with progress tracking,
// resume support, parallel downloads, and verification.
type DownloadEngine struct {
	api        *tg.Client
	logger     *slog.Logger
	downloader *downloader.Downloader
}

// NewDownloadEngine creates a new download engine.
func NewDownloadEngine(api *tg.Client, logger *slog.Logger) *DownloadEngine {
	return &DownloadEngine{
		api:        api,
		logger:     logger,
		downloader: downloader.NewDownloader(),
	}
}

// ProgressCallback is called periodically with download progress.
type ProgressCallback func(downloaded, total int64, speed float64)

// DownloadOptions configures download behavior.
type DownloadOptions struct {
	// Parallel enables parallel chunk downloads for faster speeds.
	Parallel bool
	// Threads is the number of parallel download threads (only used if Parallel is true).
	Threads int
	// Resume enables resuming interrupted downloads.
	Resume bool
	// Verify enables SHA256 hash verification after download.
	Verify bool
	// ExpectedHash is the expected SHA256 hash (only used if Verify is true).
	ExpectedHash string
	// ProgressCallback is called periodically with progress updates.
	ProgressCallback ProgressCallback
	// ProgressInterval is how often to call the progress callback.
	ProgressInterval time.Duration
}

// DefaultDownloadOptions returns sensible default options.
func DefaultDownloadOptions() DownloadOptions {
	return DownloadOptions{
		Parallel:         true,
		Threads:          4,
		Resume:           true,
		Verify:           false,
		ProgressInterval: 1 * time.Second,
	}
}

// DownloadResult contains the result of a download operation.
type DownloadResult struct {
	// FilePath is the path to the downloaded file.
	FilePath string
	// FileSize is the size of the downloaded file in bytes.
	FileSize int64
	// Hash is the SHA256 hash of the file (if verification was enabled).
	Hash string
	// Duration is how long the download took.
	Duration time.Duration
	// Resumed indicates if the download was resumed from a partial file.
	Resumed bool
}

// Download downloads a file with enhanced features.
func (e *DownloadEngine) Download(ctx context.Context, download *model.Download, fileInfo *FileInfo, downloadDir string, opts DownloadOptions) (*DownloadResult, error) {
	startTime := time.Now()

	// Ensure download directory exists
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}

	// Build output file path
	outputPath := filepath.Join(downloadDir, fileInfo.Name)

	// Check for existing partial file (resume support)
	var startOffset int64
	if opts.Resume {
		if info, err := os.Stat(outputPath); err == nil {
			startOffset = info.Size()
			if startOffset > 0 && startOffset < fileInfo.Size {
				e.logger.Info("resuming download",
					"file", fileInfo.Name,
					"existing_size", startOffset,
					"total_size", fileInfo.Size,
				)
			} else if startOffset >= fileInfo.Size {
				// File already complete
				e.logger.Info("file already downloaded", "file", fileInfo.Name)
				return &DownloadResult{
					FilePath: outputPath,
					FileSize: startOffset,
					Duration: time.Since(startTime),
					Resumed:  false,
				}, nil
			} else {
				startOffset = 0
			}
		}
	}

	// Create progress tracker
	var progressTracker *progressTracker
	if opts.ProgressCallback != nil {
		progressTracker = newProgressTracker(
			fileInfo.Size,
			startOffset,
			opts.ProgressInterval,
			opts.ProgressCallback,
		)
		progressTracker.start(ctx)
	}

	// Build download
	builder := e.downloader.Download(e.api, fileInfo.Ref)

	// Configure parallel downloads
	if opts.Parallel && opts.Threads > 1 {
		builder = builder.WithThreads(opts.Threads)
	}

	// Perform download
	var err error
	if opts.Parallel && opts.Threads > 1 {
		// Use parallel download with io.WriterAt
		var writerAt io.WriterAt
		if startOffset > 0 {
			// Resume: open existing file
			file, err := os.OpenFile(outputPath, os.O_WRONLY, 0644)
			if err != nil {
				if progressTracker != nil {
					progressTracker.stop()
				}
				return nil, fmt.Errorf("failed to open file for resume: %w", err)
			}
			defer file.Close()
			writerAt = file
		} else {
			// New download: create file
			file, err := os.Create(outputPath)
			if err != nil {
				if progressTracker != nil {
					progressTracker.stop()
				}
				return nil, fmt.Errorf("failed to create file: %w", err)
			}
			defer file.Close()
			writerAt = file
		}

		// Wrap writer with progress tracking if needed
		if progressTracker != nil {
			writerAt = &progressWriterAt{
				writer:  writerAt,
				tracker: progressTracker,
			}
		}

		_, err = builder.Parallel(ctx, writerAt)
	} else {
		// Use sequential download with io.Writer
		var writer io.Writer
		if startOffset > 0 {
			// Resume: open existing file and seek to end
			file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				if progressTracker != nil {
					progressTracker.stop()
				}
				return nil, fmt.Errorf("failed to open file for resume: %w", err)
			}
			defer file.Close()
			writer = file
		} else {
			// New download: create file
			file, err := os.Create(outputPath)
			if err != nil {
				if progressTracker != nil {
					progressTracker.stop()
				}
				return nil, fmt.Errorf("failed to create file: %w", err)
			}
			defer file.Close()
			writer = file
		}

		// Wrap writer with progress tracking
		if progressTracker != nil {
			writer = &progressWriter{
				writer:  writer,
				tracker: progressTracker,
			}
		}

		_, err = builder.Stream(ctx, writer)
	}

	if progressTracker != nil {
		progressTracker.stop()
	}

	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}

	// Get final file size
	info, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat downloaded file: %w", err)
	}

	result := &DownloadResult{
		FilePath: outputPath,
		FileSize: info.Size(),
		Duration: time.Since(startTime),
		Resumed:  startOffset > 0,
	}

	// Verify file hash if requested
	if opts.Verify {
		hash, err := calculateFileHash(outputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate file hash: %w", err)
		}
		result.Hash = hash

		if opts.ExpectedHash != "" && hash != opts.ExpectedHash {
			return nil, fmt.Errorf("hash mismatch: expected %s, got %s", opts.ExpectedHash, hash)
		}
	}

	e.logger.Info("download completed",
		"file", fileInfo.Name,
		"path", outputPath,
		"size", result.FileSize,
		"duration", result.Duration,
		"resumed", result.Resumed,
		"hash", result.Hash,
	)

	return result, nil
}

// progressTracker tracks download progress and calls callbacks periodically.
type progressTracker struct {
	total      int64
	downloaded int64
	startTime  time.Time
	interval   time.Duration
	callback   ProgressCallback
	stopCh     chan struct{}
	mu         sync.Mutex
}

func newProgressTracker(total, startOffset int64, interval time.Duration, callback ProgressCallback) *progressTracker {
	return &progressTracker{
		total:      total,
		downloaded: startOffset,
		startTime:  time.Now(),
		interval:   interval,
		callback:   callback,
		stopCh:     make(chan struct{}),
	}
}

func (p *progressTracker) start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		lastDownloaded := p.downloaded
		lastTime := time.Now()

		for {
			select {
			case <-ticker.C:
				p.mu.Lock()
				downloaded := p.downloaded
				p.mu.Unlock()

				// Calculate speed
				now := time.Now()
				elapsed := now.Sub(lastTime).Seconds()
				speed := float64(downloaded-lastDownloaded) / elapsed

				p.callback(downloaded, p.total, speed)

				lastDownloaded = downloaded
				lastTime = now
			case <-p.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (p *progressTracker) stop() {
	close(p.stopCh)
}

func (p *progressTracker) addBytes(n int) {
	p.mu.Lock()
	p.downloaded += int64(n)
	p.mu.Unlock()
}

// progressWriter wraps an io.Writer and tracks progress.
type progressWriter struct {
	writer  io.Writer
	tracker *progressTracker
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if err != nil {
		return n, err
	}
	pw.tracker.addBytes(n)
	return n, nil
}

// progressWriterAt wraps an io.WriterAt and tracks progress.
type progressWriterAt struct {
	writer  io.WriterAt
	tracker *progressTracker
}

func (pwa *progressWriterAt) WriteAt(p []byte, off int64) (int, error) {
	n, err := pwa.writer.WriteAt(p, off)
	if err != nil {
		return n, err
	}
	pwa.tracker.addBytes(n)
	return n, nil
}

// calculateFileHash calculates the SHA256 hash of a file.
func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
