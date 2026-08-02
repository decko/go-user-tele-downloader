package client

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"github.com/decko/go-user-tele-downloader/internal/config"
	"github.com/decko/go-user-tele-downloader/internal/domain"
)

// TelegramClient wraps the MTProto client with download functionality.
type TelegramClient struct {
	client  *telegram.Client
	api     *tg.Client
	service *domain.DownloadService
	config  *config.Config
	logger  *slog.Logger
}

// New creates a new TelegramClient instance.
func New(cfg *config.Config, service *domain.DownloadService, logger *slog.Logger) *TelegramClient {
	return &TelegramClient{
		service: service,
		config:  cfg,
		logger:  logger,
	}
}

// Start initializes the MTProto client and begins monitoring channels.
func (c *TelegramClient) Start(ctx context.Context) error {
	// Create session storage
	storage := &session.FileStorage{
		Path: c.config.SessionPath,
	}

	// Create MTProto client
	c.client = telegram.NewClient(c.config.APIID, c.config.APIHash, telegram.Options{
		SessionStorage: storage,
	})

	// Run client
	return c.client.Run(ctx, func(ctx context.Context) error {
		// Authenticate if needed
		if err := c.authenticate(ctx); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		// Get client API
		c.api = c.client.API()

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

	// TODO: Implement authentication flow
	// This will be implemented in Phase 2
	return fmt.Errorf("authentication not yet implemented")
}

// monitorChannels subscribes to channel updates and processes new messages.
func (c *TelegramClient) monitorChannels(ctx context.Context) error {
	// TODO: Implement channel monitoring
	// This will be implemented in Phase 3
	c.logger.Info("channel monitoring not yet implemented")
	return nil
}
