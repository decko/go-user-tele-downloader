package config

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration for the application.
type Config struct {
	// MTProto authentication
	APIID                     int
	APIHash                   string
	Phone                     string
	Password                  string // 2FA password (optional)
	SessionPath               string // Path to encrypted session file
	SessionEncryptionPassword string // Password for session encryption

	// Channel monitoring
	MonitorChannels []int64 // Channel IDs to monitor for downloads

	// Channel routing
	MovieChannels []int64 // Channels routed to Radarr (movies)
	TVChannels    []int64 // Channels routed to Sonarr (series)

	// Download settings
	DownloadDir            string
	MaxConcurrentDownloads int

	// Media library / Radarr (movies)
	MediaDir             string // Root folder of the media collection
	RadarrURL            string // Optional Radarr server URL
	RadarrAPIKey         string // Optional Radarr API key
	RadarrRootFolder     string // Radarr root folder for auto-added movies
	RadarrQualityProfile int    // Radarr quality profile ID for auto-added movies
	RadarrImportStrict   bool   // Only import exact title+year matches

	// Media library / Sonarr (series)
	SonarrURL            string // Optional Sonarr server URL
	SonarrAPIKey         string // Optional Sonarr API key
	SonarrRootFolder     string // Sonarr root folder for auto-added series
	SonarrQualityProfile int    // Sonarr quality profile ID for auto-added series
	SonarrImportStrict   bool   // Only import exact title+year matches

	PlexURL        string // Optional Plex server URL for scan triggers
	PlexToken      string // Optional Plex API token
	JellyfinURL    string // Optional Jellyfin server URL for scan triggers
	JellyfinAPIKey string // Optional Jellyfin API key

	// Database
	DatabasePath string

	// Logging
	LogLevelValue string
}

// ContentType classifies how a downloaded file is routed after completion.
type ContentType int

const (
	// ContentTypeMovie routes to Radarr.
	ContentTypeMovie ContentType = iota
	// ContentTypeSeries routes to Sonarr.
	ContentTypeSeries
	// ContentTypeDownloadOnly leaves the file in downloads/ (no import).
	ContentTypeDownloadOnly
)

// ContentTypeFor returns the routing type for a monitored channel. When no
// routing is configured (both lists empty) every channel is a movie for
// backward compatibility. Otherwise unlisted channels are download-only.
func (c *Config) ContentTypeFor(channelID int64) ContentType {
	if len(c.MovieChannels) == 0 && len(c.TVChannels) == 0 {
		return ContentTypeMovie
	}
	if containsChannel(c.MovieChannels, channelID) {
		return ContentTypeMovie
	}
	if containsChannel(c.TVChannels, channelID) {
		return ContentTypeSeries
	}
	return ContentTypeDownloadOnly
}

// Validate checks that MOVIE_CHANNELS and TV_CHANNELS are disjoint and both
// are subsets of MONITOR_CHANNELS.
func (c *Config) Validate() error {
	for _, id := range c.MovieChannels {
		if containsChannel(c.TVChannels, id) {
			return fmt.Errorf("channel %d is listed in both MOVIE_CHANNELS and TV_CHANNELS", id)
		}
	}
	for _, id := range c.MovieChannels {
		if !containsChannel(c.MonitorChannels, id) {
			return fmt.Errorf("MOVIE_CHANNELS channel %d is not in MONITOR_CHANNELS", id)
		}
	}
	for _, id := range c.TVChannels {
		if !containsChannel(c.MonitorChannels, id) {
			return fmt.Errorf("TV_CHANNELS channel %d is not in MONITOR_CHANNELS", id)
		}
	}
	return nil
}

