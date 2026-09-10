package arr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ParsedName holds the title, year, and (for series) season/episode numbers
// extracted from a release filename.
type ParsedName struct {
	Title    string
	Year     int
	Season   int
	Episodes []int // empty for movies / files without an episode token
}

// ErrNoConfidentMatch is returned when a filename cannot be confidently
// matched to an item in the library. The file is left in the download folder
// untouched.
var ErrNoConfidentMatch = errors.New("no confident match, file left in downloads")

// ErrNoEpisode is returned when a series import is attempted for a filename
// that has a season but no episode token. The file is left in the download
// folder untouched.
var ErrNoEpisode = errors.New("no episode token in filename, file left in downloads")

// seriesTokenRe matches a season/episode field such as "S01", "S01E02", or
// "S01E01E02" (case-insensitive). The season-only form is accepted so that a
// file like "Show.Name.S01.1080p.mkv" can still be attributed to a season.
var seriesTokenRe = regexp.MustCompile(`(?i)^s(\d{1,2})((?:e\d{2})*)$`)

// episodeTokenRe extracts each episode number from the episode portion of a
// series token (e.g. "E01E02" → [1, 2]).
var episodeTokenRe = regexp.MustCompile(`(?i)e(\d{2})`)

// Importer orchestrates the lookup → add → manual import flow for one Kind.
type Importer struct {
	client           *Client
	kind             Kind
	rootFolder       string
	qualityProfileID int
	strict           bool
	logger           *slog.Logger
}

// NewImporter creates an importer for the given kind. When strict is true,
// only exact title+year matches are imported.
func NewImporter(client *Client, kind Kind, rootFolder string, qualityProfileID int, strict bool, logger *slog.Logger) *Importer {
	return &Importer{
		client:           client,
		kind:             kind,
		rootFolder:       rootFolder,
		qualityProfileID: qualityProfileID,
		strict:           strict,
		logger:           logger,
	}
}

// ImportFile identifies the movie or series from the file name, adds it to the
// *arr library if missing, and triggers a manual import. Returns the item
// title on success.
func (i *Importer) ImportFile(ctx context.Context, path string) (string, error) {
	parsed := ParseName(filepath.Base(path), i.kind)

	// Series without an episode token cannot be resolved to episode IDs, so
	// bail out before making any network calls.
	if i.kind == KindSeries && len(parsed.Episodes) == 0 {
		return "", fmt.Errorf("%w: %q", ErrNoEpisode, path)
	}

	term := parsed.Title
	if parsed.Year > 0 {
		term = fmt.Sprintf("%s %d", term, parsed.Year)
	}

	// 1. Look up the item (searches the metadata provider via the *arr API).
	items, err := i.client.Lookup(ctx, term)
	if err != nil {
		return "", err
	}

	// 2. Pick the best match by confidence.
	best, bestScore := i.bestMatch(parsed, items)
	if best == nil {
		return "", fmt.Errorf("%w: no results for %q", ErrNoConfidentMatch, term)
	}

	threshold := 0.7
	if i.strict {
		threshold = 1.0
	}
	if bestScore < threshold {
		i.logger.Warn("skipping import, weak match",
			"kind", i.kind, "file", path, "candidate", best.Title, "year", best.Year, "score", bestScore, "threshold", threshold)
		return "", fmt.Errorf("%w: %q scored %.2f (need >= %.2f)", ErrNoConfidentMatch, best.Title, bestScore, threshold)
	}

	i.logger.Info("matched item", "kind", i.kind, "file", path, "title", best.Title, "year", best.Year, "score", bestScore)

	// 3. Add to the library if not already present.
	if best.ID == 0 {
		i.logger.Info("item not in arr, adding", "kind", i.kind, "title", best.Title)
		if err := i.client.Add(ctx, *best, i.rootFolder, i.qualityProfileID); err != nil {
			return best.Title, fmt.Errorf("add %s: %w", i.kind, err)
		}
		// Re-lookup to obtain the newly assigned library ID.
		items, err := i.client.Lookup(ctx, term)
		if err != nil {
			return best.Title, err
		}
		best = findAdded(items, i.kind, *best)
		if best == nil || best.ID == 0 {
			return "", fmt.Errorf("%s added but id not found for %q", i.kind, term)
		}
	}

	// 4. Trigger the manual import. Series need their episode IDs resolved
	// from the parsed episode numbers first.
	if i.kind == KindSeries {
		episodes, err := i.client.GetEpisodes(ctx, best.ID, parsed.Season)
		if err != nil {
			return best.Title, err
		}
		episodeIDs := mapEpisodeIDs(parsed.Episodes, episodes)
		if len(episodeIDs) == 0 {
			return best.Title, fmt.Errorf("episodes %v not found for series %q season %d", parsed.Episodes, best.Title, parsed.Season)
		}
		i.logger.Info("importing to arr", "kind", i.kind, "title", best.Title, "series_id", best.ID, "path", path)
		if err := i.client.ManualImport(ctx, path, best.ID, parsed.Season, episodeIDs); err != nil {
			return best.Title, err
		}
		return best.Title, nil
	}

	i.logger.Info("importing to arr", "kind", i.kind, "title", best.Title, "movie_id", best.ID, "path", path)
	if err := i.client.ManualImport(ctx, path, best.ID, 0, nil); err != nil {
		return best.Title, err
	}
	return best.Title, nil
}

