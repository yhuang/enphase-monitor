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
//   - Intelligent Caching: Disk-based caching to stay within the API Budget (1000 calls/month per key)
//   - Past Period Queries: Query any past date with --date flag
//   - Color Customization: Customize terminal output colors via config.yaml
//   - Backfill and Weather: --start-date builds per-day Enphase + PG&E + Open-Meteo
//     History Records in history/
//   - Raw Source Archive: every source response (Enphase API, Open-Meteo, PG&E
//     Green Button) is also kept untouched under raw/ with a SHA-256 manifest, and
//     offline --verify-*-raw commands re-derive history/ from it at any time
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
//	- runner.go: RunOnce execution mode, metric fetching and display
//	- trueup.go: True-Up Mode report via single-batch Lifetime Data query and report conversion
//	- backfill.go: Backfill Mode — live per-day fetch over a date range, writing History Records into history/
//	- weather.go: Best-effort weather enrichment for Day-Mode reports
//	- rawweather.go: Raw-weather maintenance — HistoryDates, RefetchWeatherRaw (--raw-refetch-weather),
//	  ApplyRawWeather (--raw-apply-weather, with the history/.weather-provenance.json sidecar)
//	- verifyenphaseraw.go: VerifyEnphaseRaw (--verify-enphase-raw), the offline re-derivation of every
//	  history/enphase-*.json day from the archived API responses
//	- verifyenphase.go: VerifyEnphaseArchive (--verify-enphase-archive), the one live check (about 10 API calls)
//	  that Enphase raw archiving works and that a fresh fetch reproduces the stored record
//
//	CLI Layer (internal/cli):
//	- flags.go: Command-line flag parsing and definitions, the grouped --help, and ValidateMaintenance
//	  (at most one maintenance command; only the raw weather ones take a date range)
//	- cache_commands.go: Cache management command handlers
//
//	Authentication (internal/oauth):
//	- oauth.go: OAuth 2.0 token acquisition and refresh (with in-memory caching)
//	- browser.go: Browser-driven OAuth authorization that auto-approves consent and obtains refresh tokens
//
//	Data Aggregation (internal/aggregator):
//	- aggregator.go: Multi-system data orchestration, metric aggregation, and the
//	  consumer-side CloudClient interface the aggregator depends on
//	- types.go: Metric data structures (AggregatedMetrics, SystemMetrics)
//
//	API Communication (internal/api):
//	- client.go: HTTP client for Enlighten Cloud API v4, handles all API requests; also archives
//	  every live response via internal/rawstore and exposes DayKWh, the day-endpoint parser the
//	  offline Enphase verification reuses
//	- types.go: API request/response types
//
//	Supporting Services:
//	- internal/browser: Headed Chrome launcher (chromedp) with a disposable profile, for portal automation
//	- internal/cache: Disk-based response caching with URL normalization, Past Period handling, and endpoint/system tagging (per-credential API Budget lives in internal/credentials)
//	- internal/config: YAML configuration loading, validation, color conversion
//	- internal/constants: Application-wide constants (ANSI codes, error messages, QueryMode enum, etc.)
//	- internal/credentials: Credential pool — least-used-first spread, 429 failover, and per-key monthly API budget (--seed-credentials, --refresh-quota)
//	- internal/display: Terminal output formatting with customizable colors
//	- internal/enphase: Developer-portal scraping (login, credential seeding, monthly stats) — the portal exposes no management API
//	- internal/geocode: Postal-code-to-coordinates lookup (Zippopotam.us) for weather geolocation
//	- internal/history: Per-day energy+weather History Record schema and JSON writer (history/); Dataset type governs prefixed filenames (enphase-<date>.json, pge-<date>.json)
//	- internal/location: Resolves and caches the systems' coordinates for weather (populated by --init)
//	- internal/parser: JSON telemetry response parsing (Interval Data and Lifetime Data shapes)
//	- internal/pge: PG&E Green Button integration — headed-Chrome browser download
//	  (browserpull.go, login.go, download.go, autoform.go) that archives the original ZIP and can rebuild history from it
//	  offline (ReprocessRaw), ESPI XML parsing (espi.go: register selection, local-clock decoding, Wh scale), write-time
//	  sanity warnings (sanity.go), raw-archive verification (verify.go), history writer (history.go)
//	- internal/rawstore: Raw source archive under raw/<source>/ with a SHA-256 manifest (raw/manifest.jsonl) — Save/Record,
//	  ImportEnphaseCache, VerifySource and the manifest readers; disabled unless rawstore.Dir is set
//	- internal/timezone: Timezone handling, Past Period detection, and date boundary calculations
//	- internal/types: Shared type definitions (SystemConfig, APIConfig) that break circular dependencies
//	- internal/urlbuilder: API URL construction with proper date ranges
//	- internal/weather: Open-Meteo daily/current weather client with WMO code mapping
//
// EXECUTION FLOW
// --------------
//  1. Parse CLI flags via internal/cli (--date, --no-cache, --debug, etc.) and enable the
//     raw archive (rawstore.Dir = "raw") so every live response is kept
//  2. Handle cache commands via internal/cli if requested, then validate the maintenance
//     commands (flags.ValidateMaintenance)
//     Commands that need no Enphase config or credentials dispatch here, before either
//     is loaded, and exit. Offline: --raw-import-cache, --verify-pge-raw,
//     --verify-enphase-raw, --verify-weather-raw, --raw-apply-weather, --pge-reprocess.
//     Network but credential-free: --raw-refetch-weather (Open-Meteo; needs the location
//     cached by --init) and the standalone --pge-web-only pull (PG&E browser download)
//  3. Load and validate config.yaml via internal/config
//  4. Handle OAuth setup via internal/oauth if requested
//  5. Create DataAggregator with OAuth adapter from internal/app
//  6. Setup display with colors from internal/app
//  7. Enforce the init guard: every report mode requires a prior --init (cached
//     location); cache-management and --update-refresh-tokens are exempt.
//  8. Configure cache/debug modes via internal/app
//  9. Dispatch to one of three run paths and exit (--verify-enphase-archive, the one
//     maintenance command that spends API quota, runs just before these, after the
//     init guard, and never writes history/):
//     a. If --start-date: call app.RunBackfill (live per-day fetch over a date range into history/) and exit.
//     b. Else if --true-up: call app.RunTrueUp (single-batch lifetime query, no battery) and exit.
//     c. Otherwise, run the standard execution mode:
//     - For each system in config:
//     i.   Get OAuth access token via internal/oauth (cached or refreshed)
//     ii.  Create API client via internal/api for the system
//     iii. Fetch metrics via Cloud API (with caching from internal/cache)
//     - Aggregate metrics via internal/aggregator
//     - Display formatted report via internal/display
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/api"
	"enphase-monitor/internal/app"
	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/cli"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/credentials"
	"enphase-monitor/internal/display"
	"enphase-monitor/internal/enphase"
	"enphase-monitor/internal/location"
	"enphase-monitor/internal/oauth"
	"enphase-monitor/internal/pge"
	"enphase-monitor/internal/rawstore"
	"enphase-monitor/internal/timezone"
	"enphase-monitor/internal/weather"
)

