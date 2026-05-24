// Package cache - cli.go
//
// PURPOSE
// -------
// This file implements CLI commands for cache management and entry listing.
//
// CACHE COMMANDS
// --------------
//   - --clear-cache: Clear today's cache only (preserves historical)
//   - --clear-all-cache: Clear all cached responses
//
// CACHE ENTRY STRUCTURE
// ---------------------
// Each cache file contains:
//   - status_code: HTTP status code
//   - headers: HTTP response headers
//   - body: Raw API response (JSON)
//   - cached_at: Timestamp when cached
//   - queried_date: Date that was queried (YYYY-MM-DD)
//
// CACHE FILE NAMING
// -----------------
// Files are named by SHA256 hash of normalized URL (includes query params).
// Format: {hash}.json where {hash} is 64-character hexadecimal string.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"enphase-monitor/internal/constants"
)

// ClearTodayCache clears only cache files for today's date.
// It identifies today's cache by checking the CachedAt timestamp and file
// modification time, preserving cache files from yesterday or earlier.
func ClearTodayCache() error {
	today := time.Now()
	todayStr := today.Format(constants.DateFormat)

	// Check if cache directory exists
	if _, err := os.Stat(getCacheDir()); os.IsNotExist(err) {
		fmt.Printf("No cache directory found\n")
		return nil
	}

	entries, err := os.ReadDir(getCacheDir())
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	var deletedCount int
	var skippedCount int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .json files
		if filepath.Ext(entry.Name()) != constants.JSONExtension {
			continue
		}

		cachePath := filepath.Join(getCacheDir(), entry.Name())

		// Check file modification time first (faster than reading the file)
		fileInfo, err := os.Stat(cachePath)
		if err != nil {
			continue
		}
		fileModDate := fileInfo.ModTime().Format(constants.DateFormat)

		// If file was not modified today, skip it (preserve past dates)
		if fileModDate != todayStr {
			skippedCount++
			continue
		}

		// Read the cache file to check CachedAt timestamp
		data, err := os.ReadFile(cachePath)
		if err != nil {
			continue
		}

		var cached CachedResponse
		if err := json.Unmarshal(data, &cached); err != nil {
			continue
		}

		// Check if cached today
		if cached.CachedAt.IsZero() {
			continue
		}

		cachedDate := cached.CachedAt.Format(constants.DateFormat)

		// Only delete if both cached today AND file modified today
		// This ensures we are deleting cache for today, not past dates that were accessed today
		if cachedDate != todayStr {
			skippedCount++
			continue
		}
		if err := os.Remove(cachePath); err == nil {
			deletedCount++
		}
	}

	if deletedCount > 0 {
		fmt.Printf("Cleared %d cache file(s) for today (%s)\n", deletedCount, todayStr)
		if skippedCount > 0 {
			fmt.Printf("Preserved %d cache file(s) from yesterday or earlier\n", skippedCount)
		}
		return nil
	}
	fmt.Printf("No cache files found for today (%s)\n", todayStr)
	if skippedCount > 0 {
		fmt.Printf("Found %d cache file(s) from other dates (preserved)\n", skippedCount)
	}

	return nil
}

// ClearAllCache clears all cached API responses.
func ClearAllCache() error {
	return os.RemoveAll(getCacheDir())
}

// CacheEntry represents a cached API response with metadata.
//
//nolint:revive // exported name clarifies package (cache.CacheEntry)
type CacheEntry struct {
	Key      string
	Path     string
	CachedAt time.Time
	Size     int64
	URLHash  string
	Endpoint string // e.g., "telemetry/battery", "energy_import_telemetry", "production_meter_readings"
	SystemID string // System ID from response
	Date     string // Date from response or cached timestamp
	Summary  string // Summary of data (array lengths, values, etc.)
}

