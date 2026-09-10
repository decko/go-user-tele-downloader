package arr

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// arrExpectation describes one request the fake *arr server expects to receive,
// in order, and the JSON body it should reply with.
type arrExpectation struct {
	method       string
	path         string
	query        url.Values
	responseJSON string
	checkBody    func(t *testing.T, body []byte)
}

// fakeArr is an ordered httptest handler. It asserts the method, path, and
// selected query params of each request and fails the test on any mismatch or
// out-of-order request.
type fakeArr struct {
	t            *testing.T
	mu           sync.Mutex
	expectations []arrExpectation
	requests     []recordedRequest
}

type recordedRequest struct {
	method string
	path   string
	query  url.Values
	body   []byte
}

func (f *fakeArr) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	f.mu.Lock()
	idx := len(f.requests)
	f.requests = append(f.requests, recordedRequest{
		method: r.Method,
		path:   r.URL.Path,
		query:  r.URL.Query(),
		body:   body,
	})
	if idx >= len(f.expectations) {
		f.mu.Unlock()
		f.t.Errorf("unexpected request #%d: %s %s", idx, r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
		return
	}
	exp := f.expectations[idx]
	f.mu.Unlock()

	if r.Method != exp.method {
		f.t.Errorf("request #%d method = %s, want %s", idx, r.Method, exp.method)
	}
	if r.URL.Path != exp.path {
		f.t.Errorf("request #%d path = %s, want %s", idx, r.URL.Path, exp.path)
	}
	for key, want := range exp.query {
		got := r.URL.Query()[key]
		if !equalStrings(got, want) {
			f.t.Errorf("request #%d query %s = %v, want %v", idx, key, got, want)
		}
	}
	if got := r.Header.Get("X-Api-Key"); got != "test-key" {
		f.t.Errorf("request #%d X-Api-Key = %q, want %q", idx, got, "test-key")
	}
	if exp.checkBody != nil {
		exp.checkBody(f.t, body)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(exp.responseJSON))
}

// verify fails the test if the number of requests received differs from the
// number expected.
func (f *fakeArr) verify() {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) != len(f.expectations) {
		f.t.Errorf("got %d requests, want %d", len(f.requests), len(f.expectations))
	}
}

// requestCount returns how many requests the fake has served.
func (f *fakeArr) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestImportFile_MovieFlow(t *testing.T) {
	const moviePath = "/downloads/Movie.Name.2026.1080p.WEB-DL.mkv"

	fake := &fakeArr{
		t: t,
		expectations: []arrExpectation{
			{
				method:       http.MethodGet,
				path:         "/api/v3/movie/lookup",
				query:        url.Values{"term": {"Movie Name 2026"}},
				responseJSON: `[{"id":0,"tmdbId":12345,"title":"Movie Name","year":2026,"hasFile":false}]`,
			},
			{
				method:       http.MethodPost,
				path:         "/api/v3/movie",
				responseJSON: `{"id":42}`,
				checkBody:    expectMovieAddBody,
			},
			{
				method:       http.MethodGet,
				path:         "/api/v3/movie/lookup",
				query:        url.Values{"term": {"Movie Name 2026"}},
				responseJSON: `[{"id":42,"tmdbId":12345,"title":"Movie Name","year":2026,"hasFile":false}]`,
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
	if title != "Movie Name" {
		t.Errorf("ImportFile() title = %q, want %q", title, "Movie Name")
	}
	fake.verify()
}

func TestImportFile_SeriesFlow(t *testing.T) {
	const seriesPath = "/downloads/Show.Name.S01E02.1080p.mkv"

	fake := &fakeArr{
		t: t,
		expectations: []arrExpectation{
			{
				method:       http.MethodGet,
				path:         "/api/v3/series/lookup",
				query:        url.Values{"term": {"Show Name"}},
				responseJSON: `[{"id":0,"tvdbId":999,"title":"Show Name","year":2020,"hasFile":false}]`,
			},
			{
				method:       http.MethodPost,
				path:         "/api/v3/series",
				responseJSON: `{"id":7}`,
				checkBody:    expectSeriesAddBody,
			},
			{
				method:       http.MethodGet,
				path:         "/api/v3/series/lookup",
				query:        url.Values{"term": {"Show Name"}},
				responseJSON: `[{"id":7,"tvdbId":999,"title":"Show Name","year":2020,"hasFile":false}]`,
			},
			{
				method: http.MethodGet,
				path:   "/api/v3/episode",
				query: url.Values{
					"seriesId":     {"7"},
					"seasonNumber": {"1"},
				},
				responseJSON: `[{"id":100,"seasonNumber":1,"episodeNumber":2}]`,
			},
			{
				method:       http.MethodPost,
				path:         "/api/v3/manualimport",
				responseJSON: `[]`,
				checkBody: func(t *testing.T, body []byte) {
					expectManualImportSeries(t, body, seriesPath, 7, 1, []int{100})
				},
			},
		},
	}
	server := httptest.NewServer(fake)
	defer server.Close()

	c := NewClient(server.URL, "test-key", KindSeries)
	imp := NewImporter(c, KindSeries, "/tv", 1, false, discardLogger())

	title, err := imp.ImportFile(context.Background(), seriesPath)
	if err != nil {
		t.Fatalf("ImportFile() error = %v", err)
	}
	if title != "Show Name" {
		t.Errorf("ImportFile() title = %q, want %q", title, "Show Name")
	}
	fake.verify()
}

func TestImportFile_SeriesNoEpisode(t *testing.T) {
	fake := &fakeArr{t: t}
	server := httptest.NewServer(fake)
	defer server.Close()

	c := NewClient(server.URL, "test-key", KindSeries)
	imp := NewImporter(c, KindSeries, "/tv", 1, false, discardLogger())

	_, err := imp.ImportFile(context.Background(), "/downloads/Show.Name.S01.1080p.mkv")
	if !errors.Is(err, ErrNoEpisode) {
		t.Fatalf("ImportFile() error = %v, want %v", err, ErrNoEpisode)
	}
	if got := fake.requestCount(); got != 0 {
		t.Errorf("ImportFile() made %d requests, want 0 (should bail before Lookup)", got)
	}
}

// expectMovieAddBody asserts the POST /api/v3/movie payload shape from the spec.
func expectMovieAddBody(t *testing.T, body []byte) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Errorf("decode movie add body: %v", err)
		return
	}
	if got, want := m["tmdbId"], float64(12345); got != want {
		t.Errorf("movie add body tmdbId = %v, want %v", got, want)
	}
	if got, want := m["title"], "Movie Name"; got != want {
		t.Errorf("movie add body title = %v, want %v", got, want)
	}
	if got, want := m["qualityProfileId"], float64(1); got != want {
		t.Errorf("movie add body qualityProfileId = %v, want %v", got, want)
	}
	if got, want := m["rootFolderPath"], "/movies"; got != want {
		t.Errorf("movie add body rootFolderPath = %v, want %v", got, want)
	}
	if got, want := m["monitored"], true; got != want {
		t.Errorf("movie add body monitored = %v, want %v", got, want)
	}
	opts, ok := m["addOptions"].(map[string]any)
	if !ok {
		t.Errorf("movie add body addOptions missing or not an object: %v", m["addOptions"])
	} else if got, want := opts["searchForMovie"], false; got != want {
		t.Errorf("movie add body addOptions.searchForMovie = %v, want %v", got, want)
	}
}

// expectSeriesAddBody asserts the POST /api/v3/series payload shape from the spec.
func expectSeriesAddBody(t *testing.T, body []byte) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Errorf("decode series add body: %v", err)
		return
	}
	if got, want := m["tvdbId"], float64(999); got != want {
		t.Errorf("series add body tvdbId = %v, want %v", got, want)
	}
	if got, want := m["title"], "Show Name"; got != want {
		t.Errorf("series add body title = %v, want %v", got, want)
	}
	if got, want := m["qualityProfileId"], float64(1); got != want {
		t.Errorf("series add body qualityProfileId = %v, want %v", got, want)
	}
	if got, want := m["rootFolderPath"], "/tv"; got != want {
		t.Errorf("series add body rootFolderPath = %v, want %v", got, want)
	}
	if got, want := m["monitored"], true; got != want {
		t.Errorf("series add body monitored = %v, want %v", got, want)
	}
	if got, want := m["seasonFolder"], true; got != want {
		t.Errorf("series add body seasonFolder = %v, want %v", got, want)
	}
	if got, want := m["seriesType"], "standard"; got != want {
		t.Errorf("series add body seriesType = %v, want %v", got, want)
	}
	opts, ok := m["addOptions"].(map[string]any)
	if !ok {
		t.Errorf("series add body addOptions missing or not an object: %v", m["addOptions"])
	} else if got, want := opts["searchForMissingEpisodes"], false; got != want {
		t.Errorf("series add body addOptions.searchForMissingEpisodes = %v, want %v", got, want)
	}
}

