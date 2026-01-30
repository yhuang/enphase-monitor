// Package main - cache_cli.go
//
// PURPOSE
// -------
// This file implements CLI commands for cache inspection and management.
// Provides tools to list, inspect, and clear cached API responses.
//
// CACHE COMMANDS
// --------------
//   - --list-cache: List all cached responses with metadata
//   - --inspect-cache <hash|date>: View detailed cache entry or all entries for a date
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
	"enphase-monitor/internal/constants"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClearTodayCache clears only cache files for today's date
// It identifies today's cache by checking:
// 1. The CachedAt timestamp (must be today)
// 2. The file modification time (must be today)
// This ensures we only delete cache files that were created/modified today,
// preserving cache files from yesterday or earlier
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
		if cachedDate == todayStr {
			// Delete this cache file (it is for today)
			if err := os.Remove(cachePath); err == nil {
				deletedCount++
			}
		} else {
			skippedCount++
		}
	}

	if deletedCount > 0 {
		fmt.Printf("Cleared %d cache file(s) for today (%s)\n", deletedCount, todayStr)
		if skippedCount > 0 {
			fmt.Printf("Preserved %d cache file(s) from yesterday or earlier\n", skippedCount)
		}
	} else {
		fmt.Printf("No cache files found for today (%s)\n", todayStr)
		if skippedCount > 0 {
			fmt.Printf("Found %d cache file(s) from other dates (preserved)\n", skippedCount)
		}
	}

	return nil
}

// ClearAllCache clears all cached API responses
func ClearAllCache() error {
	return os.RemoveAll(getCacheDir())
}

// CacheEntry represents a cached API response with metadata
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

