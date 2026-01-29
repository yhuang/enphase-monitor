// Package main implements the Enphase Monitor application.
//
// OVERVIEW
// --------
// This application monitors energy metrics from one or more Enphase solar systems
// via the Enphase Enlighten Cloud API v4. It aggregates data from multiple systems
// and displays a comprehensive energy report in a formatted terminal output.
//
// DOCUMENTATION
// -------------
// For comprehensive guides, see:
//   - README.md: User guide with installation and usage instructions
//   - QUICKSTART.md: 5-minute setup guide for new users
//   - ARCHITECTURE.md: Detailed architecture and execution flow
//   - OAUTH_SETUP.md: OAuth authentication setup guide
//   - GO_BEST_PRACTICES.md: Go patterns and idioms used in this codebase
//   - GO_CONCEPTS.md: Go language reference for concepts used here
//
// KEY FEATURES
// ------------
//   - Cloud API Integration: Uses Enphase Enlighten Cloud API v4 exclusively
//   - OAuth 2.0 Authentication: Secure token-based authentication with automatic refresh
//   - Multi-System Aggregation: Combines metrics from multiple independent systems
//   - Intelligent Caching: Disk-based caching to respect API rate limits (10 calls/minute)
//   - Historical Data: Query any past date with --date flag
//   - Color Customization: Customize terminal output colors via config.yaml
//   - Validation Mode: Test against expected values without making API calls
//
// ARCHITECTURE
// ------------
// The application follows a clear separation of concerns:
//
//	Entry Point & Control Flow:
//	- main.go: CLI flag parsing, execution modes (once/continuous/test), signal handling
//
//	Configuration & Authentication:
//	- config.go: YAML configuration loading, validation, color conversion
//	- oauth.go: OAuth token acquisition and refresh (with in-memory caching)
//	- setup_oauth.go: Interactive OAuth setup wizard for first-time configuration
//
//	Data Collection:
//	- cloud_client.go: HTTP client for Enlighten Cloud API v4, handles all API requests
//	- aggregator.go: Orchestrates data collection from multiple systems, combines metrics
//	- api_cache.go: Disk-based response caching, cache utilities
//	- cache_cli.go: Cache management CLI commands
//
//	Data Processing:
//	- response_parser.go: JSON parsing utilities for telemetry responses
//	- timezone.go: Timezone utilities for date boundary calculations
//	- url_builder.go: Helper for constructing API URLs with proper date ranges
//
//	Presentation:
//	- display.go: Terminal output formatting, ANSI color codes, report structure
//	- constants.go: ANSI escape code constants (Reset, Bold)
//
//	Testing:
//	- validation.go: Compares actual metrics against expected values with tolerance
//
// EXECUTION FLOW
// --------------
//  1. Parse CLI flags (--once, --date, --test, etc.)
//  2. Load and validate config.yaml
//  3. Create DataAggregator instance
//  4. For each system in config:
//     a. Get OAuth access token (cached or refreshed)
//     b. Create EnlightenCloudClient for the system
//     c. Fetch metrics via Cloud API (with caching)
//  5. Aggregate metrics from all systems
//  6. Display formatted report
//
// For continuous mode, steps 4-6 repeat at the configured refresh interval.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	// Command-line flags
	configFile := flag.String("config", "config.yaml", "Path to configuration file")
	once := flag.Bool("once", false, "Run once and exit (do not loop)")
	setupOAuth := flag.Bool("setup-oauth", false, "Run OAuth setup wizard (one-time for developer plan)")
	clearCache := flag.Bool("clear-cache", false, "Clear cached API responses for today's date only (preserves yesterday and earlier)")
	clearAllCache := flag.Bool("clear-all-cache", false, "Clear all cached API responses (all dates)")
	testDate := flag.String("date", "", "Query a specific date (YYYY-MM-DD format, e.g. 2026-01-19). Defaults to today if not specified.")
	testMode := flag.Bool("test", false, "Test mode: use cache only, no live API calls, validate against expected values")
	noCache := flag.Bool("no-cache", false, "Bypass cache and always make live API calls")
	listCache := flag.Bool("list-cache", false, "List all cached API responses")
	inspectCache := flag.String("inspect-cache", "", "Inspect cached responses by hash or date (YYYY-MM-DD format). Use --list-cache to see hashes.")
	flag.Parse()

	// Helper function for exiting with error message
	exitWithError := func(msg string, args ...interface{}) {
		fmt.Fprintf(os.Stderr, msg, args...)
		os.Exit(1)
	}

	// Handle clear cache for today only
	if *clearCache {
		if err := ClearTodayCache(); err != nil {
			exitWithError("Failed to clear today's cache: %v\n", err)
		}
		return
	}

	// Handle clear all cache
	if *clearAllCache {
		if err := ClearAllCache(); err != nil {
			exitWithError("Failed to clear all cache: %v\n", err)
		}
		fmt.Println("All cache cleared successfully")
		return
	}

	// Handle list cache
	if *listCache {
		entries, err := ListCacheEntries()
		if err != nil {
			exitWithError("Failed to list cache entries: %v\n", err)
		}
		if len(entries) == 0 {
			fmt.Println("No cached responses found")
			return
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
			fmt.Printf("    Cached At: %s\n", entry.CachedAt.Format(TimestampFormat))
			fmt.Printf("    Size: %d bytes\n", entry.Size)
			fmt.Println()
		}
		fmt.Println("Use --inspect-cache <hash> to view details of a specific cache entry")
		return
	}

	// Handle inspect cache by hash or date
	if *inspectCache != "" {
		// Check if it is a date (YYYY-MM-DD format) or a hash
		if date, err := time.Parse(DateFormat, *inspectCache); err == nil {
			// It is a date - show all cache entries for this date
			// Load config to get timezone, but use default if config not available
			var tz *time.Location
			if cfg, err := LoadConfig(*configFile); err == nil {
				tz, _ = LoadTimezone(cfg.Timezone)
			} else {
				tz, _ = LoadTimezone("")
			}
			entries, err := FindCacheEntriesByDate(date, tz)
			if err != nil {
				exitWithError("Failed to find cache entries: %v\n", err)
			}
			if len(entries) == 0 {
				fmt.Printf("No cached responses found for date %s\n", *inspectCache)
				// Show available dates
				allEntries, err := ListCacheEntries()
				if err == nil && len(allEntries) > 0 {
					// Collect unique dates
					dateSet := make(map[string]bool)
					for _, entry := range allEntries {
						if entry.Date != "" {
							dateSet[entry.Date] = true
						}
					}
					if len(dateSet) > 0 {
						var dates []string
						for d := range dateSet {
							dates = append(dates, d)
						}
						fmt.Printf("\nAvailable dates in cache: %s\n", strings.Join(dates, ", "))
					}
				}
				return
			}
			fmt.Printf("Found %d cached responses for %s:\n\n", len(entries), *inspectCache)
			for i, entry := range entries {
				separator := strings.Repeat("=", 71)
				fmt.Printf(separator + "\n")
				fmt.Printf("Entry %d of %d\n", i+1, len(entries))
				fmt.Printf(separator + "\n")
				if err := InspectCacheEntry(entry.Key); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to inspect cache entry %s: %v\n", entry.Key, err)
					continue
				}
				if i < len(entries)-1 {
					fmt.Println() // Add blank line between entries
				}
			}
		} else {
			// It is a hash - show single entry
			if err := InspectCacheEntry(*inspectCache); err != nil {
				exitWithError("Failed to inspect cache entry: %v\n", err)
			}
		}
		return
	}

	// Load configuration
	config, err := LoadConfig(*configFile)
	if err != nil {
		exitWithError("Failed to load configuration: %v\n\nPlease copy config.yaml.example to config.yaml and fill in your details.\n", err)
	}

	// Handle OAuth setup
	if *setupOAuth {
		if err := SetupOAuth(config); err != nil {
			exitWithError("OAuth setup failed: %v\n", err)
		}
		return
	}

	// Create aggregator
	aggregator := NewDataAggregator()

	// Load timezone for reporting/display (from config, system, or US/Pacific fallback)
	reportTZ, err := LoadTimezone(config.Timezone)
	if err != nil {
		// Should not happen, but use OS system timezone as fallback
		reportTZ = time.Now().Location()
	}

	// Create display with colors and timezone from config (or defaults)
	colors := getDefaultColors()
	if config.Colors != nil {
		colors = *config.Colors
	}
	display := NewDisplayWithColorsAndTimezone(colors, reportTZ)

	// Setup signal handling for graceful shutdown using context cancellation.
	// signal.NotifyContext returns a context that is cancelled when SIGINT or SIGTERM
	// is received. This allows in-flight HTTP requests to be cancelled immediately.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Enable test mode if requested
	if *testMode {
		SetTestMode(true)
		fmt.Println("TEST MODE: Using cache only, no live API calls")
	}

	// Enable cache bypass if requested
	if *noCache {
		SetCacheDisabled(true)
		fmt.Println("NO-CACHE MODE: Bypassing cache, making live API calls")
	}

	// Parse test date - default to today if not specified
	// reportTZ is already loaded above for display, reuse it for date parsing
	var testDateParsed time.Time
	if *testDate != "" {
		// Parse date using the reporting timezone
		parsed, err := ParseDateInTimezone(*testDate, reportTZ)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Invalid date format. Use YYYY-MM-DD (e.g. 2026-01-19): %v\n", err)
			os.Exit(1)
		}
		testDateParsed = parsed
	}
	// else: testDateParsed remains zero value (today)

	// Run once or in loop
	if *once {
		runOnce(ctx, aggregator, display, config, testDateParsed, *testMode, reportTZ)
	} else {
		runContinuous(ctx, aggregator, display, config, testDateParsed, reportTZ)
	}
}

