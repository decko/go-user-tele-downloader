// Package arr provides a thin client and importer for the *arr media
// managers. Radarr (movies) and Sonarr (series) share the same v3 API surface,
// so a single Client is parameterized by Kind.
package arr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Kind identifies which *arr system a client targets: Radarr (movies) or
// Sonarr (series). Both share the same v3 API surface, differing only in the
// resource noun ("movie" vs "series") and a few payload field names.
type Kind string

const (
	// KindMovie targets Radarr.
	KindMovie Kind = "movie"
	// KindSeries targets Sonarr.
	KindSeries Kind = "series"
)

// Client is a thin *arr API client parameterized by Kind.
type Client struct {
	baseURL string
	apiKey  string
	kind    Kind
	http    *http.Client
}

// Item is a lookup result (movie or series).
type Item struct {
	ID      int    `json:"id"`
	TMDBID  int    `json:"tmdbId"` // populated for movies
	TVDBID  int    `json:"tvdbId"` // populated for series
	Title   string `json:"title"`
	Year    int    `json:"year"`
	HasFile bool   `json:"hasFile"`
}

// Episode is a single series episode record (for manual-import resolution).
type Episode struct {
	ID            int `json:"id"`
	SeasonNumber  int `json:"seasonNumber"`
	EpisodeNumber int `json:"episodeNumber"`
}

// NewClient creates a new *arr API client for the given system kind.
func NewClient(baseURL, apiKey string, kind Kind) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		kind:    kind,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Lookup searches the *arr system's metadata provider for the given term and
// returns matches. Returned items may have ID == 0 when they are not yet added
// to the library.
func (c *Client) Lookup(ctx context.Context, term string) ([]Item, error) {
	var out []Item
	if err := c.get(ctx, "/api/v3/"+string(c.kind)+"/lookup", url.Values{"term": {term}}, &out); err != nil {
		return nil, fmt.Errorf("lookup %q: %w", term, err)
	}
	return out, nil
}

// Add adds the given item (identified by tmdbId for movies or tvdbId for
// series) to the library.
func (c *Client) Add(ctx context.Context, item Item, rootFolder string, qualityProfileID int) error {
	body := map[string]any{
		"title":            item.Title,
		"qualityProfileId": qualityProfileID,
		"rootFolderPath":   rootFolder,
		"monitored":        true,
	}
	if item.Year > 0 {
		body["year"] = item.Year
	}

	if c.kind == KindSeries {
		body["seasonFolder"] = true
		body["seriesType"] = "standard"
		body["addOptions"] = map[string]any{
			"searchForMissingEpisodes": false,
		}
		body["tvdbId"] = item.TVDBID
	} else {
		body["addOptions"] = map[string]any{
			"searchForMovie": false,
		}
		body["tmdbId"] = item.TMDBID
	}

	var out map[string]any
	if err := c.post(ctx, "/api/v3/"+string(c.kind), body, &out); err != nil {
		return fmt.Errorf("add %s %q: %w", c.kind, item.Title, err)
	}
	return nil
}

// GetEpisodes returns the episode records of a series for the given season.
func (c *Client) GetEpisodes(ctx context.Context, seriesID, seasonNumber int) ([]Episode, error) {
	params := url.Values{
		"seriesId":     {strconv.Itoa(seriesID)},
		"seasonNumber": {strconv.Itoa(seasonNumber)},
	}

	var out []Episode
	if err := c.get(ctx, "/api/v3/episode", params, &out); err != nil {
		return nil, fmt.Errorf("get episodes for series %d season %d: %w", seriesID, seasonNumber, err)
	}
	return out, nil
}

// ManualImport tells the *arr system to import the file at path for the given
// movie (season/episodeIDs ignored) or series/season/episodes. The system
// handles renaming, moving, and triggering library scans.
func (c *Client) ManualImport(ctx context.Context, path string, itemID, season int, episodeIDs []int) error {
	var body []map[string]any
	if c.kind == KindSeries {
		body = []map[string]any{
			{
				"path":         path,
				"seriesId":     itemID,
				"seasonNumber": season,
				"episodeIds":   episodeIDs,
			},
		}
	} else {
		body = []map[string]any{
			{
				"path":    path,
				"movieId": itemID,
			},
		}
	}

	var out []map[string]any
	if err := c.post(ctx, "/api/v3/manualimport", body, &out); err != nil {
		return fmt.Errorf("manual import %s: %w", path, err)
	}
	return nil
}

// get performs a GET request against the *arr API and decodes the JSON
// response into out.
func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Lookup/episode endpoints return 200 on success; accept the full 2xx
	// range to stay consistent with post (which tolerates 201 Created).
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("arr returned %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// post performs a POST request with a JSON body against the *arr API and
// decodes the JSON response into out (when non-nil).
func (c *Client) post(ctx context.Context, path string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("arr returned %d: %s", resp.StatusCode, string(body))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
