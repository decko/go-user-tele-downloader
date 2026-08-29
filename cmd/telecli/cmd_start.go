package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/decko/go-user-tele-downloader/internal/client"
	"github.com/decko/go-user-tele-downloader/internal/config"
	"github.com/decko/go-user-tele-downloader/internal/domain"
	"github.com/decko/go-user-tele-downloader/internal/migration"
	"github.com/decko/go-user-tele-downloader/internal/repository"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Telegram download daemon",
	Long: `Start the Telegram download daemon that monitors configured channels
and automatically downloads files.

The daemon will:
1. Authenticate with Telegram (if not already authenticated)
2. Monitor specified channels for new file messages
3. Download files with progress tracking and resume support
4. Store download history in SQLite database

Example:
  telecli start
  telecli start --config /path/to/config.yaml`,
	RunE: runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set up logger
	logLevel := cfg.LogLevel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("starting telecli daemon",
		"phone", cfg.Phone,
		"channels", len(cfg.MonitorChannels),
		"download_dir", cfg.DownloadDir,
	)

	// Run migrations
	if err := runMigrations(cfg); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Initialize database
	db, err := repository.NewSQLiteDB(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Initialize services
	downloadRepo := repository.NewSQLiteDownloadRepository(db)
	downloadService := domain.NewDownloadService(downloadRepo, cfg.DownloadDir, cfg.MaxConcurrentDownloads)

	// Initialize Telegram client
	telegramClient := client.New(cfg, downloadService, logger)

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	// Start the client
	logger.Info("daemon started successfully")
	if err := telegramClient.Start(ctx); err != nil {
		if ctx.Err() != nil {
			logger.Info("daemon stopped gracefully")
			return nil
		}
		return fmt.Errorf("daemon error: %w", err)
	}

	return nil
}

func runMigrations(cfg *config.Config) error {
	db, err := repository.NewSQLiteDB(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	return migration.Run(db)
}