// ListCacheEntries returns all cached API responses with their metadata.
func ListCacheEntries() ([]CacheEntry, error) {
	if _, err := os.Stat(getCacheDir()); os.IsNotExist(err) {
		return []CacheEntry{}, nil
	}

	entries, err := os.ReadDir(getCacheDir())
	if err != nil {
		return nil, err
	}

	var cacheEntries []CacheEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Skip metadata files like last_request.json and the sliding-window
		// API-call log used by the API Budget counter.
		if entry.Name() == "last_request.json" || entry.Name() == "last_request" || entry.Name() == apiCallsFilename {
			continue
		}

		// Extract hash from filename (remove .json extension)
		hash := strings.TrimSuffix(entry.Name(), constants.JSONExtension)

		cachePath := filepath.Join(getCacheDir(), entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Try to load the cached response to get timestamp and parse response
		cached, err := LoadCachedResponseByPath(cachePath)
		if err != nil {
			// If we cannot load it, still include it but with minimal info
			cacheEntries = append(cacheEntries, CacheEntry{
				Key:      hash,
				Path:     cachePath,
				Size:     info.Size(),
				URLHash:  hash,
				CachedAt: info.ModTime(),
			})
			continue
		}

		// Try to parse the response body to extract useful information
		endpoint, systemID, _, summary := parseCacheResponse(cached.Body)

		// Use QueriedDate if available (the date we actually queried), otherwise fall back to API's start_date
		date := cached.QueriedDate
		if date == "" {
			// Fallback: try to extract from response body (API's start_date)
			// Note: This is the system installation date, not the queried date
			_, _, date, _ = parseCacheResponse(cached.Body)
		}

		entry := CacheEntry{
			Key:      hash,
			Path:     cachePath,
			CachedAt: cached.CachedAt,
			Size:     info.Size(),
			URLHash:  hash,
			Endpoint: endpoint,
			SystemID: systemID,
			Date:     date,
			Summary:  summary,
		}

		cacheEntries = append(cacheEntries, entry)
	}

	return cacheEntries, nil
}

// LoadCachedResponseByPath loads a cached response from a specific file path.
func LoadCachedResponseByPath(cachePath string) (*CachedResponse, error) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	var cached CachedResponse
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}

	return &cached, nil
}

// parseCacheResponse attempts to parse the cached response body and extract useful information
// Returns: endpoint type, system ID, date, and summary
func parseCacheResponse(body []byte) (endpoint, systemID, date, summary string) {
	var telemetryResp struct {
		Intervals []struct {
			EndAt  int64   `json:"end_at"`
			WhDel  float64 `json:"wh_del"`
			WhRcv  float64 `json:"wh_rcv"`
			Enwh   float64 `json:"enwh"`
			Charge struct {
				Enwh float64 `json:"enwh"`
			} `json:"charge"`
			Discharge struct {
				Enwh float64 `json:"enwh"`
			} `json:"discharge"`
		} `json:"intervals"`
	}
	if err := json.Unmarshal(body, &telemetryResp); err != nil || len(telemetryResp.Intervals) == 0 {
		return "", "", "", ""
	}

	// Extract date from first interval's timestamp
	firstTime := time.Unix(telemetryResp.Intervals[0].EndAt, 0)
	defaultTZ, err := time.LoadLocation("US/Pacific")
	if err != nil {
		Debugf("failed to load US/Pacific timezone; cache entry will show no date in --cache output: %v", err)
	} else {
		date = firstTime.In(defaultTZ).Format(constants.DateFormat)
	}

	// Determine endpoint type from which fields are populated (go-style-core: flat with early return)
	hasCharge, hasWhDel, hasWhRcv, hasEnwh := false, false, false, false
	for _, interval := range telemetryResp.Intervals {
		if interval.Charge.Enwh > 0 || interval.Discharge.Enwh > 0 {
			hasCharge = true
		}
		if interval.WhDel > 0 {
			hasWhDel = true
		}
		if interval.WhRcv > 0 {
			hasWhRcv = true
		}
		if interval.Enwh > 0 {
			hasEnwh = true
		}
	}

	if hasCharge {
		var totalCharge, totalDischarge float64
		for _, interval := range telemetryResp.Intervals {
			totalCharge += interval.Charge.Enwh
			totalDischarge += interval.Discharge.Enwh
		}
		return "telemetry/battery", "", date,
			fmt.Sprintf("Battery: %d intervals, charge=%.2f, discharge=%.2f kWh",
				len(telemetryResp.Intervals), totalCharge/constants.WhToKWh, totalDischarge/constants.WhToKWh)
	}
	if hasWhDel && !hasWhRcv {
		var totalImport float64
		for _, interval := range telemetryResp.Intervals {
			totalImport += interval.WhDel
		}
		return "energy_import_telemetry", "", date,
			fmt.Sprintf("Import: %d intervals, total=%.2f kWh",
				len(telemetryResp.Intervals), totalImport/constants.WhToKWh)
	}
	if hasWhRcv && !hasWhDel {
		var totalExport float64
		for _, interval := range telemetryResp.Intervals {
			totalExport += interval.WhRcv
		}
		return "energy_export_telemetry", "", date,
			fmt.Sprintf("Export: %d intervals, total=%.2f kWh",
				len(telemetryResp.Intervals), totalExport/constants.WhToKWh)
	}
	if hasEnwh {
		var totalEnwh float64
		for _, interval := range telemetryResp.Intervals {
			totalEnwh += interval.Enwh
		}
		return "telemetry/production_meter", "", date,
			fmt.Sprintf("Production/Consumption: %d intervals, total=%.2f kWh",
				len(telemetryResp.Intervals), totalEnwh/constants.WhToKWh)
	}
	return "", "", date, ""
}
