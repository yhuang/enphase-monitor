// Package main implements the Enphase Monitor application.
//
// OVERVIEW
// --------
// This application monitors energy metrics from one or more Enphase solar Systems
// at a Site via the Enphase Enlighten Cloud API v4. It aggregates per-System data
// into Site-level totals and displays a formatted report in the terminal.
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
//   - Intelligent Caching: Disk-based caching to stay within the API Budget (10 calls/minute)
//   - Past Period Queries: Query any past date with --date flag
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
//	- trueup.go: True-Up Mode report via single-batch Lifetime Data query and report conversion
//	- backfill.go: Backfill Mode — live per-day fetch over a date range, writing History Records into history/
//	- weather.go: Best-effort weather enrichment for Day-Mode reports
//	- cache_report.go: --cache mode — checks per-System endpoint coverage and runs a fully-cached report
//
//	CLI Layer (internal/cli):
//	- flags.go: Command-line flag parsing and definitions
//	- cache_commands.go: Cache management command handlers
//
//	Authentication (internal/oauth):
//	- oauth.go: OAuth 2.0 token acquisition and refresh (with in-memory caching)
//	- authorization.go: Interactive OAuth authorization wizard that obtains a refresh token
//
//	Data Aggregation (internal/aggregator):
//	- aggregator.go: Multi-system data orchestration, metric aggregation, and the
//	  consumer-side CloudClient interface the aggregator depends on
//	- types.go: Metric data structures (AggregatedMetrics, SystemMetrics)
//
//	API Communication (internal/api):
//	- client.go: HTTP client for Enlighten Cloud API v4, handles all API requests
//	- types.go: API request/response types
//	- cache_check.go: Preflight per-system/endpoint cache-availability probe
//
//	Supporting Services:
//	- internal/cache: Disk-based response caching with URL normalization and the sliding-window API Budget counter
//	- internal/config: YAML configuration loading, validation, color conversion
//	- internal/constants: Application-wide constants (ANSI codes, error messages, QueryMode enum, etc.)
//	- internal/display: Terminal output formatting with customizable colors
//	- internal/geocode: Postal-code-to-coordinates lookup (Zippopotam.us) for weather geolocation
//	- internal/history: Per-day energy+weather History Record schema and JSON writer (history/)
//	- internal/location: Resolves and caches the systems' coordinates for weather (populated by --init)
//	- internal/parser: JSON telemetry response parsing (Interval Data and Lifetime Data shapes)
//	- internal/timezone: Timezone handling, Past Period detection, and date boundary calculations
//	- internal/types: Shared type definitions (SystemConfig, APIConfig) that break circular dependencies
//	- internal/urlbuilder: API URL construction with proper date ranges
//	- internal/validation: Validation Mode (--test flag) with tolerance-based comparison
//	- internal/weather: Open-Meteo daily/current weather client with WMO code mapping
//
// EXECUTION FLOW
// --------------
//  1. Parse CLI flags via internal/cli (--continuous, --date, --test, etc.)
//  2. Handle cache commands via internal/cli if requested
//  3. Load and validate config.yaml via internal/config
//  4. Handle OAuth setup via internal/oauth if requested
//  5. Create DataAggregator with OAuth adapter from internal/app
//  6. Setup display with colors from internal/app
//  7. Enforce the init guard: every report mode requires a prior --init (cached
//     location); cache-management and --update-refresh-token are exempt.
//  8. Configure modes (Validation Mode, Cache Mode) via internal/app
//  9. Dispatch to one of four run paths and exit:
//     a. If --backfill-from: call app.RunBackfill (live per-day fetch over a date range into history/) and exit.
//     b. Else if --cache: call app.RunCacheReport (cache-only run; lists missing endpoints if incomplete) and exit.
//     c. Else if --true-up: call app.RunTrueUp (single-batch lifetime query, no battery) and exit.
//     d. Otherwise, run the standard execution mode:
//     - For each system in config:
//     i.   Get OAuth access token via internal/oauth (cached or refreshed)
//     ii.  Create API client via internal/api for the system
//     iii. Fetch metrics via Cloud API (with caching from internal/cache)
//     - Aggregate metrics via internal/aggregator
//     - Validate if in Validation Mode via internal/validation
//     - Display formatted report via internal/display
//
// For continuous mode, step 8c repeats at the configured refresh interval. Continuous mode
// is restricted to today's Day Mode query — Month, Year, Past Period, and True-Up Mode
// queries are silently downgraded to run once and exit.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/app"
	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/cli"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/credentials"
	"enphase-monitor/internal/location"
	"enphase-monitor/internal/oauth"
	"enphase-monitor/internal/timezone"
	"enphase-monitor/internal/weather"
)