// containsChannel reports whether channelID is present in ids.
func containsChannel(ids []int64, channelID int64) bool {
	for _, id := range ids {
		if id == channelID {
			return true
		}
	}
	return false
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	// Load .env file if it exists (doesn't override existing env vars)
	loadEnvFile(".env")

	// MTProto authentication (required)
	apiIDStr := os.Getenv("TELEGRAM_API_ID")
	if apiIDStr == "" {
		return nil, fmt.Errorf("TELEGRAM_API_ID environment variable is required")
	}

	apiID, err := strconv.Atoi(apiIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid TELEGRAM_API_ID: %w", err)
	}

	apiHash := os.Getenv("TELEGRAM_API_HASH")
	if apiHash == "" {
		return nil, fmt.Errorf("TELEGRAM_API_HASH environment variable is required")
	}

	phone := os.Getenv("TELEGRAM_PHONE")
	if phone == "" {
		return nil, fmt.Errorf("TELEGRAM_PHONE environment variable is required")
	}

	cfg := &Config{
		APIID:                     apiID,
		APIHash:                   apiHash,
		Phone:                     phone,
		Password:                  os.Getenv("TELEGRAM_PASSWORD"),
		SessionPath:               envOrDefault("SESSION_PATH", "./session.enc"),
		SessionEncryptionPassword: getSessionEncryptionPassword(),
		DownloadDir:               envOrDefault("DOWNLOAD_DIR", "./downloads"),
		MaxConcurrentDownloads:    envOrDefaultInt("MAX_CONCURRENT_DOWNLOADS", 3),
		MediaDir:                  os.Getenv("MEDIA_DIR"),
		RadarrURL:                 os.Getenv("RADARR_URL"),
		RadarrAPIKey:              os.Getenv("RADARR_API_KEY"),
		RadarrRootFolder:          os.Getenv("RADARR_ROOT_FOLDER"),
		RadarrQualityProfile:      envOrDefaultInt("RADARR_QUALITY_PROFILE_ID", 1),
		RadarrImportStrict:        envOrDefaultBool("RADARR_IMPORT_STRICT", false),
		SonarrURL:                 os.Getenv("SONARR_URL"),
		SonarrAPIKey:              os.Getenv("SONARR_API_KEY"),
		SonarrRootFolder:          os.Getenv("SONARR_ROOT_FOLDER"),
		SonarrQualityProfile:      envOrDefaultInt("SONARR_QUALITY_PROFILE_ID", 1),
		SonarrImportStrict:        envOrDefaultBool("SONARR_IMPORT_STRICT", false),
		PlexURL:                   os.Getenv("PLEX_URL"),
		PlexToken:                 os.Getenv("PLEX_TOKEN"),
		JellyfinURL:               os.Getenv("JELLYFIN_URL"),
		JellyfinAPIKey:            os.Getenv("JELLYFIN_API_KEY"),
		DatabasePath:              envOrDefault("DATABASE_PATH", "./tele-downloader.db"),
		LogLevelValue:             envOrDefault("LOG_LEVEL", "info"),
	}

	// Parse channel IDs to monitor and route.
	if cfg.MonitorChannels, err = parseChannels("MONITOR_CHANNELS", os.Getenv("MONITOR_CHANNELS")); err != nil {
		return nil, err
	}
	if cfg.MovieChannels, err = parseChannels("MOVIE_CHANNELS", os.Getenv("MOVIE_CHANNELS")); err != nil {
		return nil, err
	}
	if cfg.TVChannels, err = parseChannels("TV_CHANNELS", os.Getenv("TV_CHANNELS")); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// parseChannels parses a comma-separated list of int64 channel IDs. It returns
// nil for an empty value. The key is used only in error messages.
func parseChannels(key, value string) ([]int64, error) {
	if value == "" {
		return nil, nil
	}

	var channels []int64
	for _, s := range strings.Split(value, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", key, err)
		}
		channels = append(channels, id)
	}
	return channels, nil
}

// LogLevel returns the slog.Level for the configured log level.
func (c *Config) LogLevel() slog.Level {
	switch strings.ToLower(c.LogLevelValue) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envOrDefaultInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func envOrDefaultBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return b
}

// getSessionEncryptionPassword returns the session encryption password.
// If 2FA password is set, it uses that. Otherwise, it uses SESSION_ENCRYPTION_PASSWORD.
func getSessionEncryptionPassword() string {
	if password := os.Getenv("TELEGRAM_PASSWORD"); password != "" {
		return password
	}
	return os.Getenv("SESSION_ENCRYPTION_PASSWORD")
}

// loadEnvFile reads a .env file and sets environment variables.
// Existing environment variables are not overridden.
func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return // .env file is optional
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// Don't override existing environment variables
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}