// tmpDir is the working directory for files that are transient and should not
// be committed (downloaded PG&E XML, etc.). Excluded from version control via .gitignore.
const tmpDir = "tmp"

// rawDir holds untouched source downloads (one subdirectory per source) kept so
// processing can be re-verified later. Gitignored: it contains account data.
const rawDir = "raw"

func main() {
	// Parse command-line flags
	flags := cli.ParseFlags()

	// Keep an untouched copy of every source response under raw/ (see rawstore).
	rawstore.Dir = rawDir

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

	// Maintenance commands (raw archive, reprocess, verify): at most one, and only
	// the raw weather ones take a date range.
	if err := flags.ValidateMaintenance(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Source-flag mutual exclusivity.
	if flags.PGEWebOnly && flags.EnphaseAPIOnly {
		fmt.Fprintln(os.Stderr, "Error: --pge-web-only and --enphase-api-only are mutually exclusive")
		os.Exit(1)
	}

	// Source flags require --start-date.
	if (flags.PGEWebOnly || flags.EnphaseAPIOnly) && flags.StartDate == "" {
		fmt.Fprintln(os.Stderr, "Error: --pge-web-only and --enphase-api-only require --start-date")
		os.Exit(1)
	}

	// --end-date is only meaningful with --start-date, except for the raw weather
	// commands, where either bound of the optional range may be given alone.
	if flags.EndDate != "" && flags.StartDate == "" && !flags.RawRefetchWeather && !flags.RawApplyWeather {
		fmt.Fprintln(os.Stderr, "Error: --end-date requires --start-date")
		os.Exit(1)
	}

	// Range mode is incompatible with true-up/init.
	if flags.StartDate != "" {
		switch {
		case flags.TrueUp != "":
			fmt.Fprintln(os.Stderr, "Error: --start-date cannot be combined with --true-up")
			os.Exit(1)
		case flags.Init:
			fmt.Fprintln(os.Stderr, "Error: --start-date cannot be combined with --init")
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

	// --raw-import-cache: copy raw Enphase responses from cache/ into raw/; offline.
	if flags.RawImportCache {
		res, err := rawstore.ImportEnphaseCache(cache.GetCacheDir())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: raw import failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Raw import complete: %d imported, %d already present, %d cache files skipped (not Enphase responses). Archive: %s\n",
			res.Imported, res.Existing, res.Skipped, filepath.Join(rawDir, "enphase"))
		return
	}

	// --verify-pge-raw: offline integrity check of raw/pge against history.
	if flags.VerifyPGERaw {
		res, err := pge.VerifyPGERaw(filepath.Join(rawDir, "pge"), app.HistoryDir, pgeTimezone(flags.ConfigFile), os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: PG&E raw verification failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("PG&E raw verification: %d archive(s) re-derived, %d day(s) compared: %d identical, %d differ, %d missing from history.\n",
			res.Archives, res.Days, res.Identical, len(res.Differ), len(res.NoRecord))
		if !res.OK() {
			os.Exit(1)
		}
		fmt.Println("OK")
		return
	}

	// --verify-enphase-raw: offline integrity check of raw/enphase against history.
	if flags.VerifyEnphaseRaw {
		res, err := app.VerifyEnphaseRaw(app.HistoryDir, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Enphase raw verification failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Enphase raw verification: %d day(s) examined: %d reproduced from raw, %d mismatched value(s), %d with missing raw.\n",
			res.Days, res.Complete, len(res.Mismatches), len(res.NoRaw))
		if !res.OK() {
			os.Exit(1)
		}
		fmt.Println("OK")
		return
	}

	// --verify-weather-raw: offline integrity check of raw/weather against history.
	if flags.VerifyWeatherRaw {
		os.Exit(runVerifyWeatherRaw(flags))
	}

	// --raw-apply-weather: rebuild record weather from archived payloads; offline.
	if flags.RawApplyWeather {
		os.Exit(runRawApplyWeather(flags))
	}

	// --raw-refetch-weather: re-fetch past weather to archive raw payloads. Needs
	// no Enphase credentials, only the location cached by --init.
	if flags.RawRefetchWeather {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		os.Exit(runRawRefetchWeather(ctx, flags))
	}

	// --pge-reprocess: rebuild PG&E history from an archived raw download; offline.
	if flags.PGEReprocess != "" {
		tz := pgeTimezone(flags.ConfigFile)
		n, from, to, err := pge.ReprocessRaw(flags.PGEReprocess, app.HistoryDir, app.HistoryDir, tz,
			func(msg string) { fmt.Fprintln(os.Stderr, msg) })
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: PG&E reprocess failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("PG&E reprocess complete: %d day(s) written to %s for %s..%s\n",
			n, app.HistoryDir, from.Format(constants.DateFormat), to.Format(constants.DateFormat))
		return
	}

	// --pge-web-only: standalone Green Button browser pull — needs no Enphase
	// config or credentials, so dispatch before config load.
	if flags.PGEWebOnly {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		tz := pgeTimezone(flags.ConfigFile)
		from, err := time.ParseInLocation(constants.DateFormat, flags.StartDate, tz)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid --start-date %q: use YYYY-MM-DD\n", flags.StartDate)
			os.Exit(1)
		}
		var to time.Time
		if flags.EndDate != "" {
			to, err = time.ParseInLocation(constants.DateFormat, flags.EndDate, tz)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid --end-date %q: use YYYY-MM-DD\n", flags.EndDate)
				os.Exit(1)
			}
		}
		res, err := runPgePull(ctx, tz, from, to)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: PG&E pull failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("PG&E pull complete: %d day(s) written to %s for %s..%s\n",
			res.DaysWritten, app.HistoryDir,
			res.From.Format(constants.DateFormat), res.To.Format(constants.DateFormat))
		return
	}

	// Load configuration
	cfg, err := config.LoadConfig(flags.ConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n\nPlease copy config.yaml.example to config.yaml and fill in your details.\n", err)
		os.Exit(1)
	}
	// --credentials-file overrides credentials_file from config.yaml for every mode.
	if flags.CredentialsFile != "" {
		cfg.CredentialsFile = flags.CredentialsFile
	}

	// --seed-credentials writes credentials.yaml (path from cfg), so run after config load.
	if flags.SeedCredentials {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		if err := runSeedCredentials(ctx, cfg.CredentialsFile, flags.QuotaPrefix); err != nil {
			fmt.Fprintf(os.Stderr, "Credential seeding failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Load API credentials from the separate credentials file. A missing file is
	// not a load error here; ApplyCredentials below does the validation, requiring
	// at least one credential set and filling each set's non-secret OAuth settings
	// (authorization_url, redirect_uri) from the shared api: block in config.yaml.
	creds, err := config.LoadCredentials(cfg.CredentialsFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "Failed to load credentials: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.ApplyCredentials(creds); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load credentials: %v\n\nPlease copy credentials.yaml.example to credentials.yaml and fill in your details (run --update-refresh-tokens for the refresh token).\n", err)
		os.Exit(1)
	}

	// Build the credential pool once: it is shared by the report path (so 429
	// cooldown state survives across Continuous Mode ticks), --init, and --update-refresh-tokens.
	pool := credentials.NewPool(cfg.Credentials)

	// Handle --refresh-quota: out-of-band portal resync of monthly usage baselines.
	if flags.RefreshQuota {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		if err := refreshPortalQuota(ctx, pool, flags.QuotaPrefix); err != nil {
			fmt.Fprintf(os.Stderr, "Quota refresh failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Handle --update-refresh-tokens (use signal context so Ctrl+C cancels token exchange)
	if flags.UpdateRefreshTokens {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		if err := updateAllRefreshTokens(ctx, pool, cfg.CredentialsFile); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	}

	// Handle --init: location cache, monthly quota baseline, and weather legend.
	if flags.Init {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		if err := runInitialize(ctx, pool, flags.Force, flags.QuotaPrefix); err != nil {
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
	// coordinates and seeds monthly quota; without them weather enrichment and
	// quota-aware key rotation are unavailable.
	loc := location.NewResolver()
	if _, ok := loc.CachedPrimaryCoordinates(); !ok {
		fmt.Fprintln(os.Stderr, "enphase-monitor: not initialized — run `enphase-monitor --init` first.")
		os.Exit(1)
	}
	if !pool.HasMonthlyBaseline(flags.QuotaPrefix) {
		fmt.Fprintln(os.Stderr, "enphase-monitor: monthly API quota not initialized — run `enphase-monitor --init` or `enphase-monitor --refresh-quota`.")
		os.Exit(1)
	}

	// --verify-enphase-archive: one-day live check of raw archiving; never writes history.
	if flags.VerifyEnphaseArchive != "" {
		day, err := time.ParseInLocation(constants.DateFormat, flags.VerifyEnphaseArchive, reportTZ)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid --verify-enphase-archive %q: use YYYY-MM-DD\n", flags.VerifyEnphaseArchive)
			os.Exit(1)
		}
		app.ConfigureModes(false /* noCache */, flags.Debug)
		rc := app.RunConfig{Agg: agg, Pool: pool, Disp: disp, Cfg: cfg, ReportTZ: reportTZ, Debug: flags.Debug}
		res, err := app.VerifyEnphaseArchive(ctx, rc, day, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Enphase archive check failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Enphase archive check for %s: %d response(s) archived this run.\n", flags.VerifyEnphaseArchive, res.Archived)
		for _, p := range res.Problems {
			fmt.Printf("✗ %s\n", p)
		}
		if len(res.Problems) > 0 {
			os.Exit(1)
		}
		fmt.Println("OK: archived intact and stored record reproduced from the fresh fetch.")
		return
	}

	// Range pull: --start-date with an optional source flag.
	// --pge-web-only was already dispatched early; here we handle:
	//   (no source flag)     → PG&E web pull, then Enphase API backfill
	//   --enphase-api-only   → Enphase API backfill only
	if flags.StartDate != "" {
		fromDate, err := time.ParseInLocation(constants.DateFormat, flags.StartDate, reportTZ)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid --start-date %q: use YYYY-MM-DD\n", flags.StartDate)
			os.Exit(1)
		}
		var endDate time.Time
		if flags.EndDate != "" {
			endDate, err = time.ParseInLocation(constants.DateFormat, flags.EndDate, reportTZ)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid --end-date %q: use YYYY-MM-DD\n", flags.EndDate)
				os.Exit(1)
			}
		}

		// PG&E web pull (skipped when --enphase-api-only). When both sources are
		// active, use PG&E's most recent available day as the Enphase end date so
		// both datasets stay aligned — PG&E data often lags Enphase by a day or two.
		if !flags.EnphaseAPIOnly {
			pgeRes, err := runPgePull(ctx, reportTZ, fromDate, endDate)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: PG&E pull failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("PG&E pull complete: %d day(s) written to %s for %s..%s\n",
				pgeRes.DaysWritten, app.HistoryDir,
				pgeRes.From.Format(constants.DateFormat), pgeRes.To.Format(constants.DateFormat))
			if !pgeRes.LastDay.IsZero() {
				endDate = pgeRes.LastDay
			} else if pgeRes.Clamped {
				endDate = pgeRes.To // no PG&E data returned, but still honor the one-year cap
			}
		}

		// Enphase API backfill. noCache stays false: RunBackfill always runs live.
		app.ConfigureModes(false /* noCache */, flags.Debug)
		printDebugStartup(flags.Debug, pool, reportTZ)
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

	// --true-up takes precedence over --date; handle it early and exit.
	if flags.TrueUp != "" {
		app.ConfigureModes(flags.NoCache, flags.Debug)
		printDebugStartup(flags.Debug, pool, reportTZ)
		rc := app.RunConfig{
			Agg:      agg,
			Pool:     pool,
			Disp:     disp,
			Cfg:      cfg,
			ReportTZ: reportTZ,
			Debug:    flags.Debug,
		}
		if err := app.RunTrueUp(ctx, rc, flags.TrueUp); err != nil {
			showRunError(disp, err)
			os.Exit(1)
		}
		return
	}

	// Parse test date (returns date and query mode)
	parsedInput, err := app.ParseTestDate(flags.Date, reportTZ)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	testDateParsed := parsedInput.Date
	queryMode := parsedInput.QueryMode

	app.ConfigureModes(flags.NoCache, flags.Debug)
	printDebugStartup(flags.Debug, pool, reportTZ)

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

	if err := app.RunOnce(ctx, rc); err != nil {
		showRunError(disp, err)
		os.Exit(1)
	}
}

// runInitialize resolves system location, seeds or refreshes monthly API quota
// from the developer portal, and writes the weather-code legend.
func runInitialize(ctx context.Context, pool *credentials.Pool, force bool, namePrefix string) error {
	if err := initializeLocation(ctx, pool.First(), pool, force); err != nil {
		return err
	}
	if force || !pool.HasMonthlyBaseline(namePrefix) {
		fmt.Fprintln(os.Stderr, "Seeding monthly API quota from the developer portal…")
		return refreshPortalQuota(ctx, pool, namePrefix)
	}
	fmt.Println("Monthly quota baseline already set for this month (use --refresh-quota or --init --force to re-sync from the portal).")
	return nil
}

// runSeedCredentials scrapes application identity fields from the developer
// portal and merges them into credentials.yaml.
func runSeedCredentials(ctx context.Context, credentialsFile, namePrefix string) error {
	status := func(msg string) { fmt.Fprintln(os.Stderr, msg) }
	printed := false
	progress := func(done, total int, name string) {
		printed = true
		fmt.Fprintf(os.Stderr, "\r\033[K[%d/%d] %s", done, total, name)
	}
	updated, added, scanned, err := enphase.SeedCredentials(ctx, credentialsFile, namePrefix, status, progress)
	if printed {
		fmt.Fprintln(os.Stderr)
	}
	if err != nil {
		return err
	}
	fmt.Printf("Seeded %s: %d updated, %d added (%d apps scanned).\n", credentialsFile, updated, added, scanned)
	if added > 0 {
		fmt.Println("Next: obtain refresh tokens for the new entries with")
		fmt.Println("  ./enphase-monitor --update-refresh-tokens")
	}
	return nil
}

// pgeProfileDir is where the persistent Chrome profile for the PG&E browser pull
// lives, so a sign-in (including MFA) is reused across runs. It is kept under the
// user's home; falls back to a working-directory dotdir if home is unavailable.
func pgeProfileDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".enphase-monitor", "pge-chrome")
	}
	return ".pge-chrome-profile"
}

// pgeTimezone loads the app timezone from config, falling back to
// America/Los_Angeles (PG&E serves California) if the config is absent or
// the timezone string is invalid, or UTC if the zone database is unavailable.
func pgeTimezone(configFile string) *time.Location {
	if cfg, err := config.LoadConfig(configFile); err == nil {
		if loc, err := timezone.LoadTimezone(cfg.Timezone); err == nil {
			return loc
		}
	}
	if loc, err := time.LoadLocation("America/Los_Angeles"); err == nil {
		return loc
	}
	return time.UTC
}

// runVerifyWeatherRaw checks raw/weather against the manifest and against the
// weather stored in history/, writing nothing, and returns the exit code.
func runVerifyWeatherRaw(flags *cli.Flags) int {
	tz := pgeTimezone(flags.ConfigFile)
	rep, err := rawstore.VerifySource("weather")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: weather raw verification failed: %v\n", err)
		return 1
	}
	rawstore.PrintReport(os.Stdout, "weather", rep)
	dates, err := app.HistoryDates(app.HistoryDir, time.Time{}, time.Time{})
	if err != nil || len(dates) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no history/enphase-*.json days to compare (%v)\n", err)
		return 1
	}
	res := app.ApplyRawWeather(dates, app.HistoryDir, filepath.Join(rawDir, "weather"), tz, os.Stdout, true)
	fmt.Printf("Weather raw verification: %d day(s) match their raw payload, %d differ, %d without a usable payload.\n",
		res.Unchanged, len(res.Updated), len(res.Failed))
	if !rep.OK() || len(res.Updated) > 0 || len(res.Failed) > 0 {
		return 1
	}
	fmt.Println("OK")
	return 0
}

// runRawApplyWeather rewrites each history record's weather from the newest
// archived payload under raw/weather and returns the process exit code.
func runRawApplyWeather(flags *cli.Flags) int {
	tz := pgeTimezone(flags.ConfigFile)
	var from, to time.Time
	var err error
	if flags.StartDate != "" {
		if from, err = time.ParseInLocation(constants.DateFormat, flags.StartDate, tz); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid --start-date %q: use YYYY-MM-DD\n", flags.StartDate)
			return 1
		}
	}
	if flags.EndDate != "" {
		if to, err = time.ParseInLocation(constants.DateFormat, flags.EndDate, tz); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid --end-date %q: use YYYY-MM-DD\n", flags.EndDate)
			return 1
		}
	}
	dates, err := app.HistoryDates(app.HistoryDir, from, to)
	if err != nil || len(dates) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no history/enphase-*.json days in range (%v)\n", err)
		return 1
	}
	res := app.ApplyRawWeather(dates, app.HistoryDir, filepath.Join(rawDir, "weather"), tz, os.Stdout, false)
	fmt.Printf("Weather apply complete: %d updated, %d already matched, %d failed.\n", len(res.Updated), res.Unchanged, len(res.Failed))
	if len(res.Failed) > 0 {
		return 1
	}
	return 0
}

