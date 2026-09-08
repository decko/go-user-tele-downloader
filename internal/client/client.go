package client

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"github.com/decko/go-user-tele-downloader/internal/config"
	"github.com/decko/go-user-tele-downloader/internal/domain"
	"github.com/decko/go-user-tele-downloader/internal/model"
	"github.com/decko/go-user-tele-downloader/internal/radarr"
)

// TelegramClient wraps the MTProto client with download functionality.
type TelegramClient struct {
	client            *telegram.Client
	api               *tg.Client
	service           *domain.DownloadService
	config            *config.Config
	logger            *slog.Logger
	downloadEngine    *DownloadEngine
	channelAccessHash map[int64]int64 // cached channel access hashes
	radarrImporter    *radarr.Importer
}

// New creates a new TelegramClient instance.
func New(cfg *config.Config, service *domain.DownloadService, logger *slog.Logger) *TelegramClient {
	return &TelegramClient{
		service:           service,
		config:            cfg,
		logger:            logger,
		channelAccessHash: make(map[int64]int64),
	}
}

// updateHandler implements telegram.UpdateHandler
type updateHandler struct {
	client *TelegramClient
}

func (h *updateHandler) Handle(ctx context.Context, updates tg.UpdatesClass) error {
	return h.client.handleUpdates(ctx, updates)
}

// Start initializes the MTProto client and begins monitoring channels.
func (c *TelegramClient) Start(ctx context.Context) error {
	// Create session manager for encrypted session storage
	sessionMgr := NewSessionManager(c.config.SessionPath, c.config.SessionEncryptionPassword)

	// Create update handler
	handler := &updateHandler{client: c}

	// Create MTProto client with encrypted session storage
	c.client = telegram.NewClient(c.config.APIID, c.config.APIHash, telegram.Options{
		SessionStorage: sessionMgr,
		UpdateHandler:  handler,
	})

	// Run client
	return c.client.Run(ctx, func(ctx context.Context) error {
		// Authenticate if needed
		if err := c.authenticate(ctx); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		// Get client API
		c.api = c.client.API()

		// Initialize download engine
		c.downloadEngine = NewDownloadEngine(c.api, c.logger)

		// Initialize Radarr importer if configured
		if c.config.RadarrURL != "" && c.config.RadarrAPIKey != "" {
			rc := radarr.NewClient(c.config.RadarrURL, c.config.RadarrAPIKey)
			c.radarrImporter = radarr.NewImporter(rc, c.config.RadarrRootFolder, c.config.RadarrQualityProfile, c.config.RadarrImportStrict, c.logger)
			c.logger.Info("radarr integration enabled", "url", c.config.RadarrURL, "strict", c.config.RadarrImportStrict)
		}

		c.logger.Info("client started", "phone", c.config.Phone)

		// Start monitoring channels
		return c.monitorChannels(ctx)
	})
}

// authenticate handles the MTProto authentication flow.
func (c *TelegramClient) authenticate(ctx context.Context) error {
	// Check if already authenticated
	status, err := c.client.Auth().Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get auth status: %w", err)
	}

	if status.Authorized {
		c.logger.Info("already authenticated", "user", status.User.FirstName)
		return nil
	}

	c.logger.Info("authentication required", "phone", c.config.Phone)

	// Create authenticator
	authenticator := NewAuthenticator(c.config, c.logger)

	// Run authentication
	if err := authenticator.Authenticate(ctx, c.client); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save session after successful authentication
	newStatus, err := c.client.Auth().Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get auth status after authentication: %w", err)
	}

	if newStatus.Authorized {
		// Session is automatically saved by the session manager
		c.logger.Info("session saved successfully")
	}

	return nil
}

// handleUpdates processes incoming updates from Telegram.
func (c *TelegramClient) handleUpdates(ctx context.Context, updates tg.UpdatesClass) error {
	c.logger.Debug("received updates", "type", fmt.Sprintf("%T", updates))

	switch u := updates.(type) {
	case *tg.Updates:
		c.logger.Debug("processing Updates", "count", len(u.Updates))
		for _, update := range u.Updates {
			c.logger.Debug("processing update", "type", fmt.Sprintf("%T", update))
			if err := c.handleUpdate(ctx, update); err != nil {
				c.logger.Error("failed to handle update", "error", err)
			}
		}
	case *tg.UpdateShort:
		c.logger.Debug("processing UpdateShort", "type", fmt.Sprintf("%T", u.Update))
		if err := c.handleUpdate(ctx, u.Update); err != nil {
			c.logger.Error("failed to handle update", "error", err)
		}
	}
	return nil
}

