package arr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
		// --- Alternate-title scoring: localized release names should match
		// the *arr primary (usually English) title. ---
		{
			name:     "exact alternate title match with year",
			parsed:   ParsedName{Title: "Cidade de Deus", Year: 2002},
			item:     Item{Title: "City of God", Year: 2002, AlternateTitles: []AlternateTitle{{Title: "Cidade de Deus"}}},
			wantPass: true,
		},
		{
			name:     "alternate title containment",
			parsed:   ParsedName{Title: "Cidade", Year: 2002},
			item:     Item{Title: "City of God", Year: 2002, AlternateTitles: []AlternateTitle{{Title: "Cidade de Deus"}}},
			wantPass: true,
		},
		{
			name:     "alternate title match but wrong year",
			parsed:   ParsedName{Title: "Cidade de Deus", Year: 1999},
			item:     Item{Title: "City of God", Year: 2002, AlternateTitles: []AlternateTitle{{Title: "Cidade de Deus"}}},
			wantPass: false,
		},
		{
			name:     "no primary or alternate match",
			parsed:   ParsedName{Title: "Something Else"},
			item:     Item{Title: "City of God", AlternateTitles: []AlternateTitle{{Title: "Cidade de Deus"}}},
			wantPass: false,
		},
		{
			name:     "non-latin alternate title does not match",
			parsed:   ParsedName{Title: "Some Random Movie", Year: 2001},
			item:     Item{Title: "Spirited Away", Year: 2001, AlternateTitles: []AlternateTitle{{Title: "千と千尋の神隠し"}}},
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

// TestImportFile_LocalizedYearFallback covers a localized (non-English)
// filename whose title does not match the *arr primary title, but whose year
// matches a single lookup result. The importer should fall back to the
// year match and complete the add + manual-import flow.
func TestImportFile_LocalizedYearFallback(t *testing.T) {
	const (
		moviePath = "/downloads/Patrulha_Canina_Uma_Aventura_Dino_2026.1080p.mkv"
		term      = "Patrulha Canina Uma Aventura Dino 2026"
	)

	fake := &fakeArr{
		t: t,
		expectations: []arrExpectation{
			{
				method:       http.MethodGet,
				path:         "/api/v3/movie/lookup",
				query:        url.Values{"term": {term}},
				responseJSON: `[{"tmdbId":1185806,"title":"PAW Patrol: The Dino Movie","year":2026}]`,
			},
			{
				method:       http.MethodPost,
				path:         "/api/v3/movie",
				responseJSON: `{"id":42}`,
			},
			{
				method:       http.MethodGet,
				path:         "/api/v3/movie/lookup",
				query:        url.Values{"term": {term}},
				responseJSON: `[{"id":42,"tmdbId":1185806,"title":"PAW Patrol: The Dino Movie","year":2026}]`,
			},
			{
				method:       http.MethodPost,
				path:         "/api/v3/manualimport",
				responseJSON: `[]`,
				checkBody: func(t *testing.T, body []byte) {
					expectManualImportMovie(t, body, moviePath, 42)
				},
			},
		},
	}
	server := httptest.NewServer(fake)
	defer server.Close()

	c := NewClient(server.URL, "test-key", KindMovie)
	imp := NewImporter(c, KindMovie, "/movies", 1, false, discardLogger())

	title, err := imp.ImportFile(context.Background(), moviePath)
	if err != nil {
		t.Fatalf("ImportFile() error = %v", err)
	}
	if title != "PAW Patrol: The Dino Movie" {
		t.Errorf("ImportFile() title = %q, want %q", title, "PAW Patrol: The Dino Movie")
	}
	fake.verify()
}

// TestImportFile_LocalizedStrictRejects covers the same localized filename in
// strict mode: a year-only fallback must not satisfy the strict 1.0 threshold,
// so the importer bails out after the lookup without adding anything.
func TestImportFile_LocalizedStrictRejects(t *testing.T) {
	const (
		moviePath = "/downloads/Patrulha_Canina_Uma_Aventura_Dino_2026.1080p.mkv"
		term      = "Patrulha Canina Uma Aventura Dino 2026"
	)

	fake := &fakeArr{
		t: t,
		expectations: []arrExpectation{
			{
				method:       http.MethodGet,
				path:         "/api/v3/movie/lookup",
				query:        url.Values{"term": {term}},
				responseJSON: `[{"tmdbId":1185806,"title":"PAW Patrol: The Dino Movie","year":2026}]`,
			},
		},
	}
	server := httptest.NewServer(fake)
	defer server.Close()

	c := NewClient(server.URL, "test-key", KindMovie)
	imp := NewImporter(c, KindMovie, "/movies", 1, true, discardLogger())

	_, err := imp.ImportFile(context.Background(), moviePath)
	if !errors.Is(err, ErrNoConfidentMatch) {
		t.Fatalf("ImportFile() error = %v, want wrapping %v", err, ErrNoConfidentMatch)
	}
	fake.verify()
}

// TestImportFile_LookupNoResults covers an empty lookup response. The error
// must wrap ErrNoConfidentMatch and be distinguishable by its "no results"
// text.
func TestImportFile_LookupNoResults(t *testing.T) {
	const term = "Some Title 2026"

	fake := &fakeArr{
		t: t,
		expectations: []arrExpectation{
			{
				method:       http.MethodGet,
				path:         "/api/v3/movie/lookup",
				query:        url.Values{"term": {term}},
				responseJSON: `[]`,
			},
		},
	}
	server := httptest.NewServer(fake)
	defer server.Close()

	c := NewClient(server.URL, "test-key", KindMovie)
	imp := NewImporter(c, KindMovie, "/movies", 1, false, discardLogger())

	_, err := imp.ImportFile(context.Background(), "/downloads/Some.Title.2026.mkv")
	if !errors.Is(err, ErrNoConfidentMatch) {
		t.Fatalf("ImportFile() error = %v, want wrapping %v", err, ErrNoConfidentMatch)
	}
	if !strings.Contains(err.Error(), "no results") {
		t.Errorf("ImportFile() error = %q, want it to contain %q", err, "no results")
	}
	fake.verify()
}

// TestImportFile_NoTitleMatch covers a lookup that returns results but none of
// them match the filename title or year. The error must wrap
// ErrNoConfidentMatch and be distinguishable from the empty-lookup case by its
// "no title match" text.
func TestImportFile_NoTitleMatch(t *testing.T) {
	const term = "Some Title 2026"

	fake := &fakeArr{
		t: t,
		expectations: []arrExpectation{
			{
				method:       http.MethodGet,
				path:         "/api/v3/movie/lookup",
				query:        url.Values{"term": {term}},
				responseJSON: `[{"tmdbId":1,"title":"Completely Unrelated","year":2020},{"tmdbId":2,"title":"Also Unrelated","year":2021}]`,
			},
		},
	}
	server := httptest.NewServer(fake)
	defer server.Close()

	c := NewClient(server.URL, "test-key", KindMovie)
	imp := NewImporter(c, KindMovie, "/movies", 1, false, discardLogger())

	_, err := imp.ImportFile(context.Background(), "/downloads/Some.Title.2026.mkv")
	if !errors.Is(err, ErrNoConfidentMatch) {
		t.Fatalf("ImportFile() error = %v, want wrapping %v", err, ErrNoConfidentMatch)
	}
	if !strings.Contains(err.Error(), "no title match") {
		t.Errorf("ImportFile() error = %q, want it to contain %q", err, "no title match")
	}
	fake.verify()
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