// runRawRefetchWeather re-fetches weather for every day in history/ (or the
// --start-date..--end-date range) with the cache bypassed so each Open-Meteo
// payload is archived under raw/weather, and returns the process exit code.
func runRawRefetchWeather(ctx context.Context, flags *cli.Flags) int {
	tz := pgeTimezone(flags.ConfigFile)
	var from, to time.Time
	var err error
	if flags.StartDate != "" {
		if from, err = time.ParseInLocation(constants.DateFormat, flags.StartDate, tz); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid --start-date %q: use YYYY-MM-DD\n", flags.StartDate)
			return 1
		}
	}
	if flags.EndDate != "" {
		if to, err = time.ParseInLocation(constants.DateFormat, flags.EndDate, tz); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid --end-date %q: use YYYY-MM-DD\n", flags.EndDate)
			return 1
		}
	}
	coords, ok := location.NewResolver().CachedPrimaryCoordinates()
	if !ok {
		fmt.Fprintln(os.Stderr, "Error: no cached location — run `enphase-monitor --init` once first.")
		return 1
	}
	dates, err := app.HistoryDates(app.HistoryDir, from, to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: listing history: %v\n", err)
		return 1
	}
	if len(dates) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no history/enphase-*.json days in range")
		return 1
	}
	wc := weather.NewClient(cache.GetCacheDir())
	wc.BypassCache = true
	fmt.Printf("Re-fetching weather for %d day(s), %s..%s\n", len(dates), dates[0], dates[len(dates)-1])
	res := app.RefetchWeatherRaw(ctx, wc, coords, dates, app.HistoryDir, tz, os.Stdout)
	fmt.Printf("Weather refetch complete: %d archived to %s, %d failed, %d differ from history records.\n",
		res.Fetched, filepath.Join(rawDir, "weather"), len(res.Failed), len(res.Differ))
	if len(res.Failed) > 0 {
		return 1
	}
	return 0
}