// bestMatch returns the highest-scoring item for the parsed filename, or nil
// if there are no results.
func (i *Importer) bestMatch(parsed ParsedName, items []Item) (*Item, float64) {
	var best *Item
	var bestScore float64
	for idx := range items {
		it := &items[idx]
		score := matchConfidence(parsed, *it)
		if score > bestScore {
			best = it
			bestScore = score
		}
	}
	return best, bestScore
}

// findAdded returns the re-looked-up item that corresponds to target, matching
// on tvdbId for series and tmdbId for movies. It returns nil when no item
// matches.
func findAdded(items []Item, kind Kind, target Item) *Item {
	for idx := range items {
		it := &items[idx]
		if kind == KindSeries {
			if it.TVDBID == target.TVDBID {
				return it
			}
			continue
		}
		if it.TMDBID == target.TMDBID {
			return it
		}
	}
	return nil
}

// mapEpisodeIDs resolves parsed episode numbers to *arr episode IDs. Numbers
// that have no matching episode record are omitted.
func mapEpisodeIDs(numbers []int, episodes []Episode) []int {
	var ids []int
	for _, want := range numbers {
		for _, ep := range episodes {
			if ep.EpisodeNumber == want {
				ids = append(ids, ep.ID)
				break
			}
		}
	}
	return ids
}

// matchConfidence scores how well a parsed filename matches an item.
//
//	0.8  exact normalized title match
//	0.5  one title contains the other
//	+0.2 filename year equals item year
//	-0.5 filename year present but differs from item year
//
// A missing year in the filename adds nothing, so exact-title matches without
// a year still score 0.8 and pass the default threshold.
func matchConfidence(parsed ParsedName, item Item) float64 {
	ft := normalizeTitle(parsed.Title)
	mt := normalizeTitle(item.Title)

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
		if item.Year == parsed.Year {
			score += 0.2
		} else {
			score -= 0.5
		}
	}

	return score
}

// ParseName extracts the title and year (movies) or title, year, season, and
// episodes (series) from a release-style filename. Season/episode tokens are
// only stripped for KindSeries:
//
//	ParseName("Movie.Name.2026.1080p.WEB-DL.mkv", KindMovie)
//	  → {Title: "Movie Name", Year: 2026}
//	ParseName("Show.Name.S01E02.1080p.mkv", KindSeries)
//	  → {Title: "Show Name", Season: 1, Episodes: [2]}
func ParseName(filename string, kind Kind) ParsedName {
	base := filepath.Base(filename)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(base)
	fields := strings.Fields(base)

	var parsed ParsedName

	// Locate and decode the season/episode token for series.
	seasonIdx := -1
	if kind == KindSeries {
		for idx, f := range fields {
			m := seriesTokenRe.FindStringSubmatch(f)
			if m == nil {
				continue
			}
			seasonIdx = idx
			parsed.Season, _ = strconv.Atoi(m[1])
			for _, em := range episodeTokenRe.FindAllStringSubmatch(m[2], -1) {
				n, _ := strconv.Atoi(em[1])
				parsed.Episodes = append(parsed.Episodes, n)
			}
			break
		}
	}

	var titleParts []string
	for idx, f := range fields {
		if idx == seasonIdx {
			continue
		}
		if y, ok := parseYear(f); ok {
			parsed.Year = y
			break
		}
		if isQualityToken(f) {
			continue
		}
		titleParts = append(titleParts, f)
	}

	parsed.Title = strings.Join(titleParts, " ")
	return parsed
}

// parseYear reports whether s is a four-digit year in the plausible range
// [1900, 2100].
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

// isQualityToken reports whether s is a release quality/codec token that
// should be excluded from a parsed title.
func isQualityToken(s string) bool {
	switch strings.ToLower(s) {
	case "1080p", "2160p", "720p", "4k", "web", "webdl", "web-dl", "bluray", "blu-ray",
		"x264", "x265", "h264", "h265", "dual", "5.1", "7.1", "hevc", "aac", "ac3",
		"hdrip", "bdrip", "webrip", "hdtv", "hdr", "dv", "10bit":
		return true
	}
	return false
}