// handleUpdate processes a single update.
func (c *TelegramClient) handleUpdate(ctx context.Context, update tg.UpdateClass) error {
	c.logger.Debug("handleUpdate called", "type", fmt.Sprintf("%T", update))

	switch u := update.(type) {
	case *tg.UpdateNewChannelMessage:
		c.logger.Debug("received UpdateNewChannelMessage")
		return c.handleChannelMessage(ctx, u)
	case *tg.UpdateNewMessage:
		// Handle private messages if needed
		c.logger.Debug("received new message", "message_id", u.Message.GetID())
	}
	return nil
}

// monitorChannels logs that channel monitoring is active.
func (c *TelegramClient) monitorChannels(ctx context.Context) error {
	c.logger.Info("channel monitoring active", "channels", c.config.MonitorChannels)

	// Keep the client running
	<-ctx.Done()
	return ctx.Err()
}

// handleChannelMessage processes new messages from monitored channels.
func (c *TelegramClient) handleChannelMessage(ctx context.Context, update *tg.UpdateNewChannelMessage) error {
	c.logger.Debug("handleChannelMessage called")

	// Extract message
	msg, ok := update.Message.(*tg.Message)
	if !ok {
		c.logger.Debug("unsupported message type", "type", fmt.Sprintf("%T", update.Message))
		return nil
	}

	// Extract channel ID from message peer
	channelID := c.extractChannelID(msg)
	if channelID == 0 {
		c.logger.Debug("could not extract channel ID")
		return nil
	}

	c.logger.Debug("extracted channel ID", "channel_id", channelID, "monitored_channels", c.config.MonitorChannels)

	// Check if this channel is being monitored
	if !c.isMonitoredChannel(channelID) {
		c.logger.Debug("channel not monitored", "channel_id", channelID)
		return nil
	}

	c.logger.Debug("channel is monitored, checking for media")

	// Check if message contains a file
	if msg.Media == nil {
		c.logger.Debug("message has no media")
		return nil
	}

	c.logger.Debug("message has media", "media_type", fmt.Sprintf("%T", msg.Media))

	// Extract file information
	fileInfo := c.extractFileInfo(msg.Media)
	if fileInfo == nil {
		c.logger.Debug("could not extract file info")
		return nil
	}

	// Check if the user opted out of Radarr import for this file.
	skipImport := strings.Contains(strings.ToLower(msg.Message), "#noimport")

	c.logger.Info("file detected in channel",
		"channel_id", channelID,
		"message_id", msg.ID,
		"file_name", fileInfo.Name,
		"file_size", fileInfo.Size,
		"mime_type", fileInfo.MimeType,
		"skip_radarr_import", skipImport,
	)

	// Create download task
	return c.createDownloadTask(ctx, channelID, int64(msg.ID), fileInfo, msg.PeerID, skipImport)
}

// extractChannelID extracts the channel ID from a message.
// Telegram MTProto returns channel IDs without the -100 prefix,
// so we add it to match the configured format.
func (c *TelegramClient) extractChannelID(msg *tg.Message) int64 {
	if peer, ok := msg.PeerID.(*tg.PeerChannel); ok {
		// Add -100 prefix to match the configured channel ID format
		return -1000000000000 - peer.ChannelID
	}
	return 0
}

// isMonitoredChannel checks if a channel ID is in the monitored list.
func (c *TelegramClient) isMonitoredChannel(channelID int64) bool {
	for _, id := range c.config.MonitorChannels {
		if id == channelID {
			return true
		}
	}
	return false
}

// FileInfo represents extracted file information from a message.
type FileInfo struct {
	FileID   int64
	Name     string
	Size     int64
	MimeType string
	Ref      tg.InputFileLocationClass
}

