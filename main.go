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
//   - docs/ARCHITECTURE.md: Detailed architecture and execution flow
//   - docs/OAUTH_SETUP.md: OAuth authentication setup guide
//   - docs/GO_BEST_PRACTICES.md: Go patterns and idioms used in this codebase
//   - docs/GO_CONCEPTS.md: Go language reference for concepts used here
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
// The application follows a clean separation of concerns with modular internal packages:
//
//	Entry Point (main package):
//	- main.go: Application orchestration, signal handling, and package coordination
//
//	Application Layer (internal/app):
//	- setup.go: Application initialization, OAuth adapter, display setup, mode configuration
//	- runner.go: Execution modes (once/continuous), metric fetching and display
//	- trueup.go: True-up year report via single-batch lifetime query and report conversion
//
//	CLI Layer (internal/cli):
//	- flags.go: Command-line flag parsing and definitions
//	- cache_commands.go: Cache management command handlers
//
//	Authentication (internal/oauth):
//	- oauth.go: OAuth 2.0 token acquisition and refresh (with in-memory caching)
//	- setup.go: Interactive OAuth setup wizard for first-time configuration
//
//	Data Aggregation (internal/aggregator):
//	- aggregator.go: Multi-system data orchestration and metric aggregation
//	- types.go: Metric data structures and interfaces
//
//	API Communication (internal/api):
//	- client.go: HTTP client for Enlighten Cloud API v4, handles all API requests
//	- types.go: API request/response types
//	- interface.go: API client interfaces
//
//	Supporting Services:
//	- internal/cache: Disk-based response caching with URL normalization
//	- internal/config: YAML configuration loading, validation, color conversion
//	- internal/constants: Application-wide constants (ANSI codes, error messages, etc.)
//	- internal/display: Terminal output formatting with customizable colors
//	- internal/parser: JSON telemetry response parsing
//	- internal/timezone: Timezone handling and date boundary calculations
//	- internal/urlbuilder: API URL construction with proper date ranges
//	- internal/validation: Test mode validation with tolerance-based comparison
//
// EXECUTION FLOW
// --------------
//  1. Parse CLI flags via internal/cli (--continuous, --date, --test, etc.)
//  2. Handle cache commands via internal/cli if requested
//  3. Load and validate config.yaml via internal/config
//  4. Handle OAuth setup via internal/oauth if requested
//  5. Create DataAggregator with OAuth adapter from internal/app
//  6. Setup display with colors from internal/app
//  7. Configure modes (test/cache) via internal/app
//  8. If --true-up: call app.RunTrueUp (single-batch lifetime query, no battery) and exit.
//     Otherwise, run the standard execution mode:
//     - For each system in config:
//     a. Get OAuth access token via internal/oauth (cached or refreshed)
//     b. Create API client via internal/api for the system
//     c. Fetch metrics via Cloud API (with caching from internal/cache)
//     - Aggregate metrics via internal/aggregator
//     - Validate if in test mode via internal/validation
//     - Display formatted report via internal/display
//
// For continuous mode, step 8 repeats at the configured refresh interval.
// Note: If a past date is supplied via --date, the program automatically runs once
// since historical data doesn't change over time.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/app"
	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/cli"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/oauth"
	"enphase-monitor/internal/timezone"
)

