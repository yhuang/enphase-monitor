// Package cache provides a caching layer for API responses.
//
// PURPOSE
// -------
// Implements disk-based caching to reduce API calls and enable offline testing.
// Cache files are stored in cache/ with SHA256 hashed filenames.
//
// For details on the caching strategy and API Budget, see:
//   - docs/ARCHITECTURE.md: "API Budget Checks" section
//   - README.md: "Caching and API Budget" section
//
// KEY FEATURES
// ------------
// The caching system:
//   - Stores API responses on disk for reuse
//   - Supports Validation Mode (--test flag) and Cached Mode (--cache flag)
//   - Exposes inspection and clear-cache helpers (cli.go) used by --clear-cache, --clear-cache-date, and --clear-all-cache
//   - Handles past date queries with cache fallback
//   - Normalizes URLs for consistent cache keys (timestamps → dates)
//   - Tags each cache entry with its endpoint + system ID so the API client can
//     find the most recent cache for a given endpoint when budget is exhausted
//
// Per-credential API budget tracking lives in internal/credentials (pool quota).
package cache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// Package-level state for cache configuration.
// These flags are set once at startup before any concurrent operations begin,
// so no mutex protection is needed.
var (
	validationMode bool
	cacheDisabled  bool
	debugMode      bool
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

// ResetState resets all cache state flags to their default values. Primarily
// used for testing to ensure clean state between tests.
func ResetState() {
	validationMode = false
	cacheDisabled = false
	debugMode = false
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

// FindMostRecentByEndpoint scans the cache directory for entries matching the
// given endpoint and system ID and returns the one with the latest CachedAt.
// Returns nil and an error if no matching entry exists. Entries cached before
// the Endpoint/SystemID fields were introduced are skipped.
//
// If maxAge > 0, entries older than time.Now() - maxAge are filtered out.
// Pass 0 to accept entries of any age.
func FindMostRecentByEndpoint(endpoint, systemID string, maxAge time.Duration) (*CachedResponse, error) {
	if endpoint == "" || systemID == "" {
		return nil, errors.New("endpoint and systemID required")
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
	if err := os.MkdirAll(getCacheDir(), 0o755); err != nil {
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
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// forEachCacheEntry invokes fn for every parseable JSON cache file in the cache
// directory, passing the file path and decoded response. Files that cannot be
// read or unmarshalled are skipped silently. fn returns false to stop iteration
// early. A missing cache directory is treated as empty (no error); only a
// failure to read an existing directory is returned as an error.
func forEachCacheEntry(fn func(path string, cached *CachedResponse) bool) error {
	dir := getCacheDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), constants.JSONExtension) {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Skip files we can't read
		}

		var cached CachedResponse
		if err := json.Unmarshal(data, &cached); err != nil {
			continue // Skip files we can't parse
		}

		if !fn(path, &cached) {
			break
		}
	}

	return nil
}

// HasCacheForDate checks if any cached responses exist for the specified date.
// Returns true if at least one cache file exists with a matching queried_date.
// This is used to validate that --test mode has data available before running.
func HasCacheForDate(targetDate string) (bool, error) {
	var found bool
	err := forEachCacheEntry(func(_ string, cached *CachedResponse) bool {
		if cached.QueriedDate == targetDate {
			found = true
			return false // stop on first match
		}
		return true
	})
	return found, err
}

// GetCacheDir returns the cache directory path for external use (e.g., error messages).
func GetCacheDir() string {
	return getCacheDir()
}
