// api_cache.go provides a caching layer for API responses.
//
// PURPOSE
// -------
// Implements disk-based caching to reduce API calls and enable offline testing.
// Cache files are stored in test-data/cache/ with SHA256 hashed filenames.
//
// For details on the caching strategy and rate limiting, see:
//   - ARCHITECTURE.md: "Intelligent Caching" section
//   - README.md: "Caching and Rate Limits" section
//
// KEY FEATURES
// ------------
// The caching system:
//   - Stores API responses on disk for reuse
//   - Supports cache-only mode for testing (--test flag)
//   - Provides cache inspection and management tools (--inspect-cache)
//   - Handles past date queries with cache fallback
//   - Normalizes URLs for consistent cache keys (timestamps → dates)
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
	"sync"
	"time"

	"enphase-monitor/internal/constants"
)

// getCacheDir returns the cache directory path, resolving it relative to the project root.
// This ensures cache files are always stored in test-data/cache/ at the project root,
// regardless of where tests or the application are run from.
func getCacheDir() string {
	// Try to find the project root by looking for go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "test-data/cache" // fallback to relative path
	}
	
	// Walk up the directory tree to find go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "test-data", "cache")
		}
		
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root without finding go.mod, use relative path
			return "test-data/cache"
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

// MinRequestInterval is the minimum time between API requests (used for cache staleness)
const MinRequestInterval = 1 * time.Minute

// cacheState holds mutable cache configuration with thread-safe access.
// Uses sync.Mutex to protect concurrent access from multiple goroutines.
//
// GO PATTERN: Thread-Safe State with Mutex
// Instead of bare global variables, we encapsulate state in a struct with a mutex.
// This ensures safe concurrent access and provides a clean reset mechanism for testing.
type cacheState struct {
	mu                    sync.Mutex
	testMode              bool
	cacheDisabled         bool
	rateLimitWarningShown bool
}

// state is the package-level cache state, protected by mutex.
var state = &cacheState{}

// TestMode returns whether test mode is enabled (thread-safe).
func TestMode() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.testMode
}

// SetTestMode enables or disables test mode (thread-safe).
func SetTestMode(enabled bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.testMode = enabled
}

// CacheDisabled returns whether cache is disabled (thread-safe).
func CacheDisabled() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.cacheDisabled
}

// SetCacheDisabled enables or disables cache bypass (thread-safe).
func SetCacheDisabled(disabled bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.cacheDisabled = disabled
}

// RateLimitWarningShown returns whether a rate limit warning has been shown (thread-safe).
func RateLimitWarningShown() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.rateLimitWarningShown
}

// SetRateLimitWarningShown sets the rate limit warning flag (thread-safe).
func SetRateLimitWarningShown(shown bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.rateLimitWarningShown = shown
}

// ResetState resets all cache state flags to their default values (thread-safe).
// This is primarily used for testing to ensure clean state between tests.
func ResetState() {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.testMode = false
	state.cacheDisabled = false
	state.rateLimitWarningShown = false
}

// CachedResponse stores a cached API response
type CachedResponse struct {
	StatusCode  int               `json:"status_code"`
	Headers     map[string]string `json:"headers"`
	Body        []byte            `json:"body"`
	CachedAt    time.Time         `json:"cached_at"`
	QueriedDate string            `json:"queried_date,omitempty"` // The date that was queried (YYYY-MM-DD), not the API's start_date
}

// toHTTPResponse reconstructs an *http.Response from a cached response.
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
// Also normalizes date strings to ensure consistent cache keys
// Example: ...start_at=1735717600&end_at=1737504000 -> ...start_at=2026-01-20&end_at=2026-01-20
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

	// For date strings, ensure both start_date and end_date use the same date
	// (since we are querying for a single day, both should be the same)
	if startDate := query.Get("start_date"); startDate != "" {
		if endDate := query.Get("end_date"); endDate != "" && startDate != endDate {
			// If dates differ, use start_date for both (the day we are querying)
			query.Set("end_date", startDate)
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

	cached := CachedResponse{
		StatusCode:  resp.StatusCode,
		Headers:     headers,
		Body:        bodyBytes,
		CachedAt:    time.Now(),
		QueriedDate: queriedDate,
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