// expectManualImportMovie asserts the movie manual-import payload.
func expectManualImportMovie(t *testing.T, body []byte, wantPath string, wantMovieID float64) {
	t.Helper()
	items, err := decodeManualImport(body)
	if err != nil {
		t.Errorf("decode movie manualimport body: %v", err)
		return
	}
	if len(items) != 1 {
		t.Errorf("movie manualimport body length = %d, want 1", len(items))
		return
	}
	if got := items[0]["path"]; got != wantPath {
		t.Errorf("movie manualimport path = %v, want %v", got, wantPath)
	}
	if got := items[0]["movieId"]; got != wantMovieID {
		t.Errorf("movie manualimport movieId = %v, want %v", got, wantMovieID)
	}
}

// expectManualImportSeries asserts the series manual-import payload.
func expectManualImportSeries(t *testing.T, body []byte, wantPath string, wantSeriesID, wantSeason float64, wantEpisodeIDs []int) {
	t.Helper()
	items, err := decodeManualImport(body)
	if err != nil {
		t.Errorf("decode series manualimport body: %v", err)
		return
	}
	if len(items) != 1 {
		t.Errorf("series manualimport body length = %d, want 1", len(items))
		return
	}
	if got := items[0]["path"]; got != wantPath {
		t.Errorf("series manualimport path = %v, want %v", got, wantPath)
	}
	if got := items[0]["seriesId"]; got != wantSeriesID {
		t.Errorf("series manualimport seriesId = %v, want %v", got, wantSeriesID)
	}
	if got := items[0]["seasonNumber"]; got != wantSeason {
		t.Errorf("series manualimport seasonNumber = %v, want %v", got, wantSeason)
	}
	rawIDs, ok := items[0]["episodeIds"].([]any)
	if !ok {
		t.Errorf("series manualimport episodeIds missing or not an array: %v", items[0]["episodeIds"])
		return
	}
	if len(rawIDs) != len(wantEpisodeIDs) {
		t.Errorf("series manualimport episodeIds = %v, want %v", rawIDs, wantEpisodeIDs)
		return
	}
	for i, want := range wantEpisodeIDs {
		if got := rawIDs[i]; got != float64(want) {
			t.Errorf("series manualimport episodeIds[%d] = %v, want %d", i, got, want)
		}
	}
}

// equalStrings compares two string slices, treating nil and empty as equal.
func equalStrings(a, b []string) bool {
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

func decodeManualImport(body []byte) ([]map[string]any, error) {
	var items []map[string]any
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}
	return items, nil
}