// ListCacheEntries returns all cached API responses with their metadata
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

		// Skip metadata files like last_request.json
		if entry.Name() == "last_request.json" || entry.Name() == "last_request" {
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

// LoadCachedResponseByPath loads a cached response from a specific file path
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

// InspectCacheEntry displays detailed information about a cached response
func InspectCacheEntry(hash string) error {
	cachePath := filepath.Join(getCacheDir(), hash+constants.JSONExtension)
	cached, err := LoadCachedResponseByPath(cachePath)
	if err != nil {
		return fmt.Errorf("failed to load cache file: %w", err)
	}

	endpoint, systemID, apiStartDate, summary := parseCacheResponse(cached.Body)

	fmt.Printf("Cache Entry: %s\n", hash)
	fmt.Printf("Path: %s\n", cachePath)
	if endpoint != "" {
		fmt.Printf("Endpoint: %s\n", endpoint)
	}
	if systemID != "" {
		fmt.Printf("System ID: %s\n", systemID)
	}
	// Show both the queried date and the API's start_date
	if cached.QueriedDate != "" {
		fmt.Printf("Queried Date: %s (date requested in API call via --date parameter)\n", cached.QueriedDate)
	} else {
		fmt.Printf("Queried Date: (not stored - this is an old cache entry)\n")
	}
	if apiStartDate != "" {
		fmt.Printf("API start_date: %s (system installation date)\n", apiStartDate)
	}
	if summary != "" {
		fmt.Printf("Summary: %s\n", summary)
	}
	fmt.Printf("Cached At: %s\n", cached.CachedAt.Format(time.RFC3339))
	fmt.Printf("Status Code: %d\n", cached.StatusCode)
	fmt.Printf("Body Size: %d bytes\n", len(cached.Body))
	fmt.Printf("\nHeaders:\n")
	for k, v := range cached.Headers {
		fmt.Printf("  %s: %s\n", k, v)
	}

	// Try to parse and pretty-print the JSON body
	var jsonData interface{}
	if err := json.Unmarshal(cached.Body, &jsonData); err == nil {
		fmt.Printf("\nBody (JSON):\n")
		prettyJSON, err := json.MarshalIndent(jsonData, "  ", "  ")
		if err == nil {
			fmt.Printf("%s\n", string(prettyJSON))
		} else {
			fmt.Printf("  (failed to format JSON: %v)\n", err)
			fmt.Printf("  Raw body: %s\n", string(cached.Body))
		}
	} else {
		fmt.Printf("\nBody (raw):\n")
		fmt.Printf("%s\n", string(cached.Body))
	}

	return nil
}

// FindCacheEntriesByDate finds cache entries that match a specific date
// If tz is nil, defaults to US/Pacific for cache operations (cache is system-agnostic)
func FindCacheEntriesByDate(targetDate time.Time, tz *time.Location) ([]CacheEntry, error) {
	allEntries, err := ListCacheEntries()
	if err != nil {
		return nil, err
	}

	// Use provided timezone or default to Pacific
	if tz == nil {
		tz, _ = time.LoadLocation("US/Pacific")
	}

	// Format the target date directly without timezone conversion
	// The dates in cache entries are already in YYYY-MM-DD format and do not need timezone adjustment
	targetDateStr := targetDate.Format(constants.DateFormat)

	var matchingEntries []CacheEntry
	for _, entry := range allEntries {
		// Use the Date field from the parsed entry (which comes from the response body)
		// If Date is not set, fall back to checking CachedAt timestamp
		if entry.Date != "" {
			// Normalize both dates for comparison (remove any whitespace)
			entryDate := strings.TrimSpace(entry.Date)
			// Also try to parse and reformat the date in case the format differs
			if parsedDate, err := time.Parse(constants.DateFormat, entryDate); err == nil {
				entryDate = parsedDate.Format(constants.DateFormat)
			} else {
				// Try other common date formats
				if parsedDate, err := time.Parse(constants.AltDateFormat, entryDate); err == nil {
					entryDate = parsedDate.Format(constants.DateFormat)
				}
			}
			// Direct string comparison should work if both are in DateFormat
			if entryDate == targetDateStr {
				matchingEntries = append(matchingEntries, entry)
			}
		} else {
			// Fallback: check cached timestamp if Date was not parsed from response
			cached, err := LoadCachedResponseByPath(entry.Path)
			if err != nil {
				continue
			}
			cachedDateStr := cached.CachedAt.In(tz).Format(constants.DateFormat)
			if cachedDateStr == targetDateStr {
				matchingEntries = append(matchingEntries, entry)
			}
		}
	}

	return matchingEntries, nil
}

// parseCacheResponse attempts to parse the cached response body and extract useful information
// Returns: endpoint type, system ID, date, and summary
func parseCacheResponse(body []byte) (endpoint, systemID, date, summary string) {
	// Try to parse as TelemetryResponse first (new approach - telemetry endpoints)
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
	if err := json.Unmarshal(body, &telemetryResp); err == nil && len(telemetryResp.Intervals) > 0 {
		// Extract date from first interval's timestamp
		// Use default timezone for cache parsing (cache is system-agnostic)
		firstTime := time.Unix(telemetryResp.Intervals[0].EndAt, 0)
		defaultTZ, _ := time.LoadLocation("US/Pacific")
		firstTime = firstTime.In(defaultTZ)
		date = firstTime.Format(constants.DateFormat)

		// Determine endpoint type based on which fields are populated
		// Battery has charge/discharge, others have WhDlvd/WhRcvd/Enwh
		hasCharge := false
		hasWhDel := false
		hasWhRcv := false
		hasEnwh := false
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
			endpoint = "telemetry/battery"
			var totalCharge, totalDischarge float64
			for _, interval := range telemetryResp.Intervals {
				totalCharge += interval.Charge.Enwh
				totalDischarge += interval.Discharge.Enwh
			}
			summary = fmt.Sprintf("Battery: %d intervals, charge=%.2f, discharge=%.2f kWh",
				len(telemetryResp.Intervals), totalCharge/constants.WhToKWh, totalDischarge/constants.WhToKWh)
		} else if hasWhDel && !hasWhRcv {
			endpoint = "energy_import_telemetry"
			var totalImport float64
			for _, interval := range telemetryResp.Intervals {
				totalImport += interval.WhDel
			}
			summary = fmt.Sprintf("Import: %d intervals, total=%.2f kWh",
				len(telemetryResp.Intervals), totalImport/constants.WhToKWh)
		} else if hasWhRcv && !hasWhDel {
			endpoint = "energy_export_telemetry"
			var totalExport float64
			for _, interval := range telemetryResp.Intervals {
				totalExport += interval.WhRcv
			}
			summary = fmt.Sprintf("Export: %d intervals, total=%.2f kWh",
				len(telemetryResp.Intervals), totalExport/constants.WhToKWh)
		} else if hasEnwh {
			// Could be production_meter or consumption_meter - check URL in cache or use Enwh
			endpoint = "telemetry/production_meter" // Default assumption
			var totalEnwh float64
			for _, interval := range telemetryResp.Intervals {
				totalEnwh += interval.Enwh
			}
			summary = fmt.Sprintf("Production/Consumption: %d intervals, total=%.2f kWh",
				len(telemetryResp.Intervals), totalEnwh/constants.WhToKWh)
		}
		return
	}

	// If we cannot parse it, return empty strings
	return "", "", "", ""
}
