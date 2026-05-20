// Package cli provides command-line flag parsing and cache management for the enphase-monitor application.
//
// CACHE COMMANDS
// --------------
// The application supports several cache management commands:
//   - --clear-cache: Clear today's cached responses only
//   - --clear-all-cache: Clear all cached responses (all dates)
//
// Flag parsing lives in flags.go; command handlers are in this file to keep main clean and focused.
package cli

import (
	"fmt"

	"enphase-monitor/internal/cache"
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

