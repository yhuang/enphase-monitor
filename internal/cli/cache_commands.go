// Package cli provides command-line flag parsing and cache management for the enphase-monitor application.
//
// CACHE COMMANDS
// --------------
// The application supports several cache management commands:
//   - --clear-cache: Clear today's cached responses only
//   - --clear-all-cache: Clear all cached responses (all dates)
//   - --list-cache: List all cached responses with metadata
//   - --inspect-cache <hash|date>: Inspect specific cached responses
//
// Flag parsing lives in flags.go; command handlers are in this file to keep main clean and focused.
package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/timezone"
)

// HandleClearCache clears cached API responses for today's date only.
func HandleClearCache() error {
	if err := cache.ClearTodayCache(); err != nil {
		return fmt.Errorf("failed to clear today's cache: %w", err)
	}
	return nil
}

// HandleClearAllCache clears all cached API responses (all dates).
func HandleClearAllCache() error {
	if err := cache.ClearAllCache(); err != nil {
		return fmt.Errorf("failed to clear all cache: %w", err)
	}
	fmt.Println("All cache cleared successfully")
	return nil
}

// HandleListCache lists all cached API responses.
func HandleListCache() error {
	entries, err := cache.ListCacheEntries()
	if err != nil {
		return fmt.Errorf("failed to list cache entries: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No cached responses found")
		return nil
	}

	fmt.Printf("Found %d cached responses:\n\n", len(entries))
	for i, entry := range entries {
		fmt.Printf("[%d] Hash: %s\n", i+1, entry.Key)
		if entry.Endpoint != "" {
			fmt.Printf("    Endpoint: %s\n", entry.Endpoint)
		}
		if entry.SystemID != "" {
			fmt.Printf("    System ID: %s\n", entry.SystemID)
		}
		if entry.Date != "" {
			fmt.Printf("    Date: %s\n", entry.Date)
		}
		if entry.Summary != "" {
			fmt.Printf("    %s\n", entry.Summary)
		}
		fmt.Printf("    Cached At: %s\n", entry.CachedAt.Format(constants.TimestampFormat))
		fmt.Printf("    Size: %d bytes\n", entry.Size)
		fmt.Println()
	}
	fmt.Println("Use --inspect-cache <hash> to view details of a specific cache entry")
	return nil
}

// HandleInspectCache inspects cached responses by hash or date.
func HandleInspectCache(inspectValue, configFile string) error {
	// Check if it is a date (YYYY-MM-DD format) or a hash
	if date, err := time.Parse(constants.DateFormat, inspectValue); err == nil {
		// It is a date - show all cache entries for this date
		return handleInspectCacheByDate(date, inspectValue, configFile)
	}

	// It is a hash - show single entry
	if err := cache.InspectCacheEntry(inspectValue); err != nil {
		return fmt.Errorf("failed to inspect cache entry: %w", err)
	}
	return nil
}

// handleInspectCacheByDate inspects all cache entries for a specific date
func handleInspectCacheByDate(date time.Time, dateStr, configFile string) error {
	// Timezone is best-effort for display only: system default, then config if loadable.
	// We ignore LoadTimezone errors so inspect works even with invalid/missing timezone in config.
	tz, _ := timezone.LoadTimezone("")
	if cfg, err := config.LoadConfig(configFile); err == nil {
		tz, _ = timezone.LoadTimezone(cfg.Timezone)
	}

	entries, err := cache.FindCacheEntriesByDate(date, tz)
	if err != nil {
		return fmt.Errorf("failed to find cache entries: %w", err)
	}

	if len(entries) == 0 {
		fmt.Printf("No cached responses found for date %s\n", dateStr)
		showAvailableDates()
		return nil
	}

	fmt.Printf("Found %d cached responses for %s:\n\n", len(entries), dateStr)
	for i, entry := range entries {
		separator := strings.Repeat("=", 71)
		fmt.Printf(separator + "\n")
		fmt.Printf("Entry %d of %d\n", i+1, len(entries))
		fmt.Printf(separator + "\n")
		if err := cache.InspectCacheEntry(entry.Key); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to inspect cache entry %s: %v\n", entry.Key, err)
			continue
		}
		if i < len(entries)-1 {
			fmt.Println() // Add blank line between entries
		}
	}
	return nil
}

// showAvailableDates displays available dates in the cache
func showAvailableDates() {
	allEntries, err := cache.ListCacheEntries()
	if err != nil || len(allEntries) == 0 {
		return
	}

	// Collect unique dates (capacity hint: at most len(allEntries) unique dates)
	dateSet := make(map[string]bool, len(allEntries))
	for _, entry := range allEntries {
		if entry.Date != "" {
			dateSet[entry.Date] = true
		}
	}

	if len(dateSet) > 0 {
		dates := make([]string, 0, len(dateSet))
		for d := range dateSet {
			dates = append(dates, d)
		}
		fmt.Printf("\nAvailable dates in cache: %s\n", strings.Join(dates, ", "))
	}
}