func runOnce(ctx context.Context, aggregator *DataAggregator, display *Display, config *Config, testDate time.Time, testMode bool, reportTZ *time.Location) {
	metrics, err := aggregator.GetAggregatedMetrics(ctx, config.Systems, config.API, testDate, reportTZ)
	if err != nil {
		// Check if it is a 429 rate limit error - if so, the error message already contains wait time info
		if isRateLimitError(err) {
			// Error message already printed in aggregator.go, just exit
			os.Exit(1)
		}
		display.ShowError(err)
		os.Exit(1)
	}

	display.ShowMetrics(metrics)

	// If in test mode and test date is provided, validate against expected values
	if testMode {
		if testDate.IsZero() {
			fmt.Fprintf(os.Stderr, "ERROR: --test flag requires --date flag to specify which date to validate\n")
			os.Exit(1)
		}
		testDateStr := testDate.Format(DateFormat)
		if err := ValidateMetrics(metrics, testDateStr); err != nil {
			fmt.Fprintf(os.Stderr, "\nValidation failed: %v\n", err)
			os.Exit(1)
		}
	}
}

func runContinuous(ctx context.Context, aggregator *DataAggregator, display *Display, config *Config, testDate time.Time, reportTZ *time.Location) {
	display.ShowInfo(fmt.Sprintf("Starting continuous monitoring (refresh every %d seconds)", config.RefreshInterval))
	display.ShowInfo("Press Ctrl+C to stop")

	ticker := time.NewTicker(time.Duration(config.RefreshInterval) * time.Second)
	defer ticker.Stop()

	// Run immediately on start
	fetchAndDisplay(ctx, aggregator, display, config, testDate, reportTZ)

	for {
		select {
		case <-ticker.C:
			fetchAndDisplay(ctx, aggregator, display, config, testDate, reportTZ)

		case <-ctx.Done():
			// Context is cancelled when SIGINT (Ctrl+C) or SIGTERM is received.
			// In-flight HTTP requests are also cancelled via the shared context.
			display.ShowInfo("Shutting down gracefully...")
			return
		}
	}
}

func fetchAndDisplay(ctx context.Context, aggregator *DataAggregator, display *Display, config *Config, testDate time.Time, reportTZ *time.Location) {
	metrics, err := aggregator.GetAggregatedMetrics(ctx, config.Systems, config.API, testDate, reportTZ)
	if err != nil {
		// If context was cancelled (shutdown in progress), exit silently
		if ctx.Err() != nil {
			return
		}
		// Check if it's a 429 rate limit error - if so, exit instead of continuing
		if isRateLimitError(err) {
			// Error message already printed in aggregator.go, just exit
			os.Exit(1)
		}
		display.ShowError(err)
		return
	}

	// Clear screen for cleaner output
	fmt.Print("\033[H\033[2J")

	display.ShowMetrics(metrics)
}
