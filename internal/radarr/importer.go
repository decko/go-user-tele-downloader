package radarr

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// ParsedName holds the movie title and year extracted from a release filename.
type ParsedName struct {
	Title string
	Year  int
}

// Importer orchestrates the lookup → add → manual import flow.
type Importer struct {
	client           *Client
	rootFolder       string
	qualityProfileID int
	strict           bool
	logger           *slog.Logger
}

// NewImporter creates a Radarr importer. When strict is true, only exact
// title+year matches are imported.
func NewImporter(client *Client, rootFolder string, qualityProfileID int, strict bool, logger *slog.Logger) *Importer {
	return &Importer{
		client:           client,
		rootFolder:       rootFolder,
		qualityProfileID: qualityProfileID,
		strict:           strict,
		logger:           logger,
	}
}

// ErrNoConfidentMatch is returned when a filename cannot be confidently
// matched to a movie. The file is left in the download folder untouched.
var ErrNoConfidentMatch = fmt.Errorf("no confident movie match, file left in downloads")

// ImportFile identifies the movie from the file name, adds it to Radarr if
// missing, and triggers a manual import. Returns the movie title on success.
func (i *Importer) ImportFile(ctx context.Context, path string) (string, error) {
	parsed := ParseName(filepath.Base(path))
	term := parsed.Title
	if parsed.Year > 0 {
		term = fmt.Sprintf("%s %d", term, parsed.Year)
	}

	// 1. Lookup the movie (searches TMDB via Radarr).
	movies, err := i.client.Lookup(ctx, term)
	if err != nil {
		return "", err
	}

	// 2. Pick the best match by confidence.
	best, bestScore := i.bestMatch(parsed, movies)
	if best == nil {
		return "", fmt.Errorf("%w: no results for %q", ErrNoConfidentMatch, term)
	}

	threshold := 0.7
	if i.strict {
		threshold = 1.0
	}
	if bestScore < threshold {
		i.logger.Warn("skipping import, weak match",
			"file", path, "candidate", best.Title, "year", best.Year, "score", bestScore, "threshold", threshold)
		return "", fmt.Errorf("%w: %q scored %.2f (need >= %.2f)", ErrNoConfidentMatch, best.Title, bestScore, threshold)
	}

	i.logger.Info("matched movie", "file", path, "title", best.Title, "year", best.Year, "score", bestScore)

	// 3. Add to Radarr if not already in the library.
	if best.ID == 0 {
		i.logger.Info("movie not in radarr, adding", "title", best.Title, "tmdb_id", best.TMDBID)
		if err := i.client.AddMovie(ctx, best.Title, best.Year, best.TMDBID, i.rootFolder, i.qualityProfileID); err != nil {
			return best.Title, fmt.Errorf("add movie: %w", err)
		}
		// Re-lookup to obtain the new Radarr ID.
		movies, err := i.client.Lookup(ctx, term)
		if err != nil {
			return best.Title, err
		}
		for idx := range movies {
			if movies[idx].TMDBID == best.TMDBID {
				best = &movies[idx]
				break
			}
		}
		if best.ID == 0 {
			return best.Title, fmt.Errorf("movie added but id not found for %q", term)
		}
	}

	// 4. Trigger manual import.
	i.logger.Info("importing to radarr", "title", best.Title, "movie_id", best.ID, "path", path)
	if err := i.client.ManualImport(ctx, path, best.ID); err != nil {
		return best.Title, err
	}

	return best.Title, nil
}

// bestMatch returns the highest-scoring movie for the parsed filename,
// or nil if there are no results.
func (i *Importer) bestMatch(parsed ParsedName, movies []Movie) (*Movie, float64) {
	var best *Movie
	var bestScore float64
	for idx := range movies {
		m := &movies[idx]
		score := matchConfidence(parsed, *m)
		if score > bestScore {
			best = m
			bestScore = score
		}
	}
	return best, bestScore
}

// matchConfidence scores how well a parsed filename matches a movie.
//
//	0.8  exact normalized title match
//	0.5  one title contains the other
//	+0.2 filename year equals movie year
//	-0.5 filename year present but differs from movie year
//
// A missing year in the filename adds nothing, so exact-title matches
// without a year still score 0.8 and pass the default threshold.
func matchConfidence(parsed ParsedName, movie Movie) float64 {
	ft := normalizeTitle(parsed.Title)
	mt := normalizeTitle(movie.Title)

	score := 0.0
	switch {
	case ft == mt:
		score += 0.8
	case strings.Contains(ft, mt) || strings.Contains(mt, ft):
		score += 0.5
	default:
		return 0.0 // unrelated titles
	}

	if parsed.Year > 0 {
		if movie.Year == parsed.Year {
			score += 0.2
		} else {
			score -= 0.5
		}
	}

	return score
}

// ParseName extracts the title and year from a release-style filename.
// "Movie.Name.2026.1080p.WEB-DL.mkv" → {Title: "Movie Name", Year: 2026}
func ParseName(filename string) ParsedName {
	base := filepath.Base(filename)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(base)
	fields := strings.Fields(base)

	var titleParts []string
	year := 0
	for _, f := range fields {
		if y, ok := parseYear(f); ok {
			year = y
			break
		}
		if isQualityToken(f) {
			continue
		}
		titleParts = append(titleParts, f)
	}

	return ParsedName{
		Title: strings.Join(titleParts, " "),
		Year:  year,
	}
}

func parseYear(s string) (int, bool) {
	if len(s) != 4 {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	y := 0
	for _, r := range s {
		y = y*10 + int(r-'0')
	}
	if y < 1900 || y > 2100 {
		return 0, false
	}
	return y, true
}

// normalizeTitle lowercases, strips punctuation, and collapses whitespace
// so that "Dune: Part Two" matches "Dune Part Two" and "Dune.Part.Two".
func normalizeTitle(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r == ' ', r == '\t':
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func isQualityToken(s string) bool {
	switch strings.ToLower(s) {
	case "1080p", "2160p", "720p", "4k", "web", "webdl", "web-dl", "bluray", "blu-ray",
		"x264", "x265", "h264", "h265", "dual", "5.1", "7.1", "hevc", "aac", "ac3",
		"hdrip", "bdrip", "webrip", "hdtv", "hdr", "dv", "10bit":
		return true
	}
	return false
}