func main() {
	// Parse command-line flags
	flags := cli.ParseFlags()

	// Handle cache management commands
	if flags.ClearCache {
		if err := cli.HandleClearCache(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to clear cache: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if flags.ClearAllCache {
		if err := cli.HandleClearAllCache(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	}

	// Load configuration
	cfg, err := config.LoadConfig(flags.ConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n\nPlease copy config.yaml.example to config.yaml and fill in your details.\n", err)
		os.Exit(1)
	}

	// Handle OAuth setup (use signal context so Ctrl+C cancels token exchange)
	if flags.SetupOAuth {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		if err := oauth.Setup(ctx, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "OAuth setup failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Create aggregator with OAuth token adapter
	getAccessTokenAdapter := app.CreateOAuthAdapter()
	agg := aggregator.NewDataAggregator(getAccessTokenAdapter)

	// Load timezone for reporting/display
	reportTZ, err := timezone.LoadTimezone(cfg.Timezone)
	if err != nil {
		// Should not happen, but use OS system timezone as fallback
		reportTZ = time.Now().Location()
	}

	// Create display with colors from config
	disp := app.SetupDisplay(cfg, reportTZ)

	// Setup signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --cache: serve report from cache only; diagnose missing endpoints if incomplete.
	// Handles --cache alone, --cache --date, and --cache --true-up.
	if flags.CacheOnly {
		app.ConfigureModes(false, false, flags.Debug)
		printDebugStartup(flags.Debug, reportTZ)
		parsedInput, err := app.ParseTestDate(flags.TestDate, reportTZ)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		rc := app.RunConfig{
			Agg:       agg,
			Disp:      disp,
			Cfg:       cfg,
			TestDate:  parsedInput.Date,
			QueryType: parsedInput.QueryType,
			ReportTZ:  reportTZ,
			Debug:     flags.Debug,
		}
		if err := app.RunCacheReport(ctx, rc, flags.TrueUp); err != nil {
			if !errors.Is(err, app.ErrCacheIncomplete) {
				disp.ShowError(err)
			}
			os.Exit(1)
		}
		return
	}

	// --true-up takes precedence over --date; handle it early and exit.
	if flags.TrueUp != "" {
		app.ConfigureModes(flags.TestMode, flags.NoCache, flags.Debug)
		printDebugStartup(flags.Debug, reportTZ)
		rc := app.RunConfig{
			Agg:      agg,
			Disp:     disp,
			Cfg:      cfg,
			ReportTZ: reportTZ,
			Debug:    flags.Debug,
		}
		if err := app.RunTrueUp(ctx, rc, flags.TrueUp); err != nil {
			if constants.IsRateLimitError(err) {
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
				fmt.Fprintf(os.Stderr, "Please wait %d seconds before rerunning the program.\n", constants.APIRateLimitWaitSeconds)
			} else {
				disp.ShowError(err)
			}
			os.Exit(1)
		}
		return
	}

	// Parse test date (returns date and query type)
	parsedInput, err := app.ParseTestDate(flags.TestDate, reportTZ)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	testDateParsed := parsedInput.Date
	queryType := parsedInput.QueryType

	// Validate cache exists when in test mode (before configuring modes)
	if flags.TestMode {
		if err := app.ValidateTestModeCache(testDateParsed, reportTZ); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
	}

	// Configure test mode and cache mode
	app.ConfigureModes(flags.TestMode, flags.NoCache, flags.Debug)
	printDebugStartup(flags.Debug, reportTZ)

	// Default is run-once; --continuous enables periodic refresh.
	// Month/year queries and past-date queries always run once regardless of --continuous.
	runContinuous := flags.Continuous
	if queryType == constants.QueryTypeMonth || queryType == constants.QueryTypeYear {
		runContinuous = false
	} else if !testDateParsed.IsZero() && timezone.IsPastPeriod(testDateParsed, queryType, reportTZ) {
		runContinuous = false
	}

	rc := app.RunConfig{
		Agg:       agg,
		Disp:      disp,
		Cfg:       cfg,
		TestDate:  testDateParsed,
		QueryType: queryType,
		ReportTZ:  reportTZ,
		Debug:     flags.Debug,
	}

	// Default: run once and exit. With --continuous, loop with periodic refresh.
	if !runContinuous {
		if err := app.RunOnce(ctx, rc, flags.TestMode); err != nil {
			if constants.IsRateLimitError(err) {
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
				fmt.Fprintf(os.Stderr, "Please wait %d seconds before rerunning the program.\n", constants.APIRateLimitWaitSeconds)
			} else {
				disp.ShowError(err)
			}
			os.Exit(1)
		}
		return
	}
	if err := app.RunContinuous(ctx, rc); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

// printDebugStartup prints the rate-limit status when debug mode is on.
// It shows the last recorded API call time (so the user knows if they are
// still inside the 60-second cooling-off window) and the remaining budget.
func printDebugStartup(debug bool, reportTZ *time.Location) {
	if !debug {
		return
	}
	now := time.Now().In(reportTZ)
	fmt.Fprintf(os.Stderr, "[DEBUG] --- startup ---\n")
	fmt.Fprintf(os.Stderr, "[DEBUG] current time : %s\n", now.Format("2006-01-02 15:04:05 MST"))
	if last, ok := cache.LastAPICallTime(); ok {
		last = last.In(reportTZ)
		age := time.Since(last).Round(time.Second)
		windowReset := cache.MinRequestInterval - time.Since(last)
		if windowReset < 0 {
			windowReset = 0
		}
		fmt.Fprintf(os.Stderr, "[DEBUG] last API call: %s (%s ago)\n", last.Format("15:04:05 MST"), age)
		if windowReset > 0 {
			fmt.Fprintf(os.Stderr, "[DEBUG] rate window resets in: %s\n", windowReset.Round(time.Second))
		} else {
			fmt.Fprintf(os.Stderr, "[DEBUG] rate window: clear (no calls in last 60s)\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "[DEBUG] last API call: none (no calls recorded in last 60s)\n")
	}
	budget := cache.RemainingBudget()
	fmt.Fprintf(os.Stderr, "[DEBUG] API budget   : %d/%d calls remaining\n", budget, cache.MaxRequestsPerWindow)
	fmt.Fprintf(os.Stderr, "[DEBUG] ---\n")
}
