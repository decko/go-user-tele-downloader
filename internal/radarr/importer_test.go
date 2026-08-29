package radarr

import (
	"testing"
)

func TestParseName(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		wantTitle string
		wantYear  int
	}{
		{
			name:      "standard release",
			filename:  "Movie.Name.2026.1080p.WEB-DL.mkv",
			wantTitle: "Movie Name",
			wantYear:  2026,
		},
		{
			name:      "no year",
			filename:  "Inception.1080p.BluRay.x264.mkv",
			wantTitle: "Inception",
			wantYear:  0,
		},
		{
			name:      "spaces and dots mixed",
			filename:  "The Dark Knight 2008 2160p.mkv",
			wantTitle: "The Dark Knight",
			wantYear:  2008,
		},
		{
			name:      "tv episode with quality tokens",
			filename:  "Show.Name.S03E01.1080p.HEVC.mkv",
			wantTitle: "Show Name S03E01",
			wantYear:  0,
		},
		{
			name:      "document",
			filename:  "Annual.Report.2025.pdf",
			wantTitle: "Annual Report",
			wantYear:  2025,
		},
		{
			name:      "double digit year suffix",
			filename:  "Dune.Part.Two.2024.mkv",
			wantTitle: "Dune Part Two",
			wantYear:  2024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseName(tt.filename)
			if got.Title != tt.wantTitle {
				t.Errorf("ParseName(%q).Title = %q, want %q", tt.filename, got.Title, tt.wantTitle)
			}
			if got.Year != tt.wantYear {
				t.Errorf("ParseName(%q).Year = %d, want %d", tt.filename, got.Year, tt.wantYear)
			}
		})
	}
}

func TestMatchConfidence(t *testing.T) {
	tests := []struct {
		name     string
		parsed   ParsedName
		movie    Movie
		wantPass bool // passes the default 0.7 threshold
	}{
		{
			name:     "exact title and year",
			parsed:   ParsedName{Title: "Dune Part Two", Year: 2024},
			movie:    Movie{Title: "Dune: Part Two", Year: 2024},
			wantPass: true,
		},
		{
			name:     "exact title no year in filename",
			parsed:   ParsedName{Title: "Inception"},
			movie:    Movie{Title: "Inception", Year: 2010},
			wantPass: true,
		},
		{
			name:     "exact title wrong year",
			parsed:   ParsedName{Title: "Inception", Year: 2019},
			movie:    Movie{Title: "Inception", Year: 2010},
			wantPass: false,
		},
		{
			name:     "unrelated titles",
			parsed:   ParsedName{Title: "Annual Report", Year: 2025},
			movie:    Movie{Title: "The Matrix", Year: 1999},
			wantPass: false,
		},
		{
			name:     "partial containment with matching year",
			parsed:   ParsedName{Title: "News", Year: 2026},
			movie:    Movie{Title: "News of the World", Year: 2026},
			wantPass: true,
		},
		{
			name:     "partial containment without year",
			parsed:   ParsedName{Title: "News"},
			movie:    Movie{Title: "News of the World", Year: 2020},
			wantPass: false,
		},
		{
			name:     "tv episode vs movie",
			parsed:   ParsedName{Title: "Show Name S03E01"},
			movie:    Movie{Title: "Show Name", Year: 2020},
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := matchConfidence(tt.parsed, tt.movie)
			gotPass := score >= 0.7
			if gotPass != tt.wantPass {
				t.Errorf("matchConfidence(%+v, %+v) = %.2f, pass=%v, want pass=%v",
					tt.parsed, tt.movie, score, gotPass, tt.wantPass)
			}
		})
	}
}

func TestStrictThreshold(t *testing.T) {
	// Strict mode requires score >= 1.0: exact title AND matching year.
	tests := []struct {
		name     string
		parsed   ParsedName
		movie    Movie
		wantPass bool
	}{
		{
			name:     "exact title and year passes strict",
			parsed:   ParsedName{Title: "Dune", Year: 2021},
			movie:    Movie{Title: "Dune", Year: 2021},
			wantPass: true,
		},
		{
			name:     "exact title no year fails strict",
			parsed:   ParsedName{Title: "Inception"},
			movie:    Movie{Title: "Inception", Year: 2010},
			wantPass: false,
		},
		{
			name:     "containment fails strict",
			parsed:   ParsedName{Title: "News", Year: 2026},
			movie:    Movie{Title: "News of the World", Year: 2026},
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := matchConfidence(tt.parsed, tt.movie)
			gotPass := score >= 1.0
			if gotPass != tt.wantPass {
				t.Errorf("strict matchConfidence(%+v, %+v) = %.2f, pass=%v, want pass=%v",
					tt.parsed, tt.movie, score, gotPass, tt.wantPass)
			}
		})
	}
}