func main() {
	// Parse command-line flags
	flags := cli.ParseFlags()

	// The cache-clearing commands are mutually exclusive; reject ambiguous
	// combinations rather than silently running whichever is checked first.
	clearCommands := 0
	if flags.ClearCache {
		clearCommands++
	}
	if flags.ClearCacheDate != "" {
		clearCommands++
	}
	if flags.ClearAllCache {
		clearCommands++
	}
	if clearCommands > 1 {
		fmt.Fprintln(os.Stderr, "Error: --clear-cache, --clear-cache-date, and --clear-all-cache are mutually exclusive")
		os.Exit(1)
	}

	// Backfill Mode is a standalone report mode; reject combinations that would
	// otherwise be silently ignored by mode-dispatch order.
	if flags.BackfillFrom != "" {
		switch {
		case flags.Continuous:
			fmt.Fprintln(os.Stderr, "Error: --backfill-from cannot be combined with --continuous")
			os.Exit(1)
		case flags.TrueUp != "":
			fmt.Fprintln(os.Stderr, "Error: --backfill-from cannot be combined with --true-up")
			os.Exit(1)
		case flags.Initialize:
			fmt.Fprintln(os.Stderr, "Error: --backfill-from cannot be combined with --init")
			os.Exit(1)
		}
	}

	// Handle cache management commands
	if flags.ClearCache {
		if err := cli.HandleClearCache(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to clear cache: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if flags.ClearCacheDate != "" {
		if err := cli.HandleClearCacheForDate(flags.ClearCacheDate); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
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

	// Load API credentials from the separate credentials file. A missing file is
	// not fatal: ApplyCredentials falls back to a legacy api: block in config.yaml
	// for backward compatibility, and errors out if neither source has credentials.
	creds, err := config.LoadCredentials(flags.CredentialsFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "Failed to load credentials: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.ApplyCredentials(creds); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load credentials: %v\n\nPlease copy credentials.yaml.example to credentials.yaml and fill in your details (run --update-refresh-token for the refresh token).\n", err)
		os.Exit(1)
	}

	// Build the credential pool once: it is shared by the report path (so 429
	// cooldown state survives across Continuous Mode ticks), --init, and --update-refresh-token.
	pool := credentials.NewPool(cfg.Credentials)

	// Handle --update-refresh-token (use signal context so Ctrl+C cancels token exchange)
	if flags.UpdateRefreshToken {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		// --all re-authorizes every configured credential in turn.
		if flags.All {
			if err := updateAllRefreshTokens(ctx, pool, flags.CredentialsFile, flags.NewOnly); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			return
		}

		cred, err := selectCredential(pool, flags.Credential)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		if err := updateOneRefreshToken(ctx, cred, flags.CredentialsFile); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	}

	// Handle --init: resolve and cache the systems' location for weather
	// reporting. Done out of band (one /systems call) so it never competes with
	// the per-minute telemetry budget on a live report. Run once before normal
	// use; re-run if the cache is cleared.
	if flags.Initialize {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		if err := initializeLocation(ctx, pool.First(), flags.Force); err != nil {
			fmt.Fprintf(os.Stderr, "Initialization failed: %v\n", err)
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

	// Require initialization before any report mode. --init caches the systems'
	// coordinates; without them weather enrichment is impossible, so we gate all
	// report-generating modes on a successful prior --init. Cache-management and
	// auth flags are exempt (handled and returned above).
	loc := location.NewResolver()
	if _, ok := loc.CachedPrimaryCoordinates(); !ok {
		fmt.Fprintln(os.Stderr, "enphase-monitor: not initialized — run `enphase-monitor --init` first.")
		os.Exit(1)
	}

	// Backfill Mode: fetch a range of past days into history/ and exit.
	if flags.BackfillFrom != "" {
		fromDate, err := time.ParseInLocation(constants.DateFormat, flags.BackfillFrom, reportTZ)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: invalid --backfill-from date %q: use YYYY-MM-DD\n", flags.BackfillFrom)
			os.Exit(1)
		}
		// An explicit --date bounds the backfill end; otherwise RunBackfill
		// defaults to yesterday. Only the day format is meaningful here.
		var endDate time.Time
		if flags.TestDate != "" {
			endDate, err = time.ParseInLocation(constants.DateFormat, flags.TestDate, reportTZ)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: invalid --date %q: use YYYY-MM-DD for backfill\n", flags.TestDate)
				os.Exit(1)
			}
		}
		// noCache stays false here: RunBackfill disables the cache itself, so we
		// avoid the redundant "LIVE MODE" notice (backfill is always live).
		app.ConfigureModes(false /* validationMode */, false /* noCache */, flags.Debug)
		printDebugStartup(flags.Debug, reportTZ)

		rc := app.RunConfig{
			Agg:      agg,
			Pool:     pool,
			Disp:     disp,
			Cfg:      cfg,
			TestDate: endDate,
			ReportTZ: reportTZ,
			Debug:    flags.Debug,
			Location: loc,
			Weather:  weather.NewClient(cache.GetCacheDir()),
		}
		if err := app.RunBackfill(ctx, rc, fromDate, flags.Force); err != nil {
			disp.ShowError(err)
			os.Exit(1)
		}
		return
	}

	// --cache: serve report from cache only; diagnose missing endpoints if incomplete.
	// Handles --cache alone, --cache --date, and --cache --true-up.
	if flags.CachedMode {
		app.ConfigureModes(false /* validationMode */, false /* noCache */, flags.Debug)
		printDebugStartup(flags.Debug, reportTZ)
		parsedInput, err := app.ParseTestDate(flags.TestDate, reportTZ)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		rc := app.RunConfig{
			Agg:       agg,
			Pool:      pool,
			Disp:      disp,
			Cfg:       cfg,
			TestDate:  parsedInput.Date,
			QueryMode: parsedInput.QueryMode,
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
		app.ConfigureModes(flags.Validation, flags.NoCache, flags.Debug)
		printDebugStartup(flags.Debug, reportTZ)
		rc := app.RunConfig{
			Agg:      agg,
			Pool:     pool,
			Disp:     disp,
			Cfg:      cfg,
			ReportTZ: reportTZ,
			Debug:    flags.Debug,
		}
		if err := app.RunTrueUp(ctx, rc, flags.TrueUp); err != nil {
			if constants.IsRateLimitError(err) {
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
				fmt.Fprintf(os.Stderr, "Please wait %d seconds before rerunning the program.\n", constants.APIBudgetWindowSeconds)
			} else {
				disp.ShowError(err)
			}
			os.Exit(1)
		}
		return
	}

	// Parse test date (returns date and query mode)
	parsedInput, err := app.ParseTestDate(flags.TestDate, reportTZ)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	testDateParsed := parsedInput.Date
	queryMode := parsedInput.QueryMode

	// Validate cache exists when in Validation Mode (before configuring modes)
	if flags.Validation {
		if err := app.ValidateValidationModeCache(testDateParsed, reportTZ); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
	}

	// Configure Validation Mode and cache mode
	app.ConfigureModes(flags.Validation, flags.NoCache, flags.Debug)
	printDebugStartup(flags.Debug, reportTZ)

	// Default is run-once; --continuous enables periodic refresh.
	// Month, Year, and Past Period queries always run once regardless of --continuous.
	runContinuous := flags.Continuous
	if queryMode == constants.QueryModeMonth || queryMode == constants.QueryModeYear {
		runContinuous = false
	} else if !testDateParsed.IsZero() && timezone.IsPastPeriod(testDateParsed, queryMode, reportTZ) {
		runContinuous = false
	}

	rc := app.RunConfig{
		Agg:       agg,
		Pool:      pool,
		Disp:      disp,
		Cfg:       cfg,
		TestDate:  testDateParsed,
		QueryMode: queryMode,
		ReportTZ:  reportTZ,
		Debug:     flags.Debug,
		// Best-effort temperature enrichment for Day-Mode reports. Both clients
		// cache aggressively, so this stays off the per-run API hot path.
		Location: loc,
		Weather:  weather.NewClient(cache.GetCacheDir()),
	}

	// Default: run once and exit. With --continuous, loop with periodic refresh.
	if !runContinuous {
		if err := app.RunOnce(ctx, rc, flags.Validation); err != nil {
			if constants.IsRateLimitError(err) {
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
				fmt.Fprintf(os.Stderr, "Please wait %d seconds before rerunning the program.\n", constants.APIBudgetWindowSeconds)
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

// printDebugStartup prints the API Budget status when debug mode is on.
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

// initializeLocation resolves the systems' coordinates (one /systems call) and
// caches them for weather reporting. Run by --init, out of band from reports,
// so it never competes with the per-minute telemetry budget. Returns an error
// (unlike the report path's best-effort enrichment) so the user knows whether
// initialization succeeded.
func initializeLocation(ctx context.Context, cred *config.APIConfig, force bool) error {
	if cred == nil {
		return errors.New("API configuration is required")
	}
	token, err := oauth.GetAccessToken(ctx, cred)
	if err != nil {
		return fmt.Errorf("could not obtain access token: %w", err)
	}
	resolver := location.NewResolver()
	resolve := resolver.SystemLocations
	if force {
		resolve = resolver.RefreshSystemLocations
	}
	locs, err := resolve(ctx, cred.Key, token)
	if err != nil {
		return fmt.Errorf("could not resolve location: %w", err)
	}
	action := "Initialized"
	if force {
		action = "Re-initialized (forced)"
	}
	fmt.Printf("%s: resolved location for %d system(s) and cached it for weather reporting.\n", action, len(locs))
	for _, l := range locs {
		fmt.Printf("  - %s: %s, %s %s (%.4f, %.4f)\n", l.Name, l.City, l.State, l.PostalCode, l.Latitude, l.Longitude)
	}

	// Write the WMO weather-code legend to the project root so the weather_code
	// field in reports and History Records is decodable. A local write that does
	// not depend on the location lookup; a failure is non-fatal to init.
	if err := weather.WriteCodeLegend(weather.CodeLegendFileName); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not write %s: %v\n", weather.CodeLegendFileName, err)
	} else {
		fmt.Printf("Wrote %s (WMO weather-code reference).\n", weather.CodeLegendFileName)
	}
	return nil
}

// selectCredential picks which credential set --update-refresh-token operates on. With a
// single credential the name is optional; with more than one, the name must be
// passed as an argument (--update-refresh-token <name>) and must match a configured name.
func selectCredential(pool *credentials.Pool, name string) (*config.APIConfig, error) {
	if name == "" {
		if pool.Len() == 1 {
			return pool.First(), nil
		}
		return nil, fmt.Errorf("multiple credentials configured; name one as an argument: --update-refresh-token <name> (available: %s)", strings.Join(pool.Names(), ", "))
	}
	cred, ok := pool.ByName(name)
	if !ok {
		return nil, fmt.Errorf("no credential named %q (available: %s)", name, strings.Join(pool.Names(), ", "))
	}
	return cred, nil
}

// updateOneRefreshToken runs the OAuth wizard for a single credential and writes
// the obtained refresh token into the credentials file.
func updateOneRefreshToken(ctx context.Context, cred *config.APIConfig, credentialsFile string) error {
	authorizer := oauth.NewBrowserAuthorizer(ctx)
	defer authorizer.Close()
	fmt.Println("A Chrome window will open. Log in — the app is approved automatically, no clicking required.")

	refreshToken, err := oauth.AuthorizeViaBrowser(ctx, authorizer, cred)
	if err != nil {
		return fmt.Errorf("failed to obtain refresh token for %q: %w", cred.Name, err)
	}
	if err := config.UpdateRefreshToken(credentialsFile, cred.Name, refreshToken); err != nil {
		return fmt.Errorf("obtained a refresh token for %q but failed to save it: %w", cred.Name, err)
	}
	fmt.Printf("Saved refresh_token for credential %q to %s\n", cred.Name, credentialsFile)
	return nil
}

// updateAllRefreshTokens re-authorizes every configured credential in turn,
// driving one browser session that approves each app's consent automatically —
// the user logs in once and never clicks "Allow Access". It attempts each
// credential even if an earlier one failed, then reports which failed; a Ctrl+C
// (context cancellation) aborts the rest. When newOnly is set, credentials that
// already have a refresh_token are skipped, so a freshly-seeded batch can be
// authorized without re-doing the working ones.
func updateAllRefreshTokens(ctx context.Context, pool *credentials.Pool, credentialsFile string, newOnly bool) error {
	names := pool.Names()

	authorizer := oauth.NewBrowserAuthorizer(ctx)
	defer authorizer.Close()
	fmt.Println("A Chrome window will open. Log in once — each app is approved automatically, no clicking required.")

	// status writes a single self-clearing line (\r + ANSI clear-to-EOL) so the
	// per-credential progress updates in place instead of stacking up.
	status := func(format string, a ...any) {
		fmt.Fprintf(os.Stderr, "\r\033[K"+format, a...)
	}

	var failed, skipped []string
	for i, name := range names {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr)
			return ctx.Err()
		}
		cred, _ := pool.ByName(name)
		if newOnly && cred.RefreshToken != "" {
			skipped = append(skipped, name)
			continue
		}
		status("[%d/%d] %s — authorizing…", i+1, len(names), name)
		token, err := oauth.AuthorizeViaBrowser(ctx, authorizer, cred)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				fmt.Fprintln(os.Stderr)
				return err
			}
			// Persist failures on their own line (they survive later updates).
			fmt.Fprintf(os.Stderr, "\r\033[K[%d/%d] %s — FAILED: %v\n", i+1, len(names), name, err)
			failed = append(failed, name)
			continue
		}
		if err := config.UpdateRefreshToken(credentialsFile, cred.Name, token); err != nil {
			fmt.Fprintf(os.Stderr, "\r\033[K[%d/%d] %s — got token but save failed: %v\n", i+1, len(names), name, err)
			failed = append(failed, name)
			continue
		}
		status("[%d/%d] %s — saved", i+1, len(names), name)
	}
	fmt.Fprint(os.Stderr, "\r\033[K") // clear the last status line before the summary

	if len(skipped) > 0 {
		fmt.Printf("Skipped %d already-authorized credential(s) (--new-only).\n", len(skipped))
	}
	if len(failed) > 0 {
		return fmt.Errorf("authorization failed for %d of %d credential(s): %s", len(failed), len(names), strings.Join(failed, ", "))
	}
	authorized := len(names) - len(skipped)
	fmt.Printf("Done: authorized %d credential(s).\n", authorized)
	return nil
}
