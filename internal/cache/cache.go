// Package cache provides a caching layer for API responses.
//
// PURPOSE
// -------
// Implements disk-based caching to reduce API calls and enable offline testing.
// Cache files are stored in cache/ with SHA256 hashed filenames.
//
// For details on the caching strategy and rate limiting, see:
//   - ARCHITECTURE.md: "Intelligent Caching" section
//   - README.md: "Caching and Rate Limits" section
//
// KEY FEATURES
// ------------
// The caching system:
//   - Stores API responses on disk for reuse
//   - Supports Validation Mode (--test flag)
//   - Provides cache inspection and management tools (--cache mode)
//   - Handles past date queries with cache fallback
//   - Normalizes URLs for consistent cache keys (timestamps → dates)
//   - Tags each cache entry with its endpoint + system ID so the API client can
//     find the most recent cache for a given endpoint when budget is exhausted
//   - Maintains a sliding-window counter of recent live API calls in
//     cache/api_calls so the client can serve cache instead of issuing a call
//     that would exceed the per-API-key rate limit (MaxRequestsPerWindow per
//     MinRequestInterval)
package cache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"enphase-monitor/internal/constants"
)

// getCacheDir returns the cache directory path.
// ENPHASE_CACHE_DIR overrides the default so tests can redirect cache I/O to
// a temporary directory without touching the production cache.
func getCacheDir() string {
	if override := os.Getenv("ENPHASE_CACHE_DIR"); override != "" {
		return override
	}

	// Walk up the directory tree to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "cache"
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "cache")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "cache"
		}
		dir = parent
	}
}

// RedactURLKey strips the "key" query parameter from a URL for safe inclusion in error messages.
func RedactURLKey(rawURL string) string {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := parsed.Query()
	if q.Has("key") {
		q.Set("key", "REDACTED")
		parsed.RawQuery = q.Encode()
	}
	return parsed.String()
}

// MinRequestInterval is the width of the API Budget window. It serves two
// purposes:
//  1. Cache staleness: for current-period queries, a cache entry younger than
//     this is reused instead of refetched.
//  2. Sliding-window budget: RecordAPICall logs each live call and
//     RemainingBudget returns MaxRequestsPerWindow minus the count of logged
//     calls newer than now - MinRequestInterval. When budget reaches zero the
//     API client serves cache instead of issuing a call that would 429.
const MinRequestInterval = 1 * time.Minute

// Package-level state for cache configuration.
// These flags are set once at startup before any concurrent operations begin,
// so no mutex protection is needed.
var (
	validationMode    bool
	cacheDisabled     bool
	debugMode         bool
	budgetWarningShown bool
)

// ValidationMode returns whether Validation Mode is enabled.
func ValidationMode() bool {
	return validationMode
}

// SetValidationMode enables or disables Validation Mode.
func SetValidationMode(enabled bool) {
	validationMode = enabled
}

// CacheDisabled returns whether cache is disabled.
//
//nolint:revive // exported name clarifies package (cache.CacheDisabled)
func CacheDisabled() bool {
	return cacheDisabled
}

// SetCacheDisabled enables or disables cache bypass.
func SetCacheDisabled(disabled bool) {
	cacheDisabled = disabled
}

// DebugMode returns whether debug mode is enabled.
func DebugMode() bool { return debugMode }

// SetDebugMode enables or disables debug output.
func SetDebugMode(enabled bool) { debugMode = enabled }

// Debugf prints a debug message to stderr when debug mode is on.
func Debugf(format string, args ...any) {
	if !debugMode {
		return
	}
	fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
}

// LastAPICallTime returns the most recent recorded live API call timestamp.
// Returns (zero, false) when no calls have been recorded in the current window.
func LastAPICallTime() (time.Time, bool) {
	stamps := recentAPICallStampsLocked()
	if len(stamps) == 0 {
		return time.Time{}, false
	}
	latest := stamps[0]
	for _, t := range stamps[1:] {
		if t.After(latest) {
			latest = t
		}
	}
	return latest, true
}

// BudgetWarningShown returns whether an API budget warning has been shown.
func BudgetWarningShown() bool {
	return budgetWarningShown
}

