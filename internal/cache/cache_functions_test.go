// Package cache - cache_functions_test.go
//
// TEST SETUP
// ----------
// This test suite validates core caching functionality:
// - URL normalization
// - Cache key generation (SHA-256 hashing)
// - Cache file saving and loading
// - Cache entry listing and inspection
// - Cache existence validation for test mode
//
// TEST PLAN
// ---------
// 1. URL Normalization Tests
//   - Test timestamp conversion to dates (for consistent caching)
//   - Test URL encoding (spaces, special characters)
//   - Test query parameter ordering
//
// 2. Cache Key Generation Tests
//   - Test SHA-256 hash generation
//   - Test same URL produces same key
//   - Test different URLs produce different keys
//
// 3. Cache Save/Load Tests
//   - Test saving response to disk
//   - Test loading response from disk
//   - Test cache file format (JSON)
//   - Test missing cache file returns error
//
// 4. Cache Management Tests
//   - Test listing all cache entries
//   - Test cache inspection by hash
//   - Test cache inspection by date
//   - Test URL redaction (hide sensitive data)
//
// 5. Cache Existence Tests (for --test mode validation)
//   - Test HasCacheForDate returns false for non-existent dates
//   - Test HasCacheForDate returns false for empty cache directory
//   - Test GetCacheDir returns valid path
//
// TESTING APPROACH
// ----------------
// - Create temporary cache directory for tests
// - Clean up cache files after tests (defer)
// - Use test-specific URLs to avoid conflicts
// - Verify cache files are valid JSON
//
// WHY SEPARATE FILE
// -----------------
// This package has 3 test files (1:many pattern):
// - cache_test.go: State management tests
// - cache_functions_test.go (this file): Functionality tests
// - cli_test.go: CLI utilities tests
//
// PATTERN USED
// ------------
// - Pattern 1: Table-Driven Tests
// - Pattern 3: Subtests with t.Run()
//
// See TESTING.md for detailed pattern explanations.
package cache

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRedactURLKey(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "URL with key parameter",
			url:      "https://api.example.com/endpoint?key=secret123&other=value",
			expected: "https://api.example.com/endpoint?key=REDACTED&other=value",
		},
		{
			name:     "URL without key parameter",
			url:      "https://api.example.com/endpoint?other=value",
			expected: "https://api.example.com/endpoint?other=value",
		},
		{
			name:     "Invalid URL",
			url:      "://invalid",
			expected: "://invalid",
		},
		{
			name:     "Empty URL",
			url:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactURLKey(tt.url)
			if result != tt.expected {
				t.Errorf("RedactURLKey() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetCacheKey(t *testing.T) {
	tz := time.UTC

	url1 := "https://api.example.com/endpoint?key=test123"
	url2 := "https://api.example.com/endpoint?key=test456"

	key1 := GetCacheKey(url1, tz)
	key2 := GetCacheKey(url2, tz)

	// Keys should be 64 character hex strings (SHA256)
	if len(key1) != 64 {
		t.Errorf("GetCacheKey() returned key of length %d, want 64", len(key1))
	}

	// Same URL should produce same key
	key1Again := GetCacheKey(url1, tz)
	if key1 != key1Again {
		t.Error("GetCacheKey() should return same key for same URL")
	}

	// Different URLs should produce different keys
	if key1 == key2 {
		t.Error("GetCacheKey() should return different keys for different URLs")
	}
}

func TestNormalizeURLForCache(t *testing.T) {
	tz := time.UTC

	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "URL with timestamps",
			url:      "https://api.example.com/endpoint?key=test&start_at=1768896000&end_at=1768982400",
			expected: "https://api.example.com/endpoint?end_date=2026-01-21&key=test&start_date=2026-01-20",
		},
		{
			name:     "URL with date strings",
			url:      "https://api.example.com/endpoint?key=test&start_date=2026-01-20&end_date=2026-01-20",
			expected: "https://api.example.com/endpoint?end_date=2026-01-20&key=test&start_date=2026-01-20",
		},
		{
			name:     "URL with mismatched dates",
			url:      "https://api.example.com/endpoint?start_date=2026-01-20&end_date=2026-01-21",
			expected: "https://api.example.com/endpoint?end_date=2026-01-21&start_date=2026-01-20",
		},
		{
			name:     "URL without date parameters",
			url:      "https://api.example.com/endpoint?key=test",
			expected: "https://api.example.com/endpoint?key=test",
		},
		{
			name:     "Invalid URL",
			url:      "://invalid",
			expected: "://invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeURLForCache(tt.url, tz)
			if result != tt.expected {
				t.Errorf("NormalizeURLForCache() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractQueriedDateFromURL(t *testing.T) {
	tz := time.UTC

	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "URL with start_at timestamp",
			url:      "https://api.example.com/endpoint?start_at=1768896000",
			expected: "2026-01-20",
		},
		{
			name:     "URL with start_date string",
			url:      "https://api.example.com/endpoint?start_date=2026-01-15",
			expected: "2026-01-15",
		},
		{
			name:     "URL without date parameters",
			url:      "https://api.example.com/endpoint?key=test",
			expected: "",
		},
		{
			name:     "Invalid URL",
			url:      "://invalid",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractQueriedDateFromURL(tt.url, tz)
			if result != tt.expected {
				t.Errorf("extractQueriedDateFromURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetCachePath(t *testing.T) {
	tz := time.UTC
	url := "https://api.example.com/endpoint?key=test123"

	path := GetCachePath(url, tz)

	// Path should end with cache/<hash>.json
	if !strings.Contains(path, "cache") {
		t.Errorf("GetCachePath() = %v, should contain 'cache'", path)
	}

	// Path should end with .json
	if !strings.HasSuffix(path, ".json") {
		t.Errorf("GetCachePath() = %v, should end with .json", path)
	}

	// Same URL should produce same path
	pathAgain := GetCachePath(url, tz)
	if path != pathAgain {
		t.Error("GetCachePath() should return same path for same URL")
	}
}

func TestToHTTPResponse(t *testing.T) {
	cached := &CachedResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"X-Custom":     "test-value",
		},
		Body:     []byte(`{"test": "data"}`),
		CachedAt: time.Now(),
	}

	resp := cached.ToHTTPResponse()

	if resp.StatusCode != 200 {
		t.Errorf("ToHTTPResponse() StatusCode = %v, want 200", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("ToHTTPResponse() Content-Type = %v, want application/json", resp.Header.Get("Content-Type"))
	}

	if resp.Header.Get("X-Custom") != "test-value" {
		t.Errorf("ToHTTPResponse() X-Custom = %v, want test-value", resp.Header.Get("X-Custom"))
	}

	if resp.Body == nil {
		t.Error("ToHTTPResponse() Body should not be nil")
	}
}

func TestSaveAndLoadCachedResponse(t *testing.T) {
	useTempCacheDir(t)

	tz := time.UTC
	testURL := "https://api.example.com/test?key=testkey&start_at=1768896000"

	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	bodyBytes := []byte(`{"test": "data"}`)

	if err := SaveCachedResponseFromBytes(testURL, resp, bodyBytes, tz); err != nil {
		t.Fatalf("SaveCachedResponseFromBytes() error = %v", err)
	}

	cached, err := LoadCachedResponse(testURL, tz)
	if err != nil {
		t.Fatalf("LoadCachedResponse() error = %v", err)
	}

	if cached.StatusCode != 200 {
		t.Errorf("LoadCachedResponse() StatusCode = %v, want 200", cached.StatusCode)
	}
	if string(cached.Body) != `{"test": "data"}` {
		t.Errorf("LoadCachedResponse() Body = %v, want {\"test\": \"data\"}", string(cached.Body))
	}
	if cached.QueriedDate != "2026-01-20" {
		t.Errorf("LoadCachedResponse() QueriedDate = %v, want 2026-01-20", cached.QueriedDate)
	}
}

func TestLoadCachedResponse_NotFound(t *testing.T) {
	tz := time.UTC
	// Use a URL that definitely doesn't have a cache file
	testURL := "https://api.example.com/nonexistent?key=test&start_at=9999999999"

	_, err := LoadCachedResponse(testURL, tz)
	if err == nil {
		t.Error("LoadCachedResponse() should return error for non-existent cache")
	}

	if !strings.Contains(err.Error(), "cache file does not exist") {
		t.Errorf("LoadCachedResponse() error = %v, should contain 'cache file does not exist'", err)
	}
}

func TestSaveCachedResponse_CreatesDirectory(t *testing.T) {
	useTempCacheDir(t)

	tz := time.UTC
	testURL := "https://api.example.com/createdir?key=test&start_at=1768896000"

	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{},
	}
	bodyBytes := []byte(`{"test": "directory creation"}`)

	if err := SaveCachedResponseFromBytes(testURL, resp, bodyBytes, tz); err != nil {
		t.Fatalf("SaveCachedResponseFromBytes() error = %v", err)
	}

	cacheDir := getCacheDir()
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Errorf("SaveCachedResponseFromBytes() should create cache directory %s", cacheDir)
	}
}

func TestGetCacheDir(t *testing.T) {
	dir := getCacheDir()

	// Should end with "cache"
	if !strings.HasSuffix(dir, "cache") {
		t.Errorf("getCacheDir() = %v, should end with 'cache'", dir)
	}

	// Should be an absolute path (starts with /)
	if !filepath.IsAbs(dir) {
		t.Errorf("getCacheDir() = %v, should be an absolute path", dir)
	}
}

// TestGetCacheDirExported tests the exported GetCacheDir wrapper.
func TestGetCacheDirExported(t *testing.T) {
	dir := GetCacheDir()
	if dir == "" {
		t.Error("GetCacheDir() returned empty string")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("GetCacheDir() = %v, should be an absolute path", dir)
	}
}

func TestNormalizeURLForCache_TimestampConversion(t *testing.T) {
	// Test that timestamps are correctly converted to dates in different timezones
	// Using 1768867200 = 2026-01-20 00:00:00 UTC = 2026-01-19 16:00:00 PST
	url := "https://api.example.com/endpoint?start_at=1768867200&end_at=1768953600"

	// UTC
	utc := time.UTC
	normalizedUTC := NormalizeURLForCache(url, utc)
	if !strings.Contains(normalizedUTC, "start_date=2026-01-20") {
		t.Errorf("NormalizeURLForCache() with UTC = %v, should contain start_date=2026-01-20", normalizedUTC)
	}

	// Pacific Time
	pacific, _ := time.LoadLocation("America/Los_Angeles")
	normalizedPacific := NormalizeURLForCache(url, pacific)
	// In Pacific timezone, this timestamp converts to 2026-01-19 (previous day)
	if !strings.Contains(normalizedPacific, "start_date=2026-01-19") {
		t.Errorf("NormalizeURLForCache() with Pacific = %v, should contain start_date=2026-01-19", normalizedPacific)
	}
}

func TestCachedResponse_EmptyHeaders(t *testing.T) {
	cached := &CachedResponse{
		StatusCode: 404,
		Headers:    map[string]string{},
		Body:       []byte("Not Found"),
		CachedAt:   time.Now(),
	}

	resp := cached.ToHTTPResponse()

	if resp.StatusCode != 404 {
		t.Errorf("ToHTTPResponse() StatusCode = %v, want 404", resp.StatusCode)
	}

	if resp.Header == nil {
		t.Error("ToHTTPResponse() Header should not be nil even with empty headers")
	}
}

// TestHasCacheForDate tests the cache existence check for a specific date.
// This function is used to validate that --test mode has data available before running.
func TestHasCacheForDate(t *testing.T) {
	useTempCacheDir(t)

	t.Run("returns false when no cache exists for distant future date", func(t *testing.T) {
		hasCache, err := HasCacheForDate("2099-12-31")
		if err != nil {
			t.Errorf("HasCacheForDate() returned error: %v", err)
		}
		if hasCache {
			t.Error("HasCacheForDate() should return false for non-existent date")
		}
	})

	t.Run("returns false when no matching date in cache files", func(t *testing.T) {
		hasCache, err := HasCacheForDate("1999-01-01")
		if err != nil {
			t.Errorf("HasCacheForDate() returned error: %v", err)
		}
		if hasCache {
			t.Error("HasCacheForDate() should return false for date with no cache")
		}
	})
}

func TestClearCacheForDate(t *testing.T) {
	useTempCacheDir(t)
	tz := time.UTC
	resp := &http.Response{StatusCode: 200, Header: http.Header{}}

	// start_at=1768809600 -> 2026-01-19, start_at=1768896000 -> 2026-01-20
	jan19 := "https://api.example.com/a?key=k&start_at=1768809600"
	jan20 := "https://api.example.com/b?key=k&start_at=1768896000"
	for _, u := range []string{jan19, jan20} {
		if err := SaveCachedResponseFromBytes(u, resp, []byte(`{}`), tz); err != nil {
			t.Fatalf("SaveCachedResponseFromBytes() error = %v", err)
		}
	}

	if err := ClearCacheForDate("2026-01-19"); err != nil {
		t.Fatalf("ClearCacheForDate() error = %v", err)
	}

	// The cleared date should be gone; the untouched date should remain.
	if has, _ := HasCacheForDate("2026-01-19"); has {
		t.Error("expected cache for 2026-01-19 to be cleared")
	}
	if has, _ := HasCacheForDate("2026-01-20"); !has {
		t.Error("expected cache for 2026-01-20 to be preserved")
	}
}

func TestClearCacheForDate_NoMatch(t *testing.T) {
	useTempCacheDir(t)
	// No cache files exist for this date; should not error.
	if err := ClearCacheForDate("2026-01-19"); err != nil {
		t.Errorf("ClearCacheForDate() error = %v", err)
	}
}

func TestClearCacheForDate_EmptyDate(t *testing.T) {
	useTempCacheDir(t)
	if err := ClearCacheForDate(""); err == nil {
		t.Error("ClearCacheForDate(\"\") = nil, want error for empty date")
	}
}

// TestClearCacheForDate_AggregateSemantics documents that matching is on the
// exact queried_date: clearing a mid-period day leaves an aggregate that starts
// on a different day intact, while clearing the aggregate's start day removes it.
func TestClearCacheForDate_AggregateSemantics(t *testing.T) {
	useTempCacheDir(t)
	tz := time.UTC
	resp := &http.Response{StatusCode: 200, Header: http.Header{}}

	// A month aggregate caches under the period's first day (start_date); a day
	// query caches under its exact date.
	monthAgg := "https://api.example.com/energy_lifetime?key=k&start_date=2026-03-01"
	midMonthDay := "https://api.example.com/telemetry?key=k&start_date=2026-03-15"
	for _, u := range []string{monthAgg, midMonthDay} {
		if err := SaveCachedResponseFromBytes(u, resp, []byte(`{}`), tz); err != nil {
			t.Fatalf("SaveCachedResponseFromBytes() error = %v", err)
		}
	}

	// Clearing a mid-period day must NOT remove the month aggregate.
	if err := ClearCacheForDate("2026-03-15"); err != nil {
		t.Fatalf("ClearCacheForDate() error = %v", err)
	}
	if has, _ := HasCacheForDate("2026-03-15"); has {
		t.Error("expected day cache for 2026-03-15 to be cleared")
	}
	if has, _ := HasCacheForDate("2026-03-01"); !has {
		t.Error("month aggregate (2026-03-01) should survive clearing a mid-period day")
	}

	// Clearing the aggregate's start day removes it.
	if err := ClearCacheForDate("2026-03-01"); err != nil {
		t.Fatalf("ClearCacheForDate() error = %v", err)
	}
	if has, _ := HasCacheForDate("2026-03-01"); has {
		t.Error("expected month aggregate (2026-03-01) to be cleared")
	}
}

// TestClearCacheForDate_RemoveFailureReturnsError verifies that a matched file
// which cannot be deleted surfaces an error rather than reporting success.
func TestClearCacheForDate_RemoveFailureReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission test is not meaningful on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	useTempCacheDir(t)
	tz := time.UTC
	resp := &http.Response{StatusCode: 200, Header: http.Header{}}
	if err := SaveCachedResponseFromBytes("https://api.example.com/a?key=k&start_date=2026-01-19", resp, []byte(`{}`), tz); err != nil {
		t.Fatalf("SaveCachedResponseFromBytes() error = %v", err)
	}

	cacheDir := getCacheDir()
	// Read-only directory: entries can still be listed and read, but files
	// inside cannot be removed.
	if err := os.Chmod(cacheDir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	// Restore writability before t.TempDir's own cleanup runs.
	t.Cleanup(func() { _ = os.Chmod(cacheDir, 0o700) })

	if err := ClearCacheForDate("2026-01-19"); err == nil {
		t.Error("ClearCacheForDate() = nil, want error when a matching file cannot be removed")
	}
}
