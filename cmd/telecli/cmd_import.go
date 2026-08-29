package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/decko/go-user-tele-downloader/internal/config"
	"github.com/decko/go-user-tele-downloader/internal/radarr"
)

var importCmd = &cobra.Command{
	Use:   "import <file-or-folder>",
	Short: "Import a downloaded file into Radarr",
	Long: `Import a file (or folder) into Radarr's library. The movie is
looked up by filename, added to Radarr if missing, and imported.

This requires Radarr to be configured (RADARR_URL and RADARR_API_KEY).

Examples:
  telecli import /data/downloads/Movie.2026.1080p.mkv
  telecli import /data/downloads/Some.Folder`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

func init() {
	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	path := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.RadarrURL == "" || cfg.RadarrAPIKey == "" {
		return fmt.Errorf("Radarr is not configured: set RADARR_URL and RADARR_API_KEY")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel(),
	}))

	rc := radarr.NewClient(cfg.RadarrURL, cfg.RadarrAPIKey)
	importer := radarr.NewImporter(rc, cfg.RadarrRootFolder, cfg.RadarrQualityProfile, cfg.RadarrImportStrict, logger)

	// If given a folder, import the first media file in it; otherwise import the file.
	importPath := path
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat %q: %w", path, err)
	}
	if info.IsDir() {
		importPath, err = findMediaFile(path)
		if err != nil {
			return fmt.Errorf("no media file found in %q: %w", path, err)
		}
	}

	title, err := importer.ImportFile(context.Background(), importPath)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	fmt.Printf("Imported %q as %q into Radarr\n", importPath, title)
	return nil
}

// findMediaFile returns the first media file in a folder.
func findMediaFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if isMediaExt(e.Name()) {
			return dir + "/" + e.Name(), nil
		}
	}
	return "", fmt.Errorf("no media file found")
}

func isMediaExt(name string) bool {
	exts := []string{".mkv", ".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".ts"}
	for _, ext := range exts {
		if len(name) >= len(ext) && name[len(name)-len(ext):] == ext {
			return true
		}
	}
	return false
}
