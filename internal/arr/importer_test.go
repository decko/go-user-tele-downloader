package arr

import (
	"testing"
)

// TestParseName covers the kind-aware filename parser. Movie expectations are
// carried over verbatim from the old internal/radarr package; series cases add
// SxxEyy detection on top.
func TestParseName(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		kind         Kind
		wantTitle    string
		wantYear     int
		wantSeason   int
		wantEpisodes []int
	}{
		// --- KindMovie: existing behavior must not change. ---
		{
			name:      "standard release",
			filename:  "Movie.Name.2026.1080p.WEB-DL.mkv",
			kind:      KindMovie,
			wantTitle: "Movie Name",
			wantYear:  2026,
		},
		{
			name:      "no year",
			filename:  "Inception.1080p.BluRay.x264.mkv",
			kind:      KindMovie,
			wantTitle: "Inception",
			wantYear:  0,
		},
		{
			name:      "spaces and dots mixed",
			filename:  "The Dark Knight 2008 2160p.mkv",
			kind:      KindMovie,
			wantTitle: "The Dark Knight",
			wantYear:  2008,
		},
		{
			name:      "tv episode with quality tokens",
			filename:  "Show.Name.S03E01.1080p.HEVC.mkv",
			kind:      KindMovie,
			wantTitle: "Show Name S03E01",
			wantYear:  0,
		},
		{
			name:      "document",
			filename:  "Annual.Report.2025.pdf",
			kind:      KindMovie,
			wantTitle: "Annual Report",
			wantYear:  2025,
		},
		{
			name:      "double digit year suffix",
			filename:  "Dune.Part.Two.2024.mkv",
			kind:      KindMovie,
			wantTitle: "Dune Part Two",
			wantYear:  2024,
		},

		// --- KindSeries: SxxEyy tokens are stripped and parsed. ---
		{
			name:         "series single episode",
			filename:     "Show.Name.S01E02.1080p.mkv",
			kind:         KindSeries,
			wantTitle:    "Show Name",
			wantYear:     0,
			wantSeason:   1,
			wantEpisodes: []int{2},
		},
		{
			name:         "series multi episode",
			filename:     "Show.Name.S01E01E02.720p.mkv",
			kind:         KindSeries,
			wantTitle:    "Show Name",
			wantYear:     0,
			wantSeason:   1,
			wantEpisodes: []int{1, 2},
		},
		{
			name:         "series season without episode",
			filename:     "Show.Name.S01.1080p.mkv",
			kind:         KindSeries,
			wantTitle:    "Show Name",
			wantYear:     0,
			wantSeason:   1,
			wantEpisodes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseName(tt.filename, tt.kind)
			if got.Title != tt.wantTitle {
				t.Errorf("ParseName(%q, %q).Title = %q, want %q", tt.filename, tt.kind, got.Title, tt.wantTitle)
			}
			if got.Year != tt.wantYear {
				t.Errorf("ParseName(%q, %q).Year = %d, want %d", tt.filename, tt.kind, got.Year, tt.wantYear)
			}
			if got.Season != tt.wantSeason {
				t.Errorf("ParseName(%q, %q).Season = %d, want %d", tt.filename, tt.kind, got.Season, tt.wantSeason)
			}
			if !equalInts(got.Episodes, tt.wantEpisodes) {
				t.Errorf("ParseName(%q, %q).Episodes = %v, want %v", tt.filename, tt.kind, got.Episodes, tt.wantEpisodes)
			}
		})
	}
}

func TestMatchConfidence(t *testing.T) {
	tests := []struct {
		name     string
		parsed   ParsedName
		item     Item
		wantPass bool // passes the default 0.7 threshold
	}{
		{
			name:     "exact title and year",
			parsed:   ParsedName{Title: "Dune Part Two", Year: 2024},
			item:     Item{Title: "Dune: Part Two", Year: 2024},
			wantPass: true,
		},
		{
			name:     "exact title no year in filename",
			parsed:   ParsedName{Title: "Inception"},
			item:     Item{Title: "Inception", Year: 2010},
			wantPass: true,
		},
		{
			name:     "exact title wrong year",
			parsed:   ParsedName{Title: "Inception", Year: 2019},
			item:     Item{Title: "Inception", Year: 2010},
			wantPass: false,
		},
		{
			name:     "unrelated titles",
			parsed:   ParsedName{Title: "Annual Report", Year: 2025},
			item:     Item{Title: "The Matrix", Year: 1999},
			wantPass: false,
		},
		{
			name:     "partial containment with matching year",
			parsed:   ParsedName{Title: "News", Year: 2026},
			item:     Item{Title: "News of the World", Year: 2026},
			wantPass: true,
		},
		{
			name:     "partial containment without year",
			parsed:   ParsedName{Title: "News"},
			item:     Item{Title: "News of the World", Year: 2020},
			wantPass: false,
		},
		{
			name:     "tv episode vs movie",
			parsed:   ParsedName{Title: "Show Name S03E01"},
			item:     Item{Title: "Show Name", Year: 2020},
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := matchConfidence(tt.parsed, tt.item)
			gotPass := score >= 0.7
			if gotPass != tt.wantPass {
				t.Errorf("matchConfidence(%+v, %+v) = %.2f, pass=%v, want pass=%v",
					tt.parsed, tt.item, score, gotPass, tt.wantPass)
			}
		})
	}
}

func TestStrictThreshold(t *testing.T) {
	// Strict mode requires score >= 1.0: exact title AND matching year.
	tests := []struct {
		name     string
		parsed   ParsedName
		item     Item
		wantPass bool
	}{
		{
			name:     "exact title and year passes strict",
			parsed:   ParsedName{Title: "Dune", Year: 2021},
			item:     Item{Title: "Dune", Year: 2021},
			wantPass: true,
		},
		{
			name:     "exact title no year fails strict",
			parsed:   ParsedName{Title: "Inception"},
			item:     Item{Title: "Inception", Year: 2010},
			wantPass: false,
		},
		{
			name:     "containment fails strict",
			parsed:   ParsedName{Title: "News", Year: 2026},
			item:     Item{Title: "News of the World", Year: 2026},
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := matchConfidence(tt.parsed, tt.item)
			gotPass := score >= 1.0
			if gotPass != tt.wantPass {
				t.Errorf("strict matchConfidence(%+v, %+v) = %.2f, pass=%v, want pass=%v",
					tt.parsed, tt.item, score, gotPass, tt.wantPass)
			}
		})
	}
}

// equalInts compares two int slices, treating nil and empty as equal.
func equalInts(a, b []int) bool {
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