// runPgePull drives the PG&E browser pull for [from, to]: sign in,
// auto-download the Green Button XML, and write one history record per day.
// Zero from/to values are filled with defaults by BrowserPull (end = yesterday,
// start = 30 days prior). It returns the pull result so the caller can read
// LastDay when aligning the Enphase backfill end date to PG&E's most recent
// available data.
func runPgePull(ctx context.Context, tz *time.Location, from, to time.Time) (*pge.BrowserPullResult, error) {
	return pge.BrowserPull(ctx, pge.BrowserPullOptions{
		ProfileDir: pgeProfileDir(),
		HistoryDir: app.HistoryDir,
		CompareDir: app.HistoryDir,
		RawDir:     tmpDir,
		ArchiveDir: filepath.Join(rawDir, "pge"),
		From:       from,
		To:         to,
		TZ:         tz,
		Notify:     func(msg string) { fmt.Fprintln(os.Stderr, msg) },
	})
}

// refreshPortalQuota reads each application's monthly hit total from the Enphase
// developer portal and writes the authoritative counts into cache/monthly-quota.json.
// Live API calls after this baseline increment the counts via RecordAPICall.
func refreshPortalQuota(ctx context.Context, pool *credentials.Pool, namePrefix string) error {
	names := pool.NamesWithPrefix(namePrefix)
	if len(names) == 0 {
		return fmt.Errorf("no credentials match name prefix %q", namePrefix)
	}

	notify := func(msg string) { fmt.Fprintln(os.Stderr, msg) }
	printed := false
	progress := func(done, total int, name string) {
		printed = true
		fmt.Fprintf(os.Stderr, "\r\033[K[%d/%d] %s", done, total, name)
	}
	stats, err := enphase.FetchMonthlyHits(ctx, names, time.Now(), notify, progress)
	if printed {
		fmt.Fprintln(os.Stderr)
	}
	if err != nil {
		return err
	}

	usage := make(map[string]int, len(stats))
	for _, s := range stats {
		usage[s.Name] = s.Used
	}
	pool.ApplyPortalMonthlyUsage(usage)

	fmt.Println("Updated cache/monthly-quota.json.")
	return nil
}

