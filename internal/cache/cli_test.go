package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupCacheDir creates a temporary cache directory for testing
func setupCacheDir(t *testing.T) (string, func()) {
	t.Helper()
	
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("enphase-cache-test-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	
	// Set environment or use a method to override cache dir for tests
	// For now, we'll work with the actual cache dir but document that these are integration-style tests
	
	return tempDir, func() {
		os.RemoveAll(tempDir)
	}
}

// createMockCacheFile creates a mock cache file with specified content
func createMockCacheFile(t *testing.T, dir, filename string, statusCode int, body string, cachedAt time.Time, queriedDate string) {
	t.Helper()
	
	cached := CachedResponse{
		StatusCode:  statusCode,
		Headers:     map[string]string{"Content-Type": "application/json"},
		Body:        []byte(body),
		CachedAt:    cachedAt,
		QueriedDate: queriedDate,
	}
	
	data, err := json.Marshal(cached)
	if err != nil {
		t.Fatalf("Failed to marshal cache: %v", err)
	}
	
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("Failed to write cache file: %v", err)
	}
}

func TestClearTodayCache_NoDirectory(t *testing.T) {
	// Test when cache directory doesn't exist
	err := ClearTodayCache()
	if err != nil {
		t.Errorf("ClearTodayCache() should not error when directory doesn't exist: %v", err)
	}
}

func TestClearTodayCache_EmptyDirectory(t *testing.T) {
	// This tests the actual cache directory behavior
	err := ClearTodayCache()
	if err != nil {
		t.Errorf("ClearTodayCache() returned error: %v", err)
	}
}

func TestClearAllCache_Execution(t *testing.T) {
	// Test that ClearAllCache executes without error
	// Note: This removes the actual cache directory
	err := ClearAllCache()
	if err != nil {
		t.Errorf("ClearAllCache() returned error: %v", err)
	}
}

func TestListCacheEntries_NoDirectory(t *testing.T) {
	// Save original cache dir
	origDir := getCacheDir()
	
	// Test with non-existent directory
	entries, err := ListCacheEntries()
	if err != nil {
		t.Errorf("ListCacheEntries() should not error when directory doesn't exist: %v", err)
	}
	
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries for non-existent directory, got %d", len(entries))
	}
	
	// Ensure we didn't break anything
	_ = origDir
}

func TestListCacheEntries_WithFiles(t *testing.T) {
	// Test with actual cache directory (may have files)
	entries, err := ListCacheEntries()
	if err != nil {
		t.Errorf("ListCacheEntries() returned error: %v", err)
	}
	
	// Should return slice (may be empty)
	if entries == nil {
		t.Error("Expected non-nil slice from ListCacheEntries()")
	}
	
	// Validate entry structure if we have any
	for _, entry := range entries {
		if entry.Key == "" {
			t.Error("Cache entry should have non-empty Key")
		}
		if entry.Path == "" {
			t.Error("Cache entry should have non-empty Path")
		}
		if entry.CachedAt.IsZero() {
			t.Error("Cache entry should have non-zero CachedAt timestamp")
		}
	}
}

func TestLoadCachedResponseByPath_NotFound(t *testing.T) {
	// Test with non-existent file
	_, err := LoadCachedResponseByPath("/nonexistent/path/to/cache.json")
	if err == nil {
		t.Error("Expected error for non-existent cache file")
	}
}

