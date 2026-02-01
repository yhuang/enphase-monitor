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
// The application follows a clean separation of concerns with modular internal packages:
//
//	Entry Point (main package):
//	- main.go: Application orchestration, signal handling, and package coordination
//
//	Application Layer (internal/app):
//	- setup.go: Application initialization, OAuth adapter, display setup, mode configuration
//	- runner.go: Execution modes (once/continuous), metric fetching and display
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
//  1. Parse CLI flags via internal/cli (--once, --date, --test, etc.)
//  2. Handle cache commands via internal/cli if requested
//  3. Load and validate config.yaml via internal/config
//  4. Handle OAuth setup via internal/oauth if requested
//  5. Create DataAggregator with OAuth adapter from internal/app
//  6. Setup display with colors from internal/app
//  7. Configure modes (test/cache) via internal/app
//  8. Run execution mode via internal/app:
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
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/app"
	"enphase-monitor/internal/cli"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/oauth"
	"enphase-monitor/internal/timezone"
)

func main() {
	// Parse command-line flags
	flags := cli.ParseFlags()

	// Handle cache management commands
	if flags.ClearCache {
		if err := cli.HandleClearCache(); err != nil {
			app.ExitWithError("Failed to clear cache: %v\n", err)
		}
		return
	}

	if flags.ClearAllCache {
		if err := cli.HandleClearAllCache(); err != nil {
			app.ExitWithError("%v\n", err)
		}
		return
	}

	if flags.ListCache {
		if err := cli.HandleListCache(); err != nil {
			app.ExitWithError("%v\n", err)
		}
		return
	}

	if flags.InspectCache != "" {
		if err := cli.HandleInspectCache(flags.InspectCache, flags.ConfigFile); err != nil {
			app.ExitWithError("%v\n", err)
		}
		return
	}

	// Load configuration
	cfg, err := config.LoadConfig(flags.ConfigFile)
	if err != nil {
		app.ExitWithError("Failed to load configuration: %v\n\nPlease copy config.yaml.example to config.yaml and fill in your details.\n", err)
	}

	// Handle OAuth setup
	if flags.SetupOAuth {
		if err := oauth.Setup(cfg); err != nil {
			app.ExitWithError("OAuth setup failed: %v\n", err)
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

	// Parse test date
	testDateParsed, err := app.ParseTestDate(flags.TestDate, reportTZ)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	// Validate cache exists when in test mode (before configuring modes)
	if flags.TestMode {
		if err := app.ValidateTestModeCache(testDateParsed, reportTZ); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
	}

	// Configure test mode and cache mode
	app.ConfigureModes(flags.TestMode, flags.NoCache)

	// Determine run mode
	// If querying a past date, always run once since historical data doesn't change
	runOnce := flags.Once
	if !testDateParsed.IsZero() && timezone.IsPastDate(testDateParsed, reportTZ) {
		runOnce = true
		if !flags.Once {
			// Inform user why we're running once instead of continuous
			fmt.Printf("Note: Running once for historical date %s (data won't change)\n\n",
				testDateParsed.Format("2006-01-02"))
		}
	}

	// Run once or continuous
	if runOnce {
		app.RunOnce(ctx, agg, disp, cfg, testDateParsed, flags.TestMode, reportTZ)
	} else {
		app.RunContinuous(ctx, agg, disp, cfg, testDateParsed, reportTZ)
	}
}