// showRunError prints a fatal run error with context for rate-limit and monthly-quota cases.
func showRunError(disp *display.Display, err error) {
	if constants.IsRateLimitError(err) {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please wait %d seconds before rerunning the program.\n", constants.APIBudgetWindowSeconds)
		return
	}
	if constants.IsPoolMonthlyQuotaExhaustedError(err) {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return
	}
	disp.ShowError(err)
}

// printDebugStartup prints the monthly quota status when debug mode is on.
func printDebugStartup(debug bool, pool *credentials.Pool, reportTZ *time.Location) {
	if !debug || pool == nil {
		return
	}
	now := time.Now().In(reportTZ)
	fmt.Fprintf(os.Stderr, "[DEBUG] --- startup ---\n")
	fmt.Fprintf(os.Stderr, "[DEBUG] current time : %s\n", now.Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", pool.QuotaSummary())
	fmt.Fprintf(os.Stderr, "[DEBUG] ---\n")
}

// initializeLocation resolves the systems' coordinates (one /systems call) and
// caches them for weather reporting. Run by --init, out of band from reports,
// so it never competes with the API Budget on a live reporting run. Returns an error
// (unlike the report path's best-effort enrichment) so the user knows whether
// initialization succeeded.
func initializeLocation(ctx context.Context, cred *config.APIConfig, pool *credentials.Pool, force bool) error {
	if cred == nil {
		return errors.New("API configuration is required")
	}
	token, err := oauth.GetAccessToken(ctx, cred)
	if err != nil {
		return fmt.Errorf("could not obtain access token: %w", err)
	}
	resolver := location.NewResolver()
	type resolveFn func(context.Context, string, string, api.BudgetTracker, string) ([]location.SystemLocation, error)
	var resolve resolveFn = resolver.SystemLocations
	if force {
		resolve = resolver.RefreshSystemLocations
	}
	locs, err := resolve(ctx, cred.Key, token, pool, cred.Name)
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

// updateAllRefreshTokens re-authorizes every credential in the credentials file
// in turn, driving one browser session that approves each app's consent automatically —
// the user logs in once and never clicks "Allow Access". It attempts each
// credential even if an earlier one failed, then reports which failed; a Ctrl+C
// (context cancellation) aborts the rest.
func updateAllRefreshTokens(ctx context.Context, pool *credentials.Pool, credentialsFile string) error {
	names := pool.Names()

	authorizer := oauth.NewBrowserAuthorizer(ctx)
	defer authorizer.Close()
	fmt.Println("A Chrome window will open. Log in once — each app is approved automatically, no clicking required.")

	// status writes a single self-clearing line (\r + ANSI clear-to-EOL) so the
	// per-credential progress updates in place instead of stacking up.
	status := func(format string, a ...any) {
		fmt.Fprintf(os.Stderr, "\r\033[K"+format, a...)
	}

	var failed []string
	for i, name := range names {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr)
			return ctx.Err()
		}
		cred, _ := pool.ByName(name)
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

	if len(failed) > 0 {
		return fmt.Errorf("authorization failed for %d of %d credential(s): %s", len(failed), len(names), strings.Join(failed, ", "))
	}
	fmt.Printf("Done: authorized %d credential(s).\n", len(names))
	return nil
}