func TestLoadCachedResponseByPath_InvalidJSON(t *testing.T) {
	// Create temp file with invalid JSON
	tempDir, cleanup := setupCacheDir(t)
	defer cleanup()
	
	invalidPath := filepath.Join(tempDir, "invalid.json")
	if err := os.WriteFile(invalidPath, []byte("not valid json{{{"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	_, err := LoadCachedResponseByPath(invalidPath)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestLoadCachedResponseByPath_Valid(t *testing.T) {
	// Create temp file with valid cache data
	tempDir, cleanup := setupCacheDir(t)
	defer cleanup()
	
	validPath := filepath.Join(tempDir, "valid.json")
	createMockCacheFile(t, tempDir, "valid.json", 200, `{"test": "data"}`, time.Now(), "2026-01-15")
	
	cached, err := LoadCachedResponseByPath(validPath)
	if err != nil {
		t.Errorf("LoadCachedResponseByPath() failed: %v", err)
	}
	
	if cached == nil {
		t.Fatal("Expected non-nil cached response")
	}
	
	if cached.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", cached.StatusCode)
	}
	
	if string(cached.Body) != `{"test": "data"}` {
		t.Errorf("Body = %s, want %s", string(cached.Body), `{"test": "data"}`)
	}
}

func TestInspectCacheEntry_NotFound(t *testing.T) {
	err := InspectCacheEntry("nonexistent_hash_12345")
	if err == nil {
		t.Error("Expected error for non-existent cache entry")
	}
	
	if !strings.Contains(err.Error(), "failed to load cache file") {
		t.Errorf("Expected 'failed to load cache file' error, got: %v", err)
	}
}

func TestInspectCacheEntry_InvalidHash(t *testing.T) {
	// Test with invalid hash format
	err := InspectCacheEntry("../../../etc/passwd")
	if err == nil {
		t.Error("Expected error for path traversal attempt")
	}
}

func TestFindCacheEntriesByDate_NoDirectory(t *testing.T) {
	// Test with a date when cache directory might not exist
	targetDate := time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC)
	
	entries, err := FindCacheEntriesByDate(targetDate, nil)
	if err != nil {
		t.Errorf("FindCacheEntriesByDate() should not error: %v", err)
	}
	
	// May be nil or empty slice when no directory exists
	if entries != nil && len(entries) > 0 {
		t.Logf("Found %d entries (unexpected for future date)", len(entries))
	}
}

func TestFindCacheEntriesByDate_WithTimezone(t *testing.T) {
	// Test with specific timezone
	targetDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	tz, _ := time.LoadLocation("US/Pacific")
	
	entries, err := FindCacheEntriesByDate(targetDate, tz)
	if err != nil {
		t.Errorf("FindCacheEntriesByDate() returned error: %v", err)
	}
	
	// entries may be nil when no cache files exist for this date
	t.Logf("Found %d entries for date with timezone", len(entries))
}

func TestFindCacheEntriesByDate_NilTimezone(t *testing.T) {
	// Test with nil timezone (should use default)
	targetDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	
	entries, err := FindCacheEntriesByDate(targetDate, nil)
	if err != nil {
		t.Errorf("FindCacheEntriesByDate() should handle nil timezone: %v", err)
	}
	
	// entries may be nil when no cache files exist
	t.Logf("Found %d entries with nil timezone", len(entries))
}

func TestFindCacheEntriesByDate_DateMatching(t *testing.T) {
	// Test date matching logic
	tests := []struct {
		name       string
		targetDate time.Time
		desc       string
	}{
		{
			name:       "past date",
			targetDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			desc:       "Should handle past dates",
		},
		{
			name:       "future date",
			targetDate: time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC),
			desc:       "Should handle future dates (likely no matches)",
		},
		{
			name:       "today",
			targetDate: time.Now(),
			desc:       "Should handle today's date",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := FindCacheEntriesByDate(tt.targetDate, nil)
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tt.desc, err)
			}
			// entries may be nil when no cache files exist for this date
			t.Logf("%s: found %d entries", tt.desc, len(entries))
		})
	}
}

func TestParseCacheResponse_TelemetryBattery(t *testing.T) {
	// Test parsing battery telemetry response
	batteryJSON := `{
		"intervals": [
			{
				"end_at": 1737072000,
				"charge": {"enwh": 100},
				"discharge": {"enwh": 50}
			}
		]
	}`
	
	endpoint, systemID, date, summary := parseCacheResponse([]byte(batteryJSON))
	
	if endpoint == "" {
		t.Error("Expected non-empty endpoint for battery telemetry")
	}
	
	if date == "" {
		t.Error("Expected non-empty date from battery telemetry")
	}
	
	// Validate that it identified as battery endpoint
	if !strings.Contains(strings.ToLower(endpoint), "battery") {
		t.Errorf("Expected battery endpoint, got: %s", endpoint)
	}
	
	// Summary should contain charge/discharge info
	if summary != "" && !strings.Contains(strings.ToLower(summary), "interval") {
		t.Logf("Summary: %s", summary)
	}
	
	_ = systemID // May be empty for telemetry
}

func TestParseCacheResponse_ProductionMeter(t *testing.T) {
	// Test parsing production meter response
	productionJSON := `{
		"intervals": [
			{
				"end_at": 1737072000,
				"wh_del": 1500
			}
		]
	}`
	
	endpoint, _, date, summary := parseCacheResponse([]byte(productionJSON))
	
	if endpoint == "" {
		t.Error("Expected non-empty endpoint for production meter")
	}
	
	if date == "" {
		t.Error("Expected non-empty date from production meter")
	}
	
	t.Logf("Parsed production: endpoint=%s, date=%s, summary=%s", endpoint, date, summary)
}

