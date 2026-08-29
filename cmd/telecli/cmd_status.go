package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/decko/go-user-tele-downloader/internal/config"
	"github.com/decko/go-user-tele-downloader/internal/repository"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show download status summary",
	Long: `Show a summary of download status including counts of pending,
downloading, completed, and failed downloads.

Example:
  telecli status
  telecli status --config /path/to/config.yaml`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
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

	// Get all downloads
	downloads, err := repo.ListAll(context.Background())
	if err != nil {
		return fmt.Errorf("failed to list downloads: %w", err)
	}

	// Count by status
	statusCounts := make(map[string]int)
	var totalSize int64
	var downloadedSize int64

	for _, d := range downloads {
		statusCounts[string(d.Status)]++
		totalSize += d.FileSize
		if d.Status == "completed" {
			downloadedSize += d.FileSize
		}
	}

	// Print summary
	fmt.Println("Download Status Summary")
	fmt.Println("=======================")
	fmt.Printf("Total Downloads:    %d\n", len(downloads))
	fmt.Printf("Pending:            %d\n", statusCounts["pending"])
	fmt.Printf("Downloading:        %d\n", statusCounts["downloading"])
	fmt.Printf("Completed:          %d\n", statusCounts["completed"])
	fmt.Printf("Failed:             %d\n", statusCounts["failed"])
	fmt.Printf("Cancelled:          %d\n", statusCounts["cancelled"])
	fmt.Println()
	fmt.Printf("Total Size:         %.2f GB\n", float64(totalSize)/(1024*1024*1024))
	fmt.Printf("Downloaded:         %.2f GB\n", float64(downloadedSize)/(1024*1024*1024))

	return nil
}