// SetBudgetWarningShown sets the API budget warning flag.
func SetBudgetWarningShown(shown bool) {
	budgetWarningShown = shown
}

// ResetState resets all cache state flags to their default values and removes
// the sliding-window api_calls file. Primarily used for testing to ensure
// clean state between tests; safe to call in production (it only clears
// throttling state, never cache entries themselves).
func ResetState() {
	validationMode = false
	cacheDisabled = false
	debugMode = false
	budgetWarningShown = false
	ClearAPICalls()
}

// ClearAPICalls removes the sliding-window api_calls file. Use this in tests
// when you want a clean API budget but want to preserve the other
// state flags (ValidationMode, CacheDisabled, etc.).
func ClearAPICalls() {
	_ = os.Remove(filepath.Join(getCacheDir(), apiCallsFilename))
}

// cachedResponseMeta holds only the lookup fields of a CachedResponse.
// Used by FindMostRecentByEndpoint to scan files without allocating response bodies.
type cachedResponseMeta struct {
	CachedAt time.Time `json:"cached_at"`
	Endpoint string    `json:"endpoint,omitempty"`
	SystemID string    `json:"system_id,omitempty"`
}

// CachedResponse stores a cached API response
type CachedResponse struct {
	StatusCode  int               `json:"status_code"`
	Headers     map[string]string `json:"headers"`
	Body        []byte            `json:"body"`
	CachedAt    time.Time         `json:"cached_at"`
	QueriedDate string            `json:"queried_date,omitempty"` // The date that was queried (YYYY-MM-DD), not the API's start_date
	Endpoint    string            `json:"endpoint,omitempty"`     // API endpoint, e.g. "telemetry/production_meter", "energy_lifetime"
	SystemID    string            `json:"system_id,omitempty"`    // System ID from the request URL
}

// ToHTTPResponse reconstructs an *http.Response from a cached response.
func (c *CachedResponse) ToHTTPResponse() *http.Response {
	resp := &http.Response{
		StatusCode: c.StatusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(c.Body)),
	}
	for k, v := range c.Headers {
		resp.Header.Set(k, v)
	}
	return resp
}

// GetCacheKey generates a cache key from a URL
// Normalizes timestamps in the URL to use date strings for consistent caching
func GetCacheKey(url string, tz *time.Location) string {
	// Normalize the URL by replacing Unix timestamps with date strings
	// This allows the same endpoint/date to use the same cache key
	normalizedURL := NormalizeURLForCache(url, tz)
	hash := sha256.Sum256([]byte(normalizedURL))
	return hex.EncodeToString(hash[:])
}

// NormalizeURLForCache normalizes a URL by replacing timestamps with date strings
// for consistent cache keys regardless of the exact timestamp within a day.
// Example: ...start_at=1740960000&end_at=1741132800 -> ...start_date=2026-03-01&end_date=2026-03-06
// Example: ...start_date=2026-01-20&end_date=2026-01-20 -> ...start_date=2026-01-20&end_date=2026-01-20 (no change)
func NormalizeURLForCache(urlStr string, tz *time.Location) string {
	// Parse URL to extract query parameters
	parsedURL, err := neturl.Parse(urlStr)
	if err != nil {
		return urlStr // Return original if parsing fails
	}

	query := parsedURL.Query()

	// Normalize start_at and end_at timestamps to dates
	// Convert timestamps to the specified timezone
	if startAt := query.Get("start_at"); startAt != "" {
		if timestamp, err := strconv.ParseInt(startAt, 10, 64); err == nil {
			t := time.Unix(timestamp, 0).In(tz)
			dateStr := t.Format(constants.DateFormat)
			query.Del("start_at")
			query.Set("start_date", dateStr)
		}
	}
	if endAt := query.Get("end_at"); endAt != "" {
		if timestamp, err := strconv.ParseInt(endAt, 10, 64); err == nil {
			t := time.Unix(timestamp, 0).In(tz)
			dateStr := t.Format(constants.DateFormat)
			query.Del("end_at")
			query.Set("end_date", dateStr)
		}
	}

	// Rebuild URL with normalized query
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String()
}