// extractFileInfo extracts file information from message media.
func (c *TelegramClient) extractFileInfo(media tg.MessageMediaClass) *FileInfo {
	switch m := media.(type) {
	case *tg.MessageMediaDocument:
		return c.extractDocumentInfo(m)
	case *tg.MessageMediaPhoto:
		return c.extractPhotoInfo(m)
	default:
		return nil
	}
}

// extractDocumentInfo extracts information from a document media.
func (c *TelegramClient) extractDocumentInfo(media *tg.MessageMediaDocument) *FileInfo {
	doc, ok := media.Document.(*tg.Document)
	if !ok {
		return nil
	}

	// Extract filename from attributes
	var fileName string
	var mimeType string
	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeFilename:
			fileName = a.FileName
		case *tg.DocumentAttributeAudio:
			if a.Title != "" {
				fileName = a.Title
			}
		}
	}

	if fileName == "" {
		fileName = fmt.Sprintf("document_%d", doc.ID)
	}

	mimeType = doc.MimeType

	return &FileInfo{
		FileID:   doc.ID,
		Name:     fileName,
		Size:     int64(doc.Size),
		MimeType: mimeType,
		Ref: &tg.InputDocumentFileLocation{
			ID:            doc.ID,
			AccessHash:    doc.AccessHash,
			FileReference: doc.FileReference,
		},
	}
}

// extractPhotoInfo extracts information from a photo media.
func (c *TelegramClient) extractPhotoInfo(media *tg.MessageMediaPhoto) *FileInfo {
	photo, ok := media.Photo.(*tg.Photo)
	if !ok {
		return nil
	}

	// Find the largest photo size
	var largestSize *tg.PhotoSize
	for _, size := range photo.Sizes {
		if s, ok := size.(*tg.PhotoSize); ok {
			if largestSize == nil || s.Size > largestSize.Size {
				largestSize = s
			}
		}
	}

	if largestSize == nil {
		return nil
	}

	fileName := fmt.Sprintf("photo_%d.jpg", photo.ID)

	return &FileInfo{
		FileID:   photo.ID,
		Name:     fileName,
		Size:     int64(largestSize.Size),
		MimeType: "image/jpeg",
		Ref: &tg.InputPhotoFileLocation{
			ID:            photo.ID,
			AccessHash:    photo.AccessHash,
			FileReference: photo.FileReference,
			ThumbSize:     largestSize.Type,
		},
	}
}

// createDownloadTask creates a download task for a detected file.
func (c *TelegramClient) createDownloadTask(ctx context.Context, channelID, messageID int64, fileInfo *FileInfo, peer tg.PeerClass, skipImport bool) error {
	// Generate unique download ID
	downloadID := fmt.Sprintf("channel_%d_msg_%d_file_%d", channelID, messageID, fileInfo.FileID)

	// Create download record
	download, err := c.service.CreateDownload(ctx, channelID, 0, fileInfo.Name)
	if err != nil {
		return fmt.Errorf("failed to create download record: %w", err)
	}

	c.logger.Info("download task created",
		"download_id", downloadID,
		"file_name", fileInfo.Name,
		"file_size", fileInfo.Size,
	)

	// Create input peer from the message's peer
	var inputPeer tg.InputPeerClass
	if p, ok := peer.(*tg.PeerChannel); ok {
		hash := c.resolveChannelAccessHash(ctx, p.ChannelID)
		inputPeer = &tg.InputPeerChannel{
			ChannelID:  p.ChannelID,
			AccessHash: hash,
		}
	}

	// Start download in background, bounded by the configured concurrency limit.
	go func() {
		if err := c.service.Acquire(ctx); err != nil {
			c.logger.Debug("download skipped while waiting for slot", "download_id", downloadID, "error", err)
			return
		}
		defer c.service.Release()

		if err := c.downloadFile(ctx, download, fileInfo, channelID, inputPeer, skipImport); err != nil {
			c.logger.Error("download failed",
				"download_id", downloadID,
				"error", err,
			)
			// Update download status to failed
			if updateErr := c.service.UpdateDownloadStatus(ctx, download.ID, model.DownloadStatusFailed, err.Error()); updateErr != nil {
				c.logger.Error("failed to update download status", "error", updateErr)
			}
		}
	}()

	return nil
}

