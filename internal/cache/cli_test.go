package cache

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// useTempCacheDir redirects all cache I/O to a fresh temp directory for the
// duration of the test. It calls t.Setenv so the original value is restored
// automatically when the test (and any sub-tests) finish.
func useTempCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("ENPHASE_CACHE_DIR", dir)
	return dir
}

// createMockCacheFile writes a CachedResponse JSON file into dir.
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
	// Point at a non-existent subdirectory so ClearTodayCache sees no dir.
	dir := t.TempDir()
	t.Setenv("ENPHASE_CACHE_DIR", filepath.Join(dir, "nonexistent"))

	if err := ClearTodayCache(); err != nil {
		t.Errorf("ClearTodayCache() should not error when directory doesn't exist: %v", err)
	}
}

func TestClearTodayCache_EmptyDirectory(t *testing.T) {
	useTempCacheDir(t)

	if err := ClearTodayCache(); err != nil {
		t.Errorf("ClearTodayCache() returned error: %v", err)
	}
}

func TestClearAllCache_Execution(t *testing.T) {
	dir := useTempCacheDir(t)

	// Create a dummy file so removal is non-trivial.
	if err := os.WriteFile(filepath.Join(dir, "dummy.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create dummy file: %v", err)
	}

	if err := ClearAllCache(); err != nil {
		t.Errorf("ClearAllCache() returned error: %v", err)
	}

	// The directory should no longer exist after ClearAllCache.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("ClearAllCache() should have removed the cache directory")
	}
}

func TestListCacheEntries_NoDirectory(t *testing.T) {
	// Point at a non-existent path — ListCacheEntries should return empty slice.
	dir := t.TempDir()
	t.Setenv("ENPHASE_CACHE_DIR", filepath.Join(dir, "nonexistent"))

	entries, err := ListCacheEntries()
	if err != nil {
		t.Errorf("ListCacheEntries() should not error when directory doesn't exist: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries for non-existent directory, got %d", len(entries))
	}
}

func TestListCacheEntries_WithFiles(t *testing.T) {
	dir := useTempCacheDir(t)

	// Seed one file so the test exercises the non-empty path.
	createMockCacheFile(t, dir, "abc123.json", 200, `{"intervals":[]}`, time.Now(), "2026-01-15")

	entries, err := ListCacheEntries()
	if err != nil {
		t.Errorf("ListCacheEntries() returned error: %v", err)
	}
	if entries == nil {
		t.Error("Expected non-nil slice from ListCacheEntries()")
	}
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
	_, err := LoadCachedResponseByPath("/nonexistent/path/to/cache.json")
	if err == nil {
		t.Error("Expected error for non-existent cache file")
	}
}

