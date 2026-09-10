package client

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/gotd/td/tg"
)

// StatusMessage manages a status message in a Telegram channel
// that gets edited periodically to show download progress.
type StatusMessage struct {
	api       *tg.Client
	channelID int64
	peer      tg.InputPeerClass
	fileInfo  *FileInfo
	messageID int
	startTime time.Time
	lastEdit  time.Time
	lastText  string // last message text sent/edited, guarded by mu
	mu        sync.Mutex
	logger    *slog.Logger
}

// NewStatusMessage creates a new status message manager.
// peer is obtained from the original channel message.
func NewStatusMessage(api *tg.Client, channelID int64, peer tg.InputPeerClass, fileInfo *FileInfo, logger *slog.Logger) *StatusMessage {
	return &StatusMessage{
		api:       api,
		channelID: channelID,
		peer:      peer,
		fileInfo:  fileInfo,
		startTime: time.Now(),
		logger:    logger,
	}
}

// Start sends the initial status message to the channel.
func (s *StatusMessage) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := s.buildMessage(0, 0, 0, "Starting...")
	s.lastText = msg

	// Send message to channel using the resolved peer
	req := &tg.MessagesSendMessageRequest{
		Peer:      s.peer,
		Message:   msg,
		NoWebpage: true,
		RandomID:  rand.Int63(),
	}

	resp, err := s.api.MessagesSendMessage(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send status message: %w", err)
	}

	// Get the message ID
	updates, ok := resp.(*tg.Updates)
	if ok && len(updates.Updates) > 0 {
		for _, update := range updates.Updates {
			if msgUpdate, ok := update.(*tg.UpdateMessageID); ok {
				s.messageID = msgUpdate.ID
				break
			}
		}
	}

	if s.messageID == 0 {
		s.logger.Warn("could not get message ID for status updates")
	}

	s.lastEdit = time.Now()
	s.logger.Info("status message sent", "file", s.fileInfo.Name, "message_id", s.messageID)
	return nil
}

// MessageID returns the ID of the status message, or 0 if it has not been sent.
func (s *StatusMessage) MessageID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messageID
}

// Update edits the status message with current progress.
// Rate-limited to at most one edit per 5 seconds.
func (s *StatusMessage) Update(ctx context.Context, downloaded, total int64, speed float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.messageID == 0 {
		return nil
	}

	// Rate limit: don't edit more than once every 5 seconds
	if time.Since(s.lastEdit) < 5*time.Second {
		return nil
	}

	eta := int64(0)
	if speed > 0 {
		eta = int64(float64(total-downloaded) / speed)
	}

	msg := s.buildMessage(downloaded, total, eta, "")
	s.lastText = msg

	// Edit the message
	_, err := s.api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:      s.peer,
		ID:        s.messageID,
		Message:   msg,
		NoWebpage: true,
	})
	if err != nil {
		return fmt.Errorf("failed to edit status message: %w", err)
	}

	s.lastEdit = time.Now()
	return nil
}

// Complete edits the status message with final completion status.
func (s *StatusMessage) Complete(ctx context.Context, downloaded, total int64, duration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.messageID == 0 {
		return nil
	}

	speed := float64(total) / duration.Seconds()
	msg := fmt.Sprintf(
		"✅ Download Complete\n"+
			"File: `%s`\n"+
			"Size: `%s`\n"+
			"Time: `%s`\n"+
			"Average Speed: `%.2f MB/s`",
		s.fileInfo.Name,
		formatSize(total),
		formatDuration(duration),
		speed/1024/1024,
	)
	s.lastText = msg

	_, err := s.api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:      s.peer,
		ID:        s.messageID,
		Message:   msg,
		NoWebpage: true,
	})
	if err != nil {
		return fmt.Errorf("failed to edit status message: %w", err)
	}

	return nil
}

// Fail edits the status message with failure information.
func (s *StatusMessage) Fail(ctx context.Context, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.messageID == 0 {
		return nil
	}

	msg := fmt.Sprintf(
		"❌ Download Failed\n"+
			"File: `%s`\n"+
			"Error: `%s`",
		s.fileInfo.Name,
		errMsg,
	)
	s.lastText = msg

	_, editErr := s.api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:      s.peer,
		ID:        s.messageID,
		Message:   msg,
		NoWebpage: true,
	})
	if editErr != nil {
		return fmt.Errorf("failed to edit status message: %w", editErr)
	}

	return nil
}

// ImportResult appends the import outcome line to the completed status message
// and edits it in place. It is a no-op if the message was never sent or line is
// empty.
func (s *StatusMessage) ImportResult(ctx context.Context, line string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.messageID == 0 || line == "" {
		return nil
	}

	msg := s.lastText + "\n\n" + line
	_, err := s.api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:      s.peer,
		ID:        s.messageID,
		Message:   msg,
		NoWebpage: true,
	})
	if err != nil {
		return fmt.Errorf("failed to edit status message with import result: %w", err)
	}
	s.lastText = msg
	return nil
}

// buildMessage builds the status message text.
func (s *StatusMessage) buildMessage(downloaded, total, eta int64, customStatus string) string {
	if customStatus != "" {
		return fmt.Sprintf(
			"📥 Downloading...\n"+
				"File: `%s`\n"+
				"Size: `%s`\n"+
				"Status: `%s`",
			s.fileInfo.Name,
			formatSize(total),
			customStatus,
		)
	}

	progress := float64(downloaded) / float64(total) * 100
	bar := buildProgressBar(progress, 20)
	speed := float64(downloaded) / time.Since(s.startTime).Seconds()

	return fmt.Sprintf(
		"📥 Downloading...\n"+
			"File: `%s`\n"+
			"Size: `%s / %s`\n"+
			"Progress: %s `%.1f%%`\n"+
			"Speed: `%.2f MB/s`\n"+
			"ETA: `%s`",
		s.fileInfo.Name,
		formatSize(downloaded),
		formatSize(total),
		bar,
		progress,
		speed/1024/1024,
		formatDuration(time.Duration(eta)*time.Second),
	)
}

// buildProgressBar creates a visual progress bar.
func buildProgressBar(progress float64, width int) string {
	filled := int(math.Round(progress / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	empty := width - filled

	bar := ""
	for i := 0; i < filled; i++ {
		bar += "█"
	}
	for i := 0; i < empty; i++ {
		bar += "░"
	}
	return bar
}

// formatDuration formats a duration in a human-readable format.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// formatSize formats bytes in a human-readable format.
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
