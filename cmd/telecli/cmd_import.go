package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/decko/go-user-tele-downloader/internal/arr"
	"github.com/decko/go-user-tele-downloader/internal/config"
)

// importType selects which *arr system receives the imported file.
var importType string

var importCmd = &cobra.Command{
	Use:   "import <file-or-folder>",
	Short: "Import a downloaded file into Radarr or Sonarr",
	Long: `Import a file (or folder) into a *arr library. The movie or series is
looked up by filename, added to the library if missing, and imported.

The target is selected with --type (default "movie"):

  movie   Radarr   requires RADARR_URL and RADARR_API_KEY
  series  Sonarr   requires SONARR_URL and SONARR_API_KEY

Examples:
  telecli import /data/downloads/Movie.2026.1080p.mkv
  telecli import --type series /data/downloads/Show.S01E02.1080p.mkv
  telecli import /data/downloads/Some.Folder`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

func init() {
	importCmd.Flags().StringVar(&importType, "type", "movie", `content type to import: "movie" or "series"`)
	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	path := args[0]

	kind, err := parseImportType(importType)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	url, apiKey, rootFolder, qualityProfile, strict, system, err := selectArrConfig(cfg, kind)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel(),
	}))

	client := arr.NewClient(url, apiKey, kind)
	importer := arr.NewImporter(client, kind, rootFolder, qualityProfile, strict, logger)

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

	fmt.Printf("Imported %q as %q into %s\n", importPath, title, system)
	return nil
}

// parseImportType maps the --type flag value to an arr.Kind.
func parseImportType(value string) (arr.Kind, error) {
	switch value {
	case string(arr.KindMovie):
		return arr.KindMovie, nil
	case string(arr.KindSeries):
		return arr.KindSeries, nil
	default:
		return "", fmt.Errorf("invalid --type %q: must be %q or %q", value, arr.KindMovie, arr.KindSeries)
	}
}

// selectArrConfig returns the connection settings and display name for the
// *arr system matching kind. It errors when that system is not configured.
func selectArrConfig(cfg *config.Config, kind arr.Kind) (url, apiKey, rootFolder string, qualityProfile int, strict bool, system string, err error) {
	if kind == arr.KindSeries {
		if cfg.SonarrURL == "" || cfg.SonarrAPIKey == "" {
			return "", "", "", 0, false, "", errors.New("sonarr is not configured: set SONARR_URL and SONARR_API_KEY")
		}
		return cfg.SonarrURL, cfg.SonarrAPIKey, cfg.SonarrRootFolder, cfg.SonarrQualityProfile, cfg.SonarrImportStrict, "Sonarr", nil
	}
	if cfg.RadarrURL == "" || cfg.RadarrAPIKey == "" {
		return "", "", "", 0, false, "", errors.New("radarr is not configured: set RADARR_URL and RADARR_API_KEY")
	}
	return cfg.RadarrURL, cfg.RadarrAPIKey, cfg.RadarrRootFolder, cfg.RadarrQualityProfile, cfg.RadarrImportStrict, "Radarr", nil
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
	return "", errors.New("no media file found")
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