// extractQueriedDateFromURL extracts the queried date from a URL
// This is the date that was queried, not the API's start_date field
func extractQueriedDateFromURL(urlStr string, tz *time.Location) string {
	parsedURL, err := neturl.Parse(urlStr)
	if err != nil {
		return ""
	}

	query := parsedURL.Query()

	// Check for start_at timestamp
	if startAt := query.Get("start_at"); startAt != "" {
		if timestamp, err := strconv.ParseInt(startAt, 10, 64); err == nil {
			t := time.Unix(timestamp, 0).In(tz)
			return t.Format(constants.DateFormat)
		}
	}

	// Check for start_date string
	if startDate := query.Get("start_date"); startDate != "" {
		return startDate
	}

	return ""
}

// ExtractEndpointAndSystemID parses an Enphase API request URL and returns the
// endpoint path (e.g. "telemetry/production_meter", "energy_lifetime") and the
// system ID. Returns empty strings if the URL doesn't match the expected shape.
//
// Expected URL path: /api/v4/systems/{system_id}/{endpoint...}
func ExtractEndpointAndSystemID(rawURL string) (endpoint, systemID string) {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, p := range parts {
		if p == "systems" && i+1 < len(parts) {
			systemID = parts[i+1]
			if i+2 < len(parts) {
				endpoint = strings.Join(parts[i+2:], "/")
			}
			return
		}
	}
	return "", ""
}

// apiCallsFilename stores the timestamps of recent live API calls as
// newline-separated RFC3339Nano values. It has no .json extension so the
// cache iteration loops (HasCacheForDate, ClearTodayCache, etc.) skip it
// naturally; ListCacheEntries has an explicit skip-list entry for it.
const apiCallsFilename = "api_calls"

// MaxRequestsPerWindow is the per-API-key rate limit (calls per
// MinRequestInterval). The Enphase Cloud API v4 allows 10 requests per minute
// per API key shared across all systems.
const MaxRequestsPerWindow = 10

// MaxCurrentDayCacheAge bounds how stale a today's-day-query cache may be
// before we treat it as expired. Today's numbers change throughout the day,
// so a stale cache would mislead the user about current production /
// consumption.
const MaxCurrentDayCacheAge = 1 * time.Hour

// MaxCurrentPeriodCacheAge bounds how stale a current-period cache may be
// for month-to-date / year-to-date / current-true-up-year queries. These
// totals are large cumulative numbers where a one-day delay in the refresh
// is acceptable but a multi-day delay would be misleading.
//
// Past Periods (already-ended Day / Month / Year / True-Up) never
// expire and are served regardless of age — see client.cacheMaxAge.
const MaxCurrentPeriodCacheAge = 24 * time.Hour

// RecordAPICall appends the current timestamp to the api_calls file and prunes
// entries older than MinRequestInterval. The pruning keeps the file small and
// avoids it growing unbounded over time. Failures are ignored — the counter
// is best-effort throttling state.
func RecordAPICall() {
	if err := os.MkdirAll(getCacheDir(), 0755); err != nil {
		return
	}
	stamps := recentAPICallStampsLocked()
	stamps = append(stamps, time.Now())
	writeAPICallStamps(stamps)
}

// RemainingBudget returns the number of API calls that can still be made in
// the current MinRequestInterval window without exceeding
// MaxRequestsPerWindow. Returns 0 when the budget is exhausted.
func RemainingBudget() int {
	used := len(recentAPICallStampsLocked())
	if used >= MaxRequestsPerWindow {
		return 0
	}
	return MaxRequestsPerWindow - used
}

// recentAPICallStampsLocked reads the api_calls file and returns the
// timestamps within the last MinRequestInterval. Malformed lines are skipped.
// "Locked" is aspirational — there is no actual mutex; the CLI is single-
// process and call sites are serialized within a run.
func recentAPICallStampsLocked() []time.Time {
	path := filepath.Join(getCacheDir(), apiCallsFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-MinRequestInterval)
	var stamps []time.Time
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, line)
		if err != nil {
			continue
		}
		if t.After(cutoff) {
			stamps = append(stamps, t)
		}
	}
	return stamps
}

