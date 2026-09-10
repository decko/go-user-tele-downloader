package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/decko/go-user-tele-downloader/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current configuration",
	Long: `Show the current configuration loaded from environment variables
and/or config file. This is useful for debugging configuration issues.

Example:
  telecli config`,
	RunE: runConfig,
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("Current Configuration")
	fmt.Println("=====================")
	fmt.Printf("API ID:              %d\n", cfg.APIID)
	fmt.Printf("API Hash:            %s\n", maskString(cfg.APIHash, 8))
	fmt.Printf("Phone:               %s\n", maskString(cfg.Phone, 4))
	fmt.Printf("2FA Password:        %s\n", maskString(cfg.Password, 0))
	fmt.Printf("Session Path:        %s\n", cfg.SessionPath)
	fmt.Printf("Session Password:    %s\n", maskString(cfg.SessionEncryptionPassword, 0))
	fmt.Printf("Database Path:       %s\n", cfg.DatabasePath)
	fmt.Printf("Download Dir:        %s\n", cfg.DownloadDir)
	fmt.Printf("Max Concurrent:      %d\n", cfg.MaxConcurrentDownloads)
	fmt.Printf("Log Level:           %s\n", cfg.LogLevelValue)
	fmt.Printf("Monitor Channels:    %v\n", cfg.MonitorChannels)
	fmt.Printf("Movie Channels:      %v\n", cfg.MovieChannels)
	fmt.Printf("TV Channels:         %v\n", cfg.TVChannels)

	return nil
}

func maskString(s string, showChars int) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= showChars {
		return "****"
	}
	return s[:showChars] + "****"
}
