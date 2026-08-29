package radarr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is a thin Radarr API client.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// Movie is a Radarr movie record (subset of the API response).
type Movie struct {
	ID      int    `json:"id"`
	TMDBID  int    `json:"tmdbId"`
	Title   string `json:"title"`
	Year    int    `json:"year"`
	HasFile bool   `json:"hasFile"`
}

// NewClient creates a Radarr API client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: stringsTrimSlash(baseURL),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Lookup searches TMDB via Radarr for the given term and returns matches.
// The returned movie may have ID == 0 when it is not yet added to Radarr.
func (c *Client) Lookup(ctx context.Context, term string) ([]Movie, error) {
	var out []Movie
	if err := c.get(ctx, "/api/v3/movie/lookup", url.Values{"term": {term}}, &out); err != nil {
		return nil, fmt.Errorf("lookup %q: %w", term, err)
	}
	return out, nil
}

// AddMovie adds a movie (identified by tmdbID) to Radarr's library.
func (c *Client) AddMovie(ctx context.Context, title string, year, tmdbID int, rootFolder string, qualityProfileID int) error {
	body := map[string]any{
		"title":            title,
		"qualityProfileId": qualityProfileID,
		"rootFolderPath":   rootFolder,
		"monitored":        true,
		"addOptions": map[string]any{
			"searchForMovie": false,
		},
		"tmdbId": tmdbID,
	}
	if year > 0 {
		body["year"] = year
	}

	var out map[string]any
	if err := c.post(ctx, "/api/v3/movie", body, &out); err != nil {
		return fmt.Errorf("add movie %s (%d): %w", title, tmdbID, err)
	}
	return nil
}

// ManualImport tells Radarr to import the file at path for the given movie.
// Radarr handles renaming, moving, and triggering library scans.
func (c *Client) ManualImport(ctx context.Context, path string, movieID int) error {
	body := []map[string]any{
		{
			"path":    path,
			"movieId": movieID,
		},
	}

	var out []map[string]any
	if err := c.post(ctx, "/api/v3/manualimport", body, &out); err != nil {
		return fmt.Errorf("manual import %s (movie %d): %w", path, movieID, err)
	}
	return nil
}

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("radarr returned %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

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
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("radarr returned %d: %s", resp.StatusCode, string(body))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func stringsTrimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
