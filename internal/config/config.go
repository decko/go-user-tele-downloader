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
	APIID       int
	APIHash     string
	Phone       string
	Password    string // 2FA password (optional)
	SessionPath string // Path to encrypted session file

	// Channel monitoring
	MonitorChannels []int64 // Channel IDs to monitor for downloads

	// Download settings
	DownloadDir            string
	MaxConcurrentDownloads int

	// Database
	DatabasePath string

	// Logging
	LogLevelValue string
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
		APIID:                  apiID,
		APIHash:                apiHash,
		Phone:                  phone,
		Password:               os.Getenv("TELEGRAM_PASSWORD"),
		SessionPath:            envOrDefault("SESSION_PATH", "./session.enc"),
		DownloadDir:            envOrDefault("DOWNLOAD_DIR", "./downloads"),
		MaxConcurrentDownloads: envOrDefaultInt("MAX_CONCURRENT_DOWNLOADS", 3),
		DatabasePath:           envOrDefault("DATABASE_PATH", "./tele-downloader.db"),
		LogLevelValue:          envOrDefault("LOG_LEVEL", "info"),
	}

	// Parse channel IDs to monitor
	if channels := os.Getenv("MONITOR_CHANNELS"); channels != "" {
		for _, s := range strings.Split(channels, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing MONITOR_CHANNELS: %w", err)
			}
			cfg.MonitorChannels = append(cfg.MonitorChannels, id)
		}
	}

	return cfg, nil
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

// loadEnvFile reads a .env file and sets environment variables.
// Existing environment variables are not overridden.
func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return // .env file is optional
	}
	defer file.Close()

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
			os.Setenv(key, value)
		}
	}
}
