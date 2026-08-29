package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/decko/go-user-tele-downloader/internal/config"
	"github.com/decko/go-user-tele-downloader/internal/model"
	"github.com/decko/go-user-tele-downloader/internal/repository"
)

var (
	listLimit  int
	listStatus string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List downloads",
	Long: `List downloads with detailed information including ID, filename,
status, progress, size, and timestamps.

Use flags to filter by status or limit the number of results.

Examples:
  telecli list
  telecli list --limit 10
  telecli list --status downloading
  telecli list --status failed`,
	RunE: runList,
}

func init() {
	listCmd.Flags().IntVarP(&listLimit, "limit", "l", 20, "Maximum number of downloads to show")
	listCmd.Flags().StringVarP(&listStatus, "status", "s", "", "Filter by status (pending, downloading, completed, failed, cancelled)")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
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

	// Get downloads
	downloads, err := repo.ListAll(context.Background())
	if err != nil {
		return fmt.Errorf("failed to list downloads: %w", err)
	}

	// Filter by status if specified
	if listStatus != "" {
		filtered := make([]*model.Download, 0)
		for _, d := range downloads {
			if string(d.Status) == listStatus {
				filtered = append(filtered, d)
			}
		}
		downloads = filtered
	}

	// Apply limit
	if listLimit > 0 && len(downloads) > listLimit {
		downloads = downloads[:listLimit]
	}

	if len(downloads) == 0 {
		fmt.Println("No downloads found")
		return nil
	}

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tFILENAME\tSTATUS\tPROGRESS\tSIZE\tCREATED")
	fmt.Fprintln(w, "--\t--------\t------\t--------\t----\t-------")

	for _, d := range downloads {
		filename := d.URL
		if len(filename) > 30 {
			filename = filename[:27] + "..."
		}

		progress := "-"
		if d.Status == "downloading" {
			progress = fmt.Sprintf("%.1f%%", d.Progress)
		}

		size := formatSize(d.FileSize)
		created := d.CreatedAt.Format("2006-01-02 15:04")

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			d.ID[:8],
			filename,
			d.Status,
			progress,
			size,
			created,
		)
	}

	w.Flush()
	return nil
}

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