// downloadFile downloads a file using the enhanced download engine.
func (c *TelegramClient) downloadFile(ctx context.Context, download *model.Download, fileInfo *FileInfo, channelID int64, peer tg.InputPeerClass, skipImport bool) error {
	// Update status to downloading
	if err := c.service.UpdateDownloadStatus(ctx, download.ID, model.DownloadStatusDownloading, ""); err != nil {
		return fmt.Errorf("failed to update download status: %w", err)
	}

	// Create status message for the channel
	statusMsg := NewStatusMessage(c.api, channelID, peer, fileInfo, c.logger)
	if err := statusMsg.Start(ctx); err != nil {
		c.logger.Warn("failed to send status message", "error", err)
		// Don't fail the download if status message fails
	}

	// Configure download options
	opts := DefaultDownloadOptions()
	opts.Parallel = true
	opts.Threads = 4
	opts.Resume = true
	opts.Verify = false

	// Set up progress callback that updates the status message
	opts.ProgressCallback = func(downloaded, total int64, speed float64) {
		progress := float64(downloaded) / float64(total) * 100
		c.logger.Debug("download progress",
			"file", fileInfo.Name,
			"progress", fmt.Sprintf("%.1f%%", progress),
			"downloaded", downloaded,
			"total", total,
			"speed", fmt.Sprintf("%.2f MB/s", speed/1024/1024),
		)

		// Update status message in channel
		if err := statusMsg.Update(ctx, downloaded, total, speed); err != nil {
			c.logger.Warn("failed to update status message", "error", err)
		}

		// Update progress in database
		if err := c.service.UpdateDownloadProgress(ctx, download.ID, progress); err != nil {
			c.logger.Error("failed to update download progress", "error", err)
		}
	}

	// Perform download
	result, err := c.downloadEngine.Download(ctx, download, fileInfo, c.config.DownloadDir, opts)
	if err != nil {
		// Mark status message as failed
		statusMsg.Fail(ctx, err.Error())
		return err
	}

	// Mark status message as complete
	statusMsg.Complete(ctx, result.FileSize, result.FileSize, result.Duration)

	// Update status to completed
	if err := c.service.UpdateDownloadStatus(ctx, download.ID, model.DownloadStatusCompleted, ""); err != nil {
		return fmt.Errorf("failed to update download status: %w", err)
	}

	c.logger.Info("download completed successfully",
		"download_id", download.ID,
		"file_name", fileInfo.Name,
		"file_path", result.FilePath,
		"file_size", result.FileSize,
		"duration", result.Duration,
		"resumed", result.Resumed,
	)

	// Optionally import the file into Radarr (opt-in via config, per-file opt-out via #noimport).
	if !skipImport && c.radarrImporter != nil {
		c.logger.Info("importing download into radarr", "file", result.FilePath)
		title, err := c.radarrImporter.ImportFile(ctx, result.FilePath)
		if err != nil {
			c.logger.Error("radarr import failed", "file", result.FilePath, "error", err)
		} else {
			c.logger.Info("radarr import triggered", "title", title, "file", result.FilePath)
		}
	}

	return nil
}

// resolveChannelAccessHash resolves and caches the access hash for a channel.
// This is needed to send/edit messages in the channel.
func (c *TelegramClient) resolveChannelAccessHash(ctx context.Context, channelID int64) int64 {
	// Check cache first
	if hash, ok := c.channelAccessHash[channelID]; ok {
		return hash
	}

	// Resolve channel info
	resp, err := c.api.ChannelsGetChannels(ctx, []tg.InputChannelClass{
		&tg.InputChannel{
			ChannelID:  channelID,
			AccessHash: 0,
		},
	})
	if err != nil {
		c.logger.Warn("failed to resolve channel access hash", "channel_id", channelID, "error", err)
		return 0
	}

	chats, ok := resp.(*tg.MessagesChats)
	if !ok {
		return 0
	}

	for _, chat := range chats.Chats {
		if ch, ok := chat.(*tg.Channel); ok && ch.ID == channelID {
			c.channelAccessHash[channelID] = ch.AccessHash
			c.logger.Info("resolved channel access hash", "channel_id", channelID, "hash", ch.AccessHash)
			return ch.AccessHash
		}
	}

	return 0
}
