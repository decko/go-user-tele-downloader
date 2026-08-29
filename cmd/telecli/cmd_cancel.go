package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/decko/go-user-tele-downloader/internal/config"
	"github.com/decko/go-user-tele-downloader/internal/model"
	"github.com/decko/go-user-tele-downloader/internal/repository"
)

var cancelCmd = &cobra.Command{
	Use:   "cancel [download-id]",
	Short: "Cancel a pending or downloading download",
	Long: `Cancel a download by its ID. Only pending and downloading downloads
can be cancelled.

The download ID can be found using the 'list' command.

Examples:
  telecli cancel abc12345
  telecli cancel abc12345 --config /path/to/config.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: runCancel,
}

func init() {
	rootCmd.AddCommand(cancelCmd)
}

func runCancel(cmd *cobra.Command, args []string) error {
	downloadID := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	db, err := repository.NewSQLiteDB(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	repo := repository.NewSQLiteDownloadRepository(db)

	// Get the download
	download, err := repo.GetByID(context.Background(), downloadID)
	if err != nil {
		return fmt.Errorf("download not found: %w", err)
	}

	// Check if it can be cancelled
	if download.Status != model.DownloadStatusPending && download.Status != model.DownloadStatusDownloading {
		return fmt.Errorf("cannot cancel download with status: %s (only pending or downloading can be cancelled)", download.Status)
	}

	// Cancel the download
	if err := repo.UpdateStatus(context.Background(), downloadID, model.DownloadStatusCancelled, "Cancelled by user"); err != nil {
		return fmt.Errorf("failed to cancel download: %w", err)
	}

	fmt.Printf("Download %s cancelled successfully\n", downloadID)
	return nil
}