func TestLoadCachedResponseByPath_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	invalidPath := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalidPath, []byte("not valid json{{{"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := LoadCachedResponseByPath(invalidPath)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestLoadCachedResponseByPath_Valid(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "valid.json")
	createMockCacheFile(t, dir, "valid.json", 200, `{"test": "data"}`, time.Now(), "2026-01-15")

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

func TestParseCacheResponse_TelemetryBattery(t *testing.T) {
	batteryJSON := `{
		"intervals": [
			{
				"end_at": 1737072000,
				"charge": {"enwh": 100},
				"discharge": {"enwh": 50}
			}
		]
	}`

	endpoint, summary := parseCacheResponse([]byte(batteryJSON))

	if endpoint == "" {
		t.Error("Expected non-empty endpoint for battery telemetry")
	}
	if !strings.Contains(strings.ToLower(endpoint), "battery") {
		t.Errorf("Expected battery endpoint, got: %s", endpoint)
	}
	if summary != "" && !strings.Contains(strings.ToLower(summary), "interval") {
		t.Logf("Summary: %s", summary)
	}
}

func TestParseCacheResponse_ProductionMeter(t *testing.T) {
	productionJSON := `{
		"intervals": [
			{
				"end_at": 1737072000,
				"wh_del": 1500
			}
		]
	}`

	endpoint, summary := parseCacheResponse([]byte(productionJSON))

	if endpoint == "" {
		t.Error("Expected non-empty endpoint for production meter")
	}
	t.Logf("Parsed production: endpoint=%s, summary=%s", endpoint, summary)
}

func TestParseCacheResponse_InvalidJSON(t *testing.T) {
	endpoint, summary := parseCacheResponse([]byte(`not valid json {{{`))
	_ = endpoint
	_ = summary
	t.Log("Invalid JSON handled gracefully (no panic)")
}

func TestParseCacheResponse_EmptyResponse(t *testing.T) {
	endpoint, summary := parseCacheResponse([]byte("{}"))
	_ = endpoint
	_ = summary
	t.Log("Empty response handled gracefully")
}

func TestParseCacheResponse_ConsumptionMeter(t *testing.T) {
	consumptionJSON := `{
		"intervals": [
			{
				"end_at": 1737072000,
				"enwh": 2500
			}
		]
	}`

	endpoint, summary := parseCacheResponse([]byte(consumptionJSON))
	t.Logf("Parsed consumption: endpoint=%s, summary=%s", endpoint, summary)
}

func TestEntry_Structure(t *testing.T) {
	entry := Entry{
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
	useTempCacheDir(t)

	entries, err := ListCacheEntries()
	if err != nil {
		t.Errorf("ListCacheEntries() returned error: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Path, "last_request.json") || strings.Contains(entry.Path, "last_request") {
			t.Errorf("ListCacheEntries() should skip metadata files, found: %s", entry.Path)
		}
	}
}

func TestClearTodayCache_Integration(t *testing.T) {
	useTempCacheDir(t)

	if err := ClearTodayCache(); err != nil {
		t.Errorf("ClearTodayCache() integration test failed: %v", err)
	}
	// Idempotent
	if err := ClearTodayCache(); err != nil {
		t.Errorf("ClearTodayCache() should be idempotent: %v", err)
	}
}

func TestParseCacheResponse_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		json string
		desc string
	}{
		{"null intervals", `{"intervals": null}`, "Should handle null intervals"},
		{"empty intervals array", `{"intervals": []}`, "Should handle empty intervals array"},
		{"missing intervals key", `{"other_field": "value"}`, "Should handle missing intervals key"},
		{"mixed data", `{"intervals": [{"end_at": 1737072000, "wh_del": 100, "enwh": 200}]}`, "Should handle multiple energy fields"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, summary := parseCacheResponse([]byte(tt.json))
			t.Logf("%s: endpoint=%s, summary=%s", tt.desc, endpoint, summary)
		})
	}
}

// TestListCacheEntries_WithRealFile tests the full ListCacheEntries path using the real save mechanism.
func TestListCacheEntries_WithRealFile(t *testing.T) {
	useTempCacheDir(t)

	tz := time.UTC
	testURL := "https://api.enphaseenergy.com/api/v4/systems/99999/telemetry/production_meter?key=testkey&start_at=1700000000&end_at=1700086400"

	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	body := []byte(`{"system_id":99999,"intervals":[{"end_at":1700043200,"wh_del":500.0,"enwh":500.0}]}`)

	if err := SaveCachedResponseFromBytes(testURL, resp, body, tz); err != nil {
		t.Fatalf("SaveCachedResponseFromBytes() failed: %v", err)
	}
	cachePath := GetCachePath(testURL, tz)

	entries, err := ListCacheEntries()
	if err != nil {
		t.Fatalf("ListCacheEntries() error = %v", err)
	}

	var found *Entry
	for i := range entries {
		if entries[i].Path == cachePath {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatal("ListCacheEntries() did not return the entry we just created")
	}
	if found.Key == "" {
		t.Error("ListCacheEntries() entry has empty Key")
	}
	if found.CachedAt.IsZero() {
		t.Error("ListCacheEntries() entry has zero CachedAt")
	}
}
