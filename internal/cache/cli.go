// cli.go implements cache management commands (--clear-cache, --clear-cache-date,
// --clear-all-cache) and cache entry listing used by the --cache diagnostic path.
package cache

import (
	"encoding/json"
	"errors"
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

// ClearCacheForDate clears cache files whose queried_date exactly matches the
// given date (YYYY-MM-DD). Unlike ClearTodayCache, it identifies entries by the
// date the data was queried for rather than when the file was cached, so it can
// target a specific past date regardless of when its cache was written.
//
// Matching is on the exact queried_date, which is the query's start date. Day
// queries store that day; Month / Year / True-Up aggregates store the period's
// first day (e.g. a March monthly total is "2026-03-01"). Consequently clearing
// a mid-period day does not invalidate an aggregate that spans it, and clearing
// a period's first day also removes that aggregate's cache. It returns an error
// if any matching file could not be deleted.
func ClearCacheForDate(targetDate string) error {
	if targetDate == "" {
		return errors.New("targetDate must not be empty")
	}

	var matched, deleted int
	var firstRemoveErr error
	err := forEachCacheEntry(func(path string, cached *CachedResponse) bool {
		if cached.QueriedDate != targetDate {
			return true
		}
		matched++
		if rmErr := os.Remove(path); rmErr != nil {
			if firstRemoveErr == nil {
				firstRemoveErr = rmErr
			}
			return true
		}
		deleted++
		return true
	})
	if err != nil {
		return err
	}

	if matched == 0 {
		fmt.Printf("No cache files found for %s\n", targetDate)
		return nil
	}
	if deleted < matched {
		return fmt.Errorf("cleared %d of %d matching cache file(s); %d could not be deleted: %w",
			deleted, matched, matched-deleted, firstRemoveErr)
	}
	fmt.Printf("Cleared %d cache file(s) for %s\n", deleted, targetDate)
	return nil
}

// ClearAllCache clears all cached API responses.
func ClearAllCache() error {
	return os.RemoveAll(getCacheDir())
}

// Entry represents a cached API response with metadata.
type Entry struct {
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
func ListCacheEntries() ([]Entry, error) {
	if _, err := os.Stat(getCacheDir()); os.IsNotExist(err) {
		return []Entry{}, nil
	}

	entries, err := os.ReadDir(getCacheDir())
	if err != nil {
		return nil, err
	}

	var cacheEntries []Entry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Skip metadata files and the per-credential quota ledger.
		if entry.Name() == "last_request.json" || entry.Name() == "last_request" || entry.Name() == "monthly-quota.json" || entry.Name() == "quota.json" {
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
			cacheEntries = append(cacheEntries, Entry{
				Key:      hash,
				Path:     cachePath,
				Size:     info.Size(),
				URLHash:  hash,
				CachedAt: info.ModTime(),
			})
			continue
		}

		// Use authoritative metadata from the cached response; fall back to body-parsed
		// endpoint for old cache entries that predate the Endpoint/SystemID fields.
		parsedEndpoint, summary := parseCacheResponse(cached.Body)
		endpoint := cached.Endpoint
		if endpoint == "" {
			endpoint = parsedEndpoint
		}
		systemID := cached.SystemID

		// Use QueriedDate if available; fall back to CachedAt date for pre-metadata entries.
		date := cached.QueriedDate
		if date == "" && !cached.CachedAt.IsZero() {
			date = cached.CachedAt.Format(constants.DateFormat)
		}

		entry := Entry{
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

// parseCacheResponse attempts to parse the cached response body and extract a human-readable
// summary. It also identifies the endpoint type as a fallback for old cache entries that
// predate the Endpoint metadata field. Returns empty strings if the body cannot be parsed.
func parseCacheResponse(body []byte) (endpoint, summary string) {
	var telemetryResp struct {
		Intervals []struct {
			EndAt      int64   `json:"end_at"`
			WhImported float64 `json:"wh_imported"`
			WhExported float64 `json:"wh_exported"`
			WhDel      float64 `json:"wh_del"`
			Enwh       float64 `json:"enwh"`
			Charge     struct {
				Enwh float64 `json:"enwh"`
			} `json:"charge"`
			Discharge struct {
				Enwh float64 `json:"enwh"`
			} `json:"discharge"`
		} `json:"intervals"`
	}
	if err := json.Unmarshal(body, &telemetryResp); err != nil || len(telemetryResp.Intervals) == 0 {
		return "", ""
	}

	var hasCharge, hasWhImported, hasWhExported, hasWhDel, hasEnwh bool
	for _, interval := range telemetryResp.Intervals {
		if interval.Charge.Enwh > 0 || interval.Discharge.Enwh > 0 {
			hasCharge = true
		}
		if interval.WhImported > 0 {
			hasWhImported = true
		}
		if interval.WhExported > 0 {
			hasWhExported = true
		}
		if interval.WhDel > 0 {
			hasWhDel = true
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
		return "telemetry/battery",
			fmt.Sprintf("Battery: %d intervals, charge=%.2f, discharge=%.2f kWh",
				len(telemetryResp.Intervals), totalCharge/constants.WhToKWh, totalDischarge/constants.WhToKWh)
	}
	if hasWhImported {
		var total float64
		for _, interval := range telemetryResp.Intervals {
			total += interval.WhImported
		}
		return "energy_import_telemetry",
			fmt.Sprintf("Import: %d intervals, total=%.2f kWh",
				len(telemetryResp.Intervals), total/constants.WhToKWh)
	}
	if hasWhExported {
		var total float64
		for _, interval := range telemetryResp.Intervals {
			total += interval.WhExported
		}
		return "energy_export_telemetry",
			fmt.Sprintf("Export: %d intervals, total=%.2f kWh",
				len(telemetryResp.Intervals), total/constants.WhToKWh)
	}
	if hasWhDel {
		var total float64
		for _, interval := range telemetryResp.Intervals {
			total += interval.WhDel
		}
		return "telemetry/production_meter",
			fmt.Sprintf("Production: %d intervals, total=%.2f kWh",
				len(telemetryResp.Intervals), total/constants.WhToKWh)
	}
	if hasEnwh {
		var total float64
		for _, interval := range telemetryResp.Intervals {
			total += interval.Enwh
		}
		return "telemetry/consumption_meter",
			fmt.Sprintf("Consumption: %d intervals, total=%.2f kWh",
				len(telemetryResp.Intervals), total/constants.WhToKWh)
	}
	return "", ""
}
