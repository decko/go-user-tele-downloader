package config

import "testing"

// setRequiredEnv sets the environment variables Load() requires so tests can
// exercise the channel/Sonarr parsing without touching real credentials.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TELEGRAM_API_ID", "12345")
	t.Setenv("TELEGRAM_API_HASH", "abc123")
	t.Setenv("TELEGRAM_PHONE", "+15550001111")
}

func TestLoadChannels(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MONITOR_CHANNELS", "-1001,-1002")
	t.Setenv("MOVIE_CHANNELS", "-1001")
	t.Setenv("TV_CHANNELS", "-1002")
	t.Setenv("SONARR_URL", "http://sonarr:8989")
	t.Setenv("SONARR_API_KEY", "sonarr-key")
	t.Setenv("SONARR_ROOT_FOLDER", "/tv")
	t.Setenv("SONARR_QUALITY_PROFILE_ID", "4")
	t.Setenv("SONARR_IMPORT_STRICT", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !equalInt64(cfg.MonitorChannels, []int64{-1001, -1002}) {
		t.Errorf("MonitorChannels = %v, want [-1001 -1002]", cfg.MonitorChannels)
	}
	if !equalInt64(cfg.MovieChannels, []int64{-1001}) {
		t.Errorf("MovieChannels = %v, want [-1001]", cfg.MovieChannels)
	}
	if !equalInt64(cfg.TVChannels, []int64{-1002}) {
		t.Errorf("TVChannels = %v, want [-1002]", cfg.TVChannels)
	}
	if cfg.SonarrURL != "http://sonarr:8989" {
		t.Errorf("SonarrURL = %q, want %q", cfg.SonarrURL, "http://sonarr:8989")
	}
	if cfg.SonarrAPIKey != "sonarr-key" {
		t.Errorf("SonarrAPIKey = %q, want %q", cfg.SonarrAPIKey, "sonarr-key")
	}
	if cfg.SonarrRootFolder != "/tv" {
		t.Errorf("SonarrRootFolder = %q, want %q", cfg.SonarrRootFolder, "/tv")
	}
	if cfg.SonarrQualityProfile != 4 {
		t.Errorf("SonarrQualityProfile = %d, want 4", cfg.SonarrQualityProfile)
	}
	if !cfg.SonarrImportStrict {
		t.Errorf("SonarrImportStrict = false, want true")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid disjoint subset",
			cfg: Config{
				MonitorChannels: []int64{-1, -2},
				MovieChannels:   []int64{-1},
				TVChannels:      []int64{-2},
			},
			wantErr: false,
		},
		{
			name: "empty routing is valid",
			cfg: Config{
				MonitorChannels: []int64{-1},
			},
			wantErr: false,
		},
		{
			name: "channel in both lists",
			cfg: Config{
				MonitorChannels: []int64{-1, -2},
				MovieChannels:   []int64{-1},
				TVChannels:      []int64{-1},
			},
			wantErr: true,
		},
		{
			name: "movie channel not monitored",
			cfg: Config{
				MonitorChannels: []int64{-1},
				MovieChannels:   []int64{-2},
			},
			wantErr: true,
		},
		{
			name: "tv channel not monitored",
			cfg: Config{
				MonitorChannels: []int64{-1},
				TVChannels:      []int64{-2},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContentTypeFor(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		channel int64
		want    ContentType
	}{
		{
			name:    "backward compatible both empty routes to movie",
			cfg:     Config{MonitorChannels: []int64{-1}},
			channel: -1,
			want:    ContentTypeMovie,
		},
		{
			name:    "listed movie channel",
			cfg:     Config{MovieChannels: []int64{-1}, TVChannels: []int64{-2}},
			channel: -1,
			want:    ContentTypeMovie,
		},
		{
			name:    "listed tv channel",
			cfg:     Config{MovieChannels: []int64{-1}, TVChannels: []int64{-2}},
			channel: -2,
			want:    ContentTypeSeries,
		},
		{
			name:    "unlisted channel with routing active",
			cfg:     Config{MovieChannels: []int64{-1}, TVChannels: []int64{-2}},
			channel: -3,
			want:    ContentTypeDownloadOnly,
		},
		{
			name:    "only tv list configured leaves unlisted download only",
			cfg:     Config{TVChannels: []int64{-2}},
			channel: -1,
			want:    ContentTypeDownloadOnly,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			if got := cfg.ContentTypeFor(tt.channel); got != tt.want {
				t.Errorf("ContentTypeFor(%d) = %v, want %v", tt.channel, got, tt.want)
			}
		})
	}
}

func TestLoadValidate(t *testing.T) {
	t.Run("overlap", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("MONITOR_CHANNELS", "-1,-2")
		t.Setenv("MOVIE_CHANNELS", "-1")
		t.Setenv("TV_CHANNELS", "-1")

		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want overlap validation error")
		}
	})

	t.Run("non-subset", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("MONITOR_CHANNELS", "-1")
		t.Setenv("MOVIE_CHANNELS", "-2")

		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want non-subset validation error")
		}
	})
}

// equalInt64 compares two int64 slices, treating nil and empty as equal.
func equalInt64(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