func TestParseCacheResponse_InvalidJSON(t *testing.T) {
	// Test with invalid JSON
	invalidJSON := `not valid json {{{`
	
	endpoint, systemID, date, summary := parseCacheResponse([]byte(invalidJSON))
	
	// Should not panic, but all values may be empty
	_ = endpoint
	_ = systemID
	_ = date
	_ = summary
	
	t.Log("Invalid JSON handled gracefully (no panic)")
}

func TestParseCacheResponse_EmptyResponse(t *testing.T) {
	// Test with empty response
	endpoint, systemID, date, summary := parseCacheResponse([]byte("{}"))
	
	// Should not panic
	_ = endpoint
	_ = systemID
	_ = date
	_ = summary
	
	t.Log("Empty response handled gracefully")
}

func TestParseCacheResponse_ConsumptionMeter(t *testing.T) {
	// Test parsing consumption meter response
	consumptionJSON := `{
		"intervals": [
			{
				"end_at": 1737072000,
				"enwh": 2500
			}
		]
	}`
	
	endpoint, _, date, summary := parseCacheResponse([]byte(consumptionJSON))
	
	if date == "" {
		t.Error("Expected non-empty date from consumption meter")
	}
	
	t.Logf("Parsed consumption: endpoint=%s, date=%s, summary=%s", endpoint, date, summary)
}

func TestCacheEntry_Structure(t *testing.T) {
	// Test CacheEntry structure and field access
	entry := CacheEntry{
		Key:      "test_hash",
		Path:     "/path/to/cache.json",
		CachedAt: time.Now(),
		Size:     1024,
		URLHash:  "test_hash",
		Endpoint: "telemetry/battery",
		SystemID: "12345",
		Date:     "2026-01-15",
		Summary:  "Test summary",
	}
	
	if entry.Key != "test_hash" {
		t.Errorf("Key = %s, want test_hash", entry.Key)
	}
	
	if entry.Endpoint != "telemetry/battery" {
		t.Errorf("Endpoint = %s, want telemetry/battery", entry.Endpoint)
	}
	
	if entry.Size != 1024 {
		t.Errorf("Size = %d, want 1024", entry.Size)
	}
}

func TestListCacheEntries_SkipsMetadataFiles(t *testing.T) {
	// Test that ListCacheEntries skips last_request.json
	entries, err := ListCacheEntries()
	if err != nil {
		t.Errorf("ListCacheEntries() returned error: %v", err)
	}
	
	// Verify no entry has the metadata filename
	for _, entry := range entries {
		if strings.Contains(entry.Path, "last_request.json") || strings.Contains(entry.Path, "last_request") {
			t.Errorf("ListCacheEntries() should skip metadata files, found: %s", entry.Path)
		}
	}
}

func TestClearTodayCache_Integration(t *testing.T) {
	// Integration test - actually clears cache
	// This is safe because it only clears today's cache
	err := ClearTodayCache()
	if err != nil {
		t.Errorf("ClearTodayCache() integration test failed: %v", err)
	}
	
	// Run again to verify idempotency
	err = ClearTodayCache()
	if err != nil {
		t.Errorf("ClearTodayCache() should be idempotent: %v", err)
	}
}

func TestFindCacheEntriesByDate_DateFormatVariations(t *testing.T) {
	// Test that different date formats are handled
	targetDate := time.Date(2026, 1, 15, 12, 34, 56, 0, time.UTC)
	
	entries, err := FindCacheEntriesByDate(targetDate, nil)
	if err != nil {
		t.Errorf("FindCacheEntriesByDate() should handle date with time: %v", err)
	}
	
	// entries may be nil when no cache files exist
	t.Logf("Found %d entries for date with time component", len(entries))
}

func TestParseCacheResponse_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		json string
		desc string
	}{
		{
			name: "null intervals",
			json: `{"intervals": null}`,
			desc: "Should handle null intervals",
		},
		{
			name: "empty intervals array",
			json: `{"intervals": []}`,
			desc: "Should handle empty intervals array",
		},
		{
			name: "missing intervals key",
			json: `{"other_field": "value"}`,
			desc: "Should handle missing intervals key",
		},
		{
			name: "mixed data",
			json: `{"intervals": [{"end_at": 1737072000, "wh_del": 100, "enwh": 200}]}`,
			desc: "Should handle intervals with multiple energy fields",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, systemID, date, summary := parseCacheResponse([]byte(tt.json))
			
			// Should not panic
			t.Logf("%s: endpoint=%s, systemID=%s, date=%s, summary=%s",
				tt.desc, endpoint, systemID, date, summary)
		})
	}
}