// writeAPICallStamps overwrites the api_calls file with the given timestamps.
func writeAPICallStamps(stamps []time.Time) {
	var b strings.Builder
	for _, t := range stamps {
		b.WriteString(t.Format(time.RFC3339Nano))
		b.WriteByte('\n')
	}
	path := filepath.Join(getCacheDir(), apiCallsFilename)
	_ = os.WriteFile(path, []byte(b.String()), 0644)
}

// FindMostRecentByEndpoint scans the cache directory for entries matching the
// given endpoint and system ID and returns the one with the latest CachedAt.
// Returns nil and an error if no matching entry exists. Entries cached before
// the Endpoint/SystemID fields were introduced are skipped.
//
// If maxAge > 0, entries older than time.Now() - maxAge are filtered out.
// Pass 0 to accept entries of any age.
func FindMostRecentByEndpoint(endpoint, systemID string, maxAge time.Duration) (*CachedResponse, error) {
	if endpoint == "" || systemID == "" {
		return nil, fmt.Errorf("endpoint and systemID required")
	}
	dir := getCacheDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	// First pass: read only metadata (no body) to find the best matching path.
	var bestPath string
	var bestCachedAt time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), constants.JSONExtension) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var meta cachedResponseMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		if meta.Endpoint != endpoint || meta.SystemID != systemID {
			continue
		}
		if maxAge > 0 && time.Since(meta.CachedAt) > maxAge {
			continue
		}
		if bestPath == "" || meta.CachedAt.After(bestCachedAt) {
			bestPath = path
			bestCachedAt = meta.CachedAt
		}
	}
	if bestPath == "" {
		return nil, fmt.Errorf("no cache entry found for endpoint=%s system=%s", endpoint, systemID)
	}

	// Second pass: full load only for the winner.
	return LoadCachedResponseByPath(bestPath)
}

// GetCachePath returns the file path for a cached response
func GetCachePath(url string, tz *time.Location) string {
	key := GetCacheKey(url, tz)
	return filepath.Join(getCacheDir(), key+constants.JSONExtension)
}

// LoadCachedResponse loads a cached response from disk
func LoadCachedResponse(url string, tz *time.Location) (*CachedResponse, error) {
	cachePath := GetCachePath(url, tz)

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cache file does not exist: %s", cachePath)
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var cached CachedResponse
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("failed to parse cache file: %w", err)
	}

	return &cached, nil
}

// SaveCachedResponseFromBytes saves a response to the cache using pre-read body bytes
func SaveCachedResponseFromBytes(url string, resp *http.Response, bodyBytes []byte, tz *time.Location) error {
	// Create cache directory if it does not exist
	if err := os.MkdirAll(getCacheDir(), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Save headers
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	// Extract the queried date from the URL (this is the date we queried, not the API's start_date)
	queriedDate := extractQueriedDateFromURL(url, tz)
	endpoint, systemID := ExtractEndpointAndSystemID(url)

	cached := CachedResponse{
		StatusCode:  resp.StatusCode,
		Headers:     headers,
		Body:        bodyBytes,
		CachedAt:    time.Now(),
		QueriedDate: queriedDate,
		Endpoint:    endpoint,
		SystemID:    systemID,
	}

	// Marshal to JSON
	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	// Write to file
	cachePath := GetCachePath(url, tz)
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// HasCacheForDate checks if any cached responses exist for the specified date.
// Returns true if at least one cache file exists with a matching queried_date.
// This is used to validate that --test mode has data available before running.
func HasCacheForDate(targetDate string) (bool, error) {
	cacheDir := getCacheDir()

	// Check if cache directory exists
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return false, nil
	}

	// Read all files in cache directory
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return false, fmt.Errorf("failed to read cache directory: %w", err)
	}

	// Check each JSON file for matching queried_date
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), constants.JSONExtension) {
			continue
		}

		filePath := filepath.Join(cacheDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue // Skip files we can't read
		}

		var cached CachedResponse
		if err := json.Unmarshal(data, &cached); err != nil {
			continue // Skip files we can't parse
		}

		if cached.QueriedDate == targetDate {
			return true, nil
		}
	}

	return false, nil
}

// GetCacheDir returns the cache directory path for external use (e.g., error messages).
func GetCacheDir() string {
	return getCacheDir()
}
