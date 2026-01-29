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
package main

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
	"time"
)

const cacheDir = "test-data/cache"

// redactURLKey strips the "key" query parameter from a URL for safe inclusion in error messages.
func redactURLKey(rawURL string) string {
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

const minRequestInterval = 1 * time.Minute // Minimum time between API requests (used for cache staleness)

// testMode forces cache-only mode (no live API calls)
var testMode = false

// SetTestMode enables or disables test mode
func SetTestMode(enabled bool) {
	testMode = enabled
}

// cacheDisabled forces live API calls (skip cache lookup)
var cacheDisabled = false

// SetCacheDisabled enables or disables cache bypass
func SetCacheDisabled(disabled bool) {
	cacheDisabled = disabled
}

// rateLimitWarningShown tracks if we've already warned about 429 fallback
var rateLimitWarningShown = false

// CachedResponse stores a cached API response
type CachedResponse struct {
	StatusCode  int               `json:"status_code"`
	Headers     map[string]string `json:"headers"`
	Body        []byte            `json:"body"`
	CachedAt    time.Time         `json:"cached_at"`
	QueriedDate string            `json:"queried_date,omitempty"` // The date that was queried (YYYY-MM-DD), not the API's start_date
}

// toHTTPResponse reconstructs an *http.Response from a cached response.
func (c *CachedResponse) toHTTPResponse() *http.Response {
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

// getCacheKey generates a cache key from a URL
// Normalizes timestamps in the URL to use date strings for consistent caching
func getCacheKey(url string, tz *time.Location) string {
	// Normalize the URL by replacing Unix timestamps with date strings
	// This allows the same endpoint/date to use the same cache key
	normalizedURL := normalizeURLForCache(url, tz)
	hash := sha256.Sum256([]byte(normalizedURL))
	return hex.EncodeToString(hash[:])
}

// normalizeURLForCache normalizes a URL by replacing timestamps with date strings
// Also normalizes date strings to ensure consistent cache keys
// Example: ...start_at=1735717600&end_at=1737504000 -> ...start_at=2026-01-20&end_at=2026-01-20
// Example: ...start_date=2026-01-20&end_date=2026-01-20 -> ...start_date=2026-01-20&end_date=2026-01-20 (no change)
func normalizeURLForCache(urlStr string, tz *time.Location) string {
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
			dateStr := t.Format("2006-01-02")
			query.Del("start_at")
			query.Set("start_date", dateStr)
		}
	}
	if endAt := query.Get("end_at"); endAt != "" {
		if timestamp, err := strconv.ParseInt(endAt, 10, 64); err == nil {
			t := time.Unix(timestamp, 0).In(tz)
			dateStr := t.Format("2006-01-02")
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
			return t.Format("2006-01-02")
		}
	}

	// Check for start_date string
	if startDate := query.Get("start_date"); startDate != "" {
		return startDate
	}

	return ""
}

// getCachePath returns the file path for a cached response
func getCachePath(url string, tz *time.Location) string {
	key := getCacheKey(url, tz)
	return filepath.Join(cacheDir, key+".json")
}

// loadCachedResponse loads a cached response from disk
func loadCachedResponse(url string, tz *time.Location) (*CachedResponse, error) {
	cachePath := getCachePath(url, tz)

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

// saveCachedResponseFromBytes saves a response to the cache using pre-read body bytes
func saveCachedResponseFromBytes(url string, resp *http.Response, bodyBytes []byte, tz *time.Location) error {
	// Create cache directory if it does not exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
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
	cachePath := getCachePath(url, tz)
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}
