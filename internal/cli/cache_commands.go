// Package cli provides command-line flag parsing and cache management for the enphase-monitor application.
//
// CACHE COMMANDS
// --------------
// The application supports several cache management commands:
//   - --clear-cache: Clear today's cached responses only
//   - --clear-cache-date: Clear cached responses for a specific date (YYYY-MM-DD)
//   - --clear-all-cache: Clear all cached responses (all dates)
//
// Flag parsing lives in flags.go; command handlers are in this file to keep main clean and focused.
package cli

import (
	"fmt"
	"time"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
)

// HandleClearCache clears cached API responses for today's date only.
func HandleClearCache() error {
	if err := cache.ClearTodayCache(); err != nil {
		return fmt.Errorf("failed to clear today's cache: %w", err)
	}
	return nil
}

// HandleClearCacheForDate clears cached API responses for the given date.
// The date must be a valid YYYY-MM-DD value; future dates are rejected since
// no cache can exist for them.
func HandleClearCacheForDate(date string) error {
	if _, err := time.Parse(constants.DateFormat, date); err != nil {
		return fmt.Errorf("invalid date %q: expected YYYY-MM-DD", date)
	}
	// Compare as YYYY-MM-DD strings (which sort lexically) against the local
	// "today", matching how ClearTodayCache defines today. Avoids the timezone
	// skew of comparing a UTC-midnight time.Time against time.Now().
	if date > time.Now().Format(constants.DateFormat) {
		return fmt.Errorf("invalid date %q: date is in the future", date)
	}
	if err := cache.ClearCacheForDate(date); err != nil {
		return fmt.Errorf("failed to clear cache for %s: %w", date, err)
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
