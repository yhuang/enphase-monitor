# Enphase Monitor - Architecture Guide

This document provides a comprehensive overview of the codebase architecture,
designed to help new engineers learn Go development patterns and best practices.

> **📚 New to Go?** Before reading this guide, check out **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** for detailed explanations of intermediate Go concepts used throughout this codebase.

> **🎯 Learning Path**: 
> 1. Start with [GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md) to understand Go concepts
> 2. Read this guide to understand the system architecture
> 3. See [GO_CONCEPTS.md](GO_CONCEPTS.md#channels-and-signals) for channels, signals, and concurrency patterns

## Table of Contents

1. [Project Overview](#project-overview)
2. [Module Structure](#module-structure)
3. [Execution Flow](#execution-flow)
4. [Data Flow Diagram](#data-flow-diagram)
5. [Raw Archive and Verification](#raw-archive-and-verification)
6. [PG&E Feed Decoding](#pge-feed-decoding)
7. [API Budget Checks](#api-budget-checks)
8. [Key Go Patterns Used](#key-go-patterns-used)
9. [File Descriptions](#file-descriptions)
10. [Next Steps for Learning](#next-steps-for-learning)

---

## Project Overview

The **enphase-monitor** is a CLI application that monitors energy metrics from one or more Enphase solar Systems at a Site via the Enphase Enlighten Cloud API v4.

### Core Capabilities

- **Multi-System Monitoring**: Query and aggregate data from multiple independent Systems at a Site
- **Cloud API Integration**: Uses Enphase Enlighten Cloud API v4 exclusively (no local network access required)
- **Intelligent Caching**: Disk-based response caching to stay within the API Budget (1000 calls/month per key, spread across a credential pool)
- **Past Period Queries**: Query any past date with `--date` flag (auto-runs once since Past Period data is immutable)
- **True-Up Report**: Energy metrics across a full utility True-Up Period with `--true-up`
- **Historical Backfill**: Per-day Enphase + weather records (`--start-date`) and PG&E utility-meter records (Green Button download) written to `history/`
- **Raw Source Archive**: Every source response kept untouched under `raw/` with a SHA-256 manifest, so `history/` can be re-derived and audited offline at any time
- **Integrity Verification**: One offline `--verify-*-raw` command per source, plus a single live Enphase archiving check
- **Color Customization**: Customize terminal output colors via YAML configuration

### Tech Stack

- **Language**: Go 1.26 (see `go.mod`)
- **Configuration**: YAML (`gopkg.in/yaml.v3`)
- **HTTP Client**: Standard library `net/http` (no frameworks)
- **Authentication**: OAuth 2.0 with Bearer tokens
- **Caching**: File-based (JSON responses stored on disk)
- **Browser automation**: `chromedp` (headed Chrome for the Enphase developer portal and the PG&E Green Button download)
- **Raw archive**: files under `raw/<source>/` plus an append-only `raw/manifest.jsonl` (standard library only)

---

## Module Structure

```
enphase-monitor/
├── main.go                                # Entry point (~940 lines) - orchestration only
├── internal/
│   ├── aggregator/                        # Multi-system data aggregation
│   │   ├── types.go                       # Metric data structures (AggregatedMetrics, SystemMetrics)
│   │   ├── aggregator.go                  # Aggregation logic with dependency injection
│   │   ├── aggregator_test.go             # Aggregator tests with mock clients
│   │   └── aggregator_bench_test.go       # Benchmark tests
│   ├── api/                               # HTTP client for Cloud API v4
│   │   ├── client.go                      # Enlighten Cloud API client; archives each live response to raw/enphase (rawMeta/rawName) and exposes DayKWh for offline re-derivation
│   │   ├── types.go                       # API request/response types
│   │   ├── client_test.go                 # API client unit tests
│   │   ├── client_caching_test.go         # Characterization tests for makeCachedAPIRequest fallback branches (no-cache mode, 429/503/network-error propagation)
│   │   ├── client_functional_test.go      # Functional tests with mock HTTP servers
│   │   ├── client_lifetime_test.go        # Lifetime Data tests (Month, Year, True-Up Mode queries)
│   │   └── testmain_test.go               # TestMain: redirects cache I/O to temp dir for all api tests
│   ├── app/                               # Application execution logic
│   │   ├── setup.go                       # App initialization & configuration
│   │   ├── setup_test.go                  # Setup tests
│   │   ├── runner.go                      # RunOnce execution mode
│   │   ├── runner_test.go                 # Runner tests
│   │   ├── trueup.go                      # True-Up Mode: single-batch Lifetime Data query, True-Up Window end-date logic, report conversion
│   │   ├── trueup_test.go                 # True-up Window end-date logic (trueUpWindowEnd) and report conversion tests
│   │   ├── backfill.go                    # Backfill Mode: live per-day fetch over a date range into history/
│   │   ├── backfill_test.go               # Backfill range/skip/overwrite tests
│   │   ├── weather.go                     # Best-effort weather enrichment for Day-Mode reports
│   │   ├── weather_test.go                # Weather enrichment tests
│   │   ├── rawweather.go                  # HistoryDates, RefetchWeatherRaw (--raw-refetch-weather), ApplyRawWeather (--raw-apply-weather) + history/.weather-provenance.json sidecar
│   │   ├── rawweather_test.go             # Raw-weather, provenance, and offline Enphase-verification tests
│   │   ├── verifyenphaseraw.go            # VerifyEnphaseRaw (--verify-enphase-raw): re-derive each history day from raw/enphase bodies
│   │   └── verifyenphase.go               # VerifyEnphaseArchive (--verify-enphase-archive): one-day live archiving check
│   ├── browser/                           # Headed Chrome launcher (chromedp) for portal automation
│   │   ├── chrome.go                      # LaunchHeaded (disposable profile) + LaunchHeadedWithProfile (persistent profile)
│   │   └── chrome_test.go                 # Chrome launcher tests
│   ├── cache/                             # Disk-based response caching
│   │   ├── cache.go                       # Cache implementation (URL normalization, Past Period handling, endpoint/system tagging)
│   │   ├── cache_test.go                  # Cache state management tests (CacheDisabled, ResetState)
│   │   ├── cache_functions_test.go        # Core caching tests (URL normalization, save/load, ClearCacheForDate)
│   │   ├── cli.go                         # Cache inspection utilities
│   │   └── cli_test.go                    # CLI utilities tests
│   ├── cli/                               # Command-line interface
│   │   ├── flags.go                       # CLI flag parsing, grouped --help (printUsage), MaintenanceCommands/ValidateMaintenance
│   │   ├── flags_test.go                  # Flag parsing tests
│   │   ├── cache_commands.go              # Cache management commands
│   │   └── cache_commands_test.go         # Cache commands tests
│   ├── config/                            # Configuration types
│   │   ├── config.go                      # YAML loading & validation (uses type aliases)
│   │   ├── config_test.go                 # Configuration tests
│   │   ├── credentials.go                 # Loads/validates the credentials: pool; seeds & rewrites credentials.yaml
│   │   └── credentials_test.go            # Credentials loading/validation/merge tests
│   ├── constants/                         # Centralized constants
│   │   ├── constants.go                   # Application-wide constants
│   │   └── constants_test.go              # Constants tests
│   ├── credentials/                       # Credential pool: spread + 429 failover + monthly quota
│   │   ├── pool.go                        # Round-robin assignment, cooldown, failover
│   │   ├── pool_test.go                   # Pool selection/failover tests
│   │   ├── quota.go                       # Per-credential monthly API budget (monthly-quota.json)
│   │   ├── quota_test.go                  # Quota counting, month-rollover, and budget tests
│   │   └── quota_portal_test.go           # Portal-seeded monthly baseline tests
│   ├── display/                           # Terminal output formatting
│   │   ├── display.go                     # Display with io.Writer injection for testability
│   │   └── display_test.go                # Display output tests
│   ├── enphase/                           # Developer-portal scraping (no management API)
│   │   ├── login.go                       # Headed-Chrome portal login + session cookie capture
│   │   ├── portal.go                      # Scrape app name/key/client_id/client_secret
│   │   ├── seed.go                        # --seed-credentials: scrape + merge into credentials.yaml
│   │   ├── stats.go                       # Monthly hit totals from the portal stats page
│   │   ├── stats_browser.go               # chromedp driver for the stats page UI
│   │   ├── login_test.go                  # Login/cookie-capture tests
│   │   ├── portal_test.go                 # Portal HTML/JSON scrape tests
│   │   └── stats_test.go                  # Stats parsing/date-range tests
│   ├── geocode/                           # ZIP/postal code → coordinates (Zippopotam.us)
│   │   ├── geocode.go                     # ZIP lookup for weather geolocation
│   │   └── geocode_test.go                # Geocode tests
│   ├── history/                           # Per-day energy+weather and utility-meter JSON records (history/)
│   │   ├── history.go                     # Dataset type (Enphase/PGE sentinels), DayRecord/WeatherRec schema, FromMetrics, WriteRecord, WriteIndex
│   │   └── history_test.go                # History mapping, write, index, and Dataset tests
│   ├── location/                          # Resolve & cache systems' coordinates (--init)
│   │   ├── location.go                    # Location resolver with disk cache
│   │   └── location_test.go               # Location resolver tests
│   ├── oauth/                             # OAuth 2.0 authentication (Enphase developer-plan)
│   │   ├── oauth.go                       # Token management & refresh
│   │   ├── browser.go                     # Browser-driven OAuth authorization (auto-approves consent)
│   │   ├── oauth_test.go                  # Basic unit tests
│   │   ├── browser_test.go                # Browser-OAuth flow tests
│   │   ├── oauth_functional_test.go       # Integration tests with mock servers
│   │   └── oauth_edge_cases_test.go       # Edge case and error path tests
│   ├── pge/                               # PG&E Green Button integration: browser download, ESPI decoding, sanity checks, raw-archive verification
│   │   ├── doc.go                         # Package documentation: the PG&E data flow and the commands that reach it
│   │   ├── browserpull.go                 # BrowserPull: Green Button download via headed Chrome session (range capped at one year); archives the original ZIP; ingestDownload and ReprocessRaw (--pge-reprocess)
│   │   ├── espi.go                        # ParseReadings (register selection, Wh scale), AggregateReadingsByDay (wall-clock decoding, DST gap handling) → DayUsage intervals
│   │   ├── sanity.go                      # CheckDays: write-time warnings (negative import, interval count vs real local day, net vs Enphase)
│   │   ├── verify.go                      # VerifyPGERaw (--verify-pge-raw): hash check + re-derive raw/pge archives and diff against history
│   │   ├── history.go                     # WriteHistory: aggregate DayUsage → pge-YYYY-MM-DD.json records
│   │   ├── download.go                    # downloadGreenButton: drive the export form (autoform) and capture the new download by watching the download directory
│   │   ├── login.go                       # openLoggedInSession: headed Chrome on a persistent profile; blocks until the user has signed in to PG&E
│   │   ├── autoform.go                    # formDriver: auto-drive the Green Button export form inside PG&E's Visualforce iframe
│   │   ├── autoform_test.go               # Frame lookup and formDriver debug-buffer tests
│   │   ├── download_test.go               # Download-directory snapshot and completed-file detection tests
│   │   ├── browserpull_test.go            # One-year export-range clamp and raw-archive naming tests
│   │   ├── espi_test.go                   # ESPI parser/aggregation tests against real redacted feeds (register selection, clock decoding, DST days), CheckDays
│   │   ├── verify_test.go                 # VerifyPGERaw round-trip: clean, hand-edited record, tampered raw file; archive ordering by fetch time
│   │   └── testdata/                      # Real PG&E exports with identifiers removed: 2025-11-08, 2026-03-08 (spring-forward), 2025-11-02 (fall-back)
│   ├── parser/                            # JSON telemetry parsing
│   │   ├── parser.go                      # Response parsing utilities
│   │   ├── parser_test.go                 # Parser tests
│   │   └── parser_bench_test.go           # Benchmark tests
│   ├── rawstore/                          # Raw source archive: raw/<source>/ files + raw/manifest.jsonl (SHA-256)
│   │   ├── rawstore.go                    # Save, Record, RedactURL; disabled unless Dir is set
│   │   ├── importcache.go                 # ImportEnphaseCache: copy raw bodies from cache/ into raw/enphase (--raw-import-cache)
│   │   ├── verify.go                      # VerifySource, Entries/EntriesSince, VerifyEntry, PrintReport
│   │   └── rawstore_test.go               # Save/manifest, import idempotence, verification tests
│   ├── timezone/                          # Timezone handling
│   │   ├── timezone.go                    # Timezone utilities
│   │   └── timezone_test.go               # Timezone tests
│   ├── types/                             # Shared type definitions
│   │   └── types.go                       # SystemConfig, APIConfig (breaks circular deps)
│   ├── urlbuilder/                        # API URL construction
│   │   ├── urlbuilder.go                  # URL building helpers
│   │   └── urlbuilder_test.go             # URL builder tests
│   └── weather/                           # Open-Meteo daily/current weather client
│       ├── weather.go                     # DailyWeather/CurrentWeather fetch + WMO code mapping; archives each live payload to raw/weather; ParseDailyPayload; BypassCache
│       └── weather_test.go                # Weather client tests
│
├── docs/                                  # Additional documentation
│   ├── ARCHITECTURE.md                    # Architecture and design decisions
│   ├── GO_BEST_PRACTICES.md               # Go best practices guide
│   ├── GO_CONCEPTS.md                     # Go concepts reference
│   ├── OAUTH_SETUP.md                     # OAuth setup guide
│   ├── TESTING.md                         # Testing patterns and guidelines
│   └── adr/                               # Architecture Decision Records
│       ├── 0001-true-up-window-end-date.md # ADR for the True-Up Window end-date rule
│       └── 0002-init-guard-requires-weather-init.md # ADR: report modes require --init (weather)
├── CONTEXT.md                             # Domain glossary (Site, System, Query Mode, Net Flow, API Budget, ...)
├── test-data/                             # Recorded fixtures for the geocode and location tests
├── config.yaml                            # User configuration: non-secret settings (not in git)
├── config.yaml.example                    # Configuration template
├── credentials.yaml                       # API secrets: credentials: pool (not in git)
├── credentials.yaml.example               # Credentials template
├── cache/                                 # Cached API responses (created at runtime; clearable)
├── raw/                                   # Untouched source responses + manifest.jsonl (created at runtime; gitignored — holds account data)
│   ├── enphase/  weather/  pge/           # One directory per source; files are never overwritten
│   └── manifest.jsonl                     # One line per file: fetched_at, source, file, sha256, bytes, request, note
├── history/                               # Per-day energy+weather and utility-meter JSON records (created at runtime) + dotfile sidecars (.<prefix>-index.json, .weather-provenance.json)
├── tmp/                                   # Scratch downloads (PG&E ZIP/XML before extraction); gitignored
└── scripts/                               # Utility scripts
    ├── migrate-history-layout.sh          # One-time migration: renames <date>.json records to enphase-<date>.json
    └── history.py                         # Git history inspection helper
```

### Package Dependency Graph

The `internal/types/` package provides shared type definitions that break circular dependencies:

```
                types ─
               /  |     \
              /   |      \
        config aggregator oauth
              \   |      /
               \  |     /
                 app ──
```

Types defined in `internal/types/types.go`:
- `SystemConfig` - Configuration for a single Enphase system
- `APIConfig` - API credentials and OAuth settings

These types are re-exported as type aliases in `config` and `aggregator` packages for backward compatibility.

---

## Execution Flow

### Primary Path: Report Generation

```
┌─────────────────────────────────────────────────────────────────────┐
│  1. ENTRY POINT (main.go)                                           │
│     └─► cli.ParseFlags() from internal/cli                          │
│     └─► Handle cache commands (internal/cli) or continue            │
│     └─► Maintenance commands (raw archive / reprocess / verify)     │
│         dispatch here and exit (see Raw Archive and Verification)   │
│     └─► Build credentials.Pool from cfg.Credentials                 │
│     └─► If --update-refresh-tokens: signal context; one Chrome       │
│         session authorizes every credential                         │
│         (oauth.AuthorizeViaBrowser); Ctrl+C cancels it             │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│  2. SETUP (internal/app)                                            │
│     └─► config.LoadConfig() reads YAML file                         │
│         └─► app.CreateOAuthAdapter() for token management           │
│             └─► app.SetupDisplay() with colors                      │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                          ┌─────────┴─────────┐
                          │                   │
              --true-up provided?         --date / default
                          │                   │
                          ▼                   ▼
┌──────────────────────────────┐  ┌────────────────────────────────────────────────────────────────────┐
│  3a. TRUE-UP (internal/app)  │  │  3b. EXECUTION (internal/app)                                      │
│  app.RunTrueUp(ctx, ...)     │  │     └─► main creates signal context (SIGINT/SIGTERM), passes ctx   │
│  ├─► Parse start date        │  │     └─► app.RunOnce(ctx, ...) calls aggregator and displays report │
│  ├─► Normalize to month-1    │  └────────────────────────────────────────────────────────────────────┘
│  ├─► GetAggregatedMetrics    │
│  │   (QueryModeTrueUp,       │
│  │   4 metrics/system,       │
│  ├─► buildTrueUpReport()     │
│  └─► ShowTrueUpReport()      │
└──────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│  4. AGGREGATION (internal/aggregator)                                │
│     └─► GetAggregatedMetrics() loops through systems                 │
│         └─► Uses internal/api for HTTP requests                      │
│             └─► Fetches Production, Consumption, Grid Import/Export  │
│                 battery fetched only for today's Day query           │
└──────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│  5. API CALLS (internal/api)                                         │
│     └─► Each call goes through caching layer (internal/cache)        │
│         ├─► Past periods: served from cache (data immutable)         │
│         ├─► Current periods / cache disabled: live call; 429/503     │
│         │   propagate (aggregator fails over to a spare credential)  │
│         └─► Live success: count against the credential's monthly     │
│             quota, save to cache/, archive untouched to raw/enphase  │
└──────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│  6. RESPONSE PARSING (internal/parser)                              │
│     └─► Parse JSON API response data                                │
│         └─► Sum interval values for daily totals                    │
│             └─► Convert Wh to kWh                                   │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│  7. DISPLAY (internal/display)                                      │
│     └─► ShowMetrics() formats output                                │
│         ├─► printHeader() - Query range and timestamp               │
│         ├─► printTodayEnergy() - Combined totals                    │
│         └─► printIndividualSystems() - Per-system breakdown         │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Data Flow Diagram

```
          ┌──────────────────┐   ┌──────────────────────┐
          │   config.yaml    │   │   credentials.yaml   │
          └────────┬─────────┘   └──────────┬───────────┘
                   │                        │
                   ▼                        ▼
┌────────────────────────────────┐ ┌─────────────────────────────────────┐
│  internal/config/LoadConfig()  │ │  internal/config/LoadCredentials()  │
│  Parses YAML: systems, colors, │ │  Parses the credentials: list       │
│  shared api: OAuth settings    │ │  (secrets only)                     │
└────────────────┬───────────────┘ └────────────────┬────────────────────┘
                 │                                  │
                 └───────────┬──────────────────────┘
                             ▼
              ┌──────────────────────────────────────┐
              │   Config.ApplyCredentials()          │
              │   Merges shared api: settings into   │
              │   each credential; validates pool    │
              │   (unique names, required secrets)   │
              └──────────────┬───────────────────────┘
                             │
                             ▼
┌────────────────────────────────────────────────────────────────────┐
│                     Cloud Systems                                  │
│              (system.ID required)                                  │
└────────────────────────────┬───────────────────────────────────────┘
                             │
                             ▼
┌────────────────────────────────────────────────────────────────────┐
│              internal/api/EnlightenCloudClient                     │
│  - OAuth authentication (internal/oauth)                           │
│  - Interval Data endpoints (15-min data, single-day queries)       │
│  - Lifetime Data endpoints (daily totals; Month, Year, True-Up)    │
└────────────────────────────┬───────────────────────────────────────┘
                             │
                             ▼
┌────────────────────────────────────────────────────────────────────┐
│                  internal/cache/Cache Layer                        │
│  - Past Periods: from cache; Current Periods: live, errors propagate│
│  - Monthly API Budget is tracked per credential (internal/credentials)│
│  - Disk-based storage tagged with endpoint + system ID             │
└────────────────────────────┬───────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│              internal/aggregator/AggregatedMetrics                                  │
│   - Sums production, consumption across systems                                     │
│   - Battery tracked per System only (today's live Day Mode query); zero otherwise   │
│   - Tracks cache usage flags (CacheUsed, AllFromCache)                              │
│   - TrueUpReport built from a single lifetime-endpoint batch                        │  
└──────────────────────────┬──────────────────────────────────────────────────────────┘
                           │
                           ▼
┌────────────────────────────────────────────────────────────────────┐
│                  internal/display/Display                          │
│   - ANSI color formatting                                          │
│   - Structured report output                                       │
└────────────────────────────────────────────────────────────────────┘
```

---

## Raw Archive and Verification

`history/` is derived data. To make it checkable at any time, every external response is also kept untouched, and a family of offline commands re-derives `history/` from those copies.

### What is archived, and when

```
  live Enphase call ──► api.finalizeAndCache ───────────► raw/enphase/<system>_<endpoint>_<date>_<ts>.json
  live Open-Meteo call ─► weather.DailyWeather ─────────► raw/weather/daily_<date>_<ts>.json
  PG&E export ZIP ──────► pge.BrowserPull (before extract) ► raw/pge/pge-<from>_<to>[-n].zip
                                      │
                                      └──────────────────► raw/manifest.jsonl  (one line per file)
```

- **Only live fetches archive.** A cache hit makes no request, so nothing is written; backfill is always live (cache disabled), and days skipped as already present add nothing. The Enphase cache already holds the same bodies (it was the source for `--raw-import-cache`), but a cache can be cleared or overwritten and `raw/` cannot be touched by any cache command.
- **Write once.** Files get a UTC timestamp suffix and are created with `O_EXCL`; the PG&E archive appends `-1`, `-2` when a range is pulled again. Nothing in `raw/` is ever overwritten.
- **Opt-in at the package level.** `rawstore.Dir` is empty by default, so tests and library use never write files; `main` sets it to `raw`.
- **Manifest line**: `{fetched_at, source, file, sha256, bytes, request, note?}`. `request` is a redacted description (`system=… endpoint=… date=… url=…` for Enphase, the redacted URL for weather, the export range for PG&E; `key`, `access_token` and `client_secret` query parameters are removed). `note` records provenance for files that were not archived by a live fetch (`imported from cache`, `added retroactively`).

### Maintenance and verification commands

All of these exit after doing their one job and are dispatched in `main()` before any report mode: all but `--verify-enphase-archive` before config and credentials are loaded, and `--verify-enphase-archive` after the config load and init guard, because it needs a credential; `flags.ValidateMaintenance` rejects combining two of them and rejects `--start-date/--end-date` on any but the two raw weather commands (where either bound may be given alone).

| Command | Network | Reads | Writes |
|---|---|---|---|
| `--raw-import-cache` | none | `cache/` | `raw/enphase/`, manifest (idempotent: names derive from system, endpoint, date, fetch time) |
| `--raw-refetch-weather` | Open-Meteo (no key/quota) | `history/` days, cached location | `raw/weather/`, manifest; weather cache bypassed and left untouched; reports days that differ from history |
| `--raw-apply-weather` | none | `raw/weather/` | weather block of changed `history/enphase-*.json` (never energy fields), `history/.weather-provenance.json` |
| `--pge-reprocess <zip>` | none | one `raw/pge` archive (copied to scratch first) | `history/pge-*.json` for the range in the file name |
| `--verify-pge-raw` | none | `raw/pge`, manifest, `history/pge-*.json` | nothing |
| `--verify-weather-raw` | none | `raw/weather`, manifest, `history/enphase-*.json` | nothing |
| `--verify-enphase-raw` | none | `raw/enphase`, manifest, `history/enphase-*.json` | nothing |
| `--verify-enphase-archive <date>` | Enphase API (about 10 calls) | stored record for that day | new `raw/enphase` files + manifest lines, plus the cache entries and monthly-quota count any live call makes; never `history/` |

Each `--verify-*-raw` command does two checks. **Integrity** (`rawstore.VerifySource`): re-hash every manifest line for the source and report altered, missing and unlisted files. **Reproduction**: re-derive history from the raw files with the same code path the live run uses — `pge.ReprocessRaw` into a scratch directory then a byte comparison; `weather.ParseDailyPayload` via `ApplyRawWeather(dryRun)`; `api.DayKWh` per (system, endpoint, date), taking the newest manifest entry for each — and report each day that does not match. They exit non-zero on any finding.

There is deliberately **no umbrella "verify everything" flag**: `--verify-enphase-archive` spends API Budget, and an umbrella would spend it on every routine check. It is the one live command, it checks a single completed past day, and it never writes `history/`.

### Why PG&E processing is re-runnable

`ingestDownload` is the single extract → parse → aggregate → `CheckDays` → `WriteHistory` path, used by `BrowserPull` after archiving and by `ReprocessRaw` on a copy of an archive, so a parser fix is applied to data already collected with `--pge-reprocess` instead of a new pull. `VerifyPGERaw` runs the same path against a scratch directory, so "does history match what the current parser derives from raw" is a byte comparison.

### Weather provenance

Open-Meteo revises recent history, and older records could hold zeros for fields not yet fetched, so `--raw-apply-weather` records per day in `history/.weather-provenance.json` whether the weather was `rebuilt_from_raw` (with the replaced values in `previous`) or already `matches_raw`, with the raw file, its hash and whether radiation is exactly 0.

---

## PG&E Feed Decoding

`internal/pge/espi.go` turns a Green Button electric export into per-day `import_kwh`/`export_kwh`/`net_kwh` plus 15-minute intervals. Four properties of the real feed drive the design (each verified against real exports, and pinned by redacted fixtures in `internal/pge/testdata/`):

1. **Four registers, two of them in the same direction.** A feed carries signed *net* (`flowDirection` 1), *received* (19), *demand* in watts (0, skipped) and *gross delivered* (1). Net and gross have identical metadata, so `selectRegisters` identifies gross by the identity `net = delivered − received` on every interval and fails if it does not hold. Only gross and received are used (summing both `flowDirection` 1 registers doubled import).
2. **Wall-clock timestamps encoded as UTC.** `detectWallClock` accepts the encoding only if the earliest reading sits on a midnight in UTC (local clock) or in the report timezone (true UTC) and errors otherwise; `localStart` converts labels with `time.Date`, mapping a label inside a spring-forward gap to the instant after the gap.
3. **Wh with a misleading multiplier.** PG&E tags Wh values with `powerOfTenMultiplier` 3; values are plain Wh, so the multiplier is ignored and a per-interval magnitude guard (`maxIntervalKWh`) fails the parse if the scale ever looks wrong.
4. **DST days are not 96 intervals.** Spring-forward has 92; fall-back has only 96 of its 100 real slots because the feed omits the repeated hour. `CheckDays` compares each day's count with its real local length and warns, alongside negative import, interval import < 0 with export > 0, and net more than 10% away from Enphase's for the same date.

---

## API Budget Checks

The Enphase Cloud API allows **1000 requests per month per API key** and, server-side, **10 requests per minute per key**. The application enforces the monthly limit itself and treats the per-minute limit as something Enphase signals with HTTP 429. With two systems each making a full day query (4 endpoints, plus a 5th for today's battery), a single run is up to 10 calls, so a run alone can saturate one key's per-minute allowance; the credential pool exists to spread that.

### Where budget is tracked

- **Monthly count per credential** lives in `internal/credentials` (`Pool.RecordAPICall`, persisted in `cache/monthly-quota.json`). The baseline is seeded from the developer portal by `--init` / `--refresh-quota`, incremented on every live call (`api.BudgetTracker`), and resets on the month boundary. `hasMonthlyBudget` is true while a credential is under `constants.MaxRequestsPerMonth`.
- **No per-minute counter.** Enphase's 10/minute limit is not tracked locally: a 429 is handled reactively, by the credential failover below.

### Layer 1: credential selection and failover (`internal/aggregator`, `internal/credentials`)

`GetAggregatedMetrics` first refuses to run when every credential has spent its monthly budget (`Pool.AllMonthlyExhausted`). Each system is then assigned a credential least-used-first (`Pool.ForSystem`). When a call returns `RateLimitError` (HTTP 429), or a token request fails, the credential is put in cooldown (`MarkUnavailable`) and the system retries on a spare (`Failover`); the system only reports a rate-limit error once every credential has been tried.

### Layer 2: per-request decision in `makeCachedAPIRequest` (`internal/api/client.go`)

The decision tree for one URL:

```
Load the cached response for this URL (for Past Periods, also rescan by queried date)
Cache disabled?             YES → live call (--no-cache, Backfill Mode, --verify-enphase-archive)
                                   errors (429, 503, transport) propagate; no stale cache served
Past Period with a cache?   YES → serve the immutable cache; no budget consumed
otherwise (Current Period)  → live call; errors propagate
                              (a 429 reaches the aggregator, which fails over)
Every successful live call: RecordAPICall(credential) → finalizeAndCache
                            → save to cache/ AND archive the untouched body to raw/enphase
```

Key points:
- **Past Periods are immutable**, so a cached Past Period day/month/year is always served from cache and costs nothing. Live calls are made for Current Periods and whenever the cache is disabled.
- **Errors are never masked by stale cache on the live path.** A 429 must reach the aggregator so it can fail over to a spare credential, and a 503 surfaces as an error rather than a guess.
- **Only live calls are archived.** `finalizeAndCache` writes the body to `raw/enphase/` as well as the cache; a cache hit archives nothing (see [Raw Archive and Verification](#raw-archive-and-verification)).
- The 5th (battery) call exists only for today's live Day query (`queryMode == QueryModeDay && testDate.IsZero()`), because the battery endpoint is per-15-minute and the lifetime endpoints that serve past days carry no battery data.

### Observability: `--debug` mode

With `--debug` the application writes to stderr:
- a **startup banner** (`printDebugStartup` in [main.go](../main.go)): the current time and the credential pool's monthly quota summary (`Pool.QuotaSummary`);
- a **per-request trace** (`cache.Debugf` from `makeCachedAPIRequest`): one line per URL saying whether it served an immutable Past Period cache, made a live call, or got a 429, each with the redacted URL (the API key never appears).

Debug mode also suppresses the terminal-clearing escape sequence in `RunOnce` so the trace stays visible after the report. `cache.Debugf` is a no-op when debug mode is off.

### Tests

Budget and fallback behavior is covered by [client_caching_test.go](../internal/api/client_caching_test.go) (characterization tests for each `makeCachedAPIRequest` branch: no-cache mode, Past Period cache hits, and error propagation on 429/503/transport failure) and by the pool tests in [internal/credentials/pool_test.go](../internal/credentials/pool_test.go) and [quota_test.go](../internal/credentials/quota_test.go).

---

## Key Go Patterns Used

### 1. Struct Method Receivers

```go
// internal/api/client.go - Methods bound to struct
// See internal/api/client.go for the EnlightenCloudClient implementation
type EnlightenCloudClient struct {
    baseURL     string         // Base URL for API requests (injectable for testing)
    systemID    string
    systemName  string
    apiKey      string
    accessToken string
    timezone    *time.Location // Timezone for reporting/queries
    httpClient  *http.Client
    cacheUsed   bool           // Tracks if cache was used for the last request
}

func (c *EnlightenCloudClient) GetMetricsFromCloud(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (*LocalMetrics, bool, error) {
    // 'c' is the receiver - access struct fields via c.systemID, etc.
    // ctx is used for request cancellation and timeout handling
    // queryMode specifies the Query Mode (Day, Month, Year, or True-Up)
}
```

**Why:** Go does not have classes, but struct methods provide similar encapsulation.
The receiver acts like `this` or `self` in other languages.

**See also:** [internal/api/client.go](../internal/api/client.go) for the complete implementation.

### 2. Error Wrapping with Context

```go
// oauth.go - Adding context to errors
// See oauth.go lines 150-200 for more error wrapping examples
if err != nil {
    return nil, fmt.Errorf("failed to decode token response: %w", err)
}
```

**Why:** The `%w` verb wraps the original error, preserving the error chain.
Callers can use `errors.Is()` or `errors.As()` to inspect wrapped errors.

**See also:** [internal/oauth/oauth.go](../internal/oauth/oauth.go) and [internal/aggregator/aggregator.go](../internal/aggregator/aggregator.go) for error handling patterns throughout the codebase.

### 3. Constructor Overloading with Defaults

This codebase uses a simplified approach to optional configuration:

```go
// display.go lines 55-59 - Constructor delegates to NewDisplayWithWriter
// colors.MergeWithDefaults is called inside NewDisplayWithWriter (lines 61-74)
func NewDisplayWithColorsAndTimezone(colors config.ColorConfig, tz *time.Location) *Display {
    return NewDisplayWithWriter(colors, tz, os.Stdout)
}
```

**Why:** Timezone is required for proper date/time formatting. Colors are optional and merge with defaults.

**How it works:**
- `NewDisplayWithColorsAndTimezone(defaultColors, reportTZ)` — uses default colors with specified timezone
- `NewDisplayWithColorsAndTimezone(customColors, reportTZ)` — uses custom colors with specified timezone

#### The True Functional Options Pattern (For Reference)

Go's idiomatic "functional options" pattern uses functions that return configuration functions. This codebase does not use it (constructor overloading is simpler for our needs), but here is what it looks like:

```go
// Option is a function that configures Display
type DisplayOption func(*Display)

// Each option returns a configuration function
func WithColors(colors ColorConfig) DisplayOption {
    return func(d *Display) {
        d.colors = colors
    }
}

func WithBorder(enabled bool) DisplayOption {
    return func(d *Display) {
        d.showBorder = enabled
    }
}

// Constructor accepts variadic options
func NewDisplay(opts ...DisplayOption) *Display {
    d := &Display{
        colors:     getDefaultColors(),  // sensible defaults
        showBorder: true,
    }
    for _, opt := range opts {
        opt(d)
    }
    return d
}

// Usage - mix and match any options
display := NewDisplay()                                         // all defaults
display := NewDisplay(WithColors(myColors))                     // custom colors
display := NewDisplay(WithColors(myColors), WithBorder(false))  // multiple options
```

**When to use each approach:**

| Approach | Use When |
|----------|----------|
| Constructor overloading (this codebase) | 1-2 configuration variations |
| True functional options | Many independent optional settings, public API extensibility |

The constructor overloading approach is appropriate here since `Display` only has one optional configuration (colors).

### 4. Interface Satisfaction (Implicit)

```go
// Go interfaces are satisfied implicitly - no "implements" keyword
// Any type with matching methods automatically satisfies the interface

// io.Reader interface requires: Read(p []byte) (n int, err error)
// http.Response.Body satisfies io.Reader automatically
```

**Why:** Enables loose coupling and easy testing with mock implementations.

### 5. Deferred Resource Cleanup

```go
// internal/api/client.go - Guaranteed cleanup even on error
resp, err := c.httpClient.Do(req)
if err != nil {
    return nil, err
}
defer resp.Body.Close()  // Always executes when function returns
```

**Why:** Prevents resource leaks. `defer` executes in LIFO order.

### 6. Channels and Signal Handling (Interruptible Waiting)

`main.go` creates a signal context so that Ctrl+C (SIGINT) and SIGTERM cancel in-flight work:

```go
// main.go - Signal context for graceful shutdown
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()
```

This context is passed to `RunOnce` (and to `oauth.AuthorizeViaBrowser`) so any in-flight HTTP request is cancelled when the process receives a signal.

> **Note:** The `for`/`select` ticker loop that drove periodic re-fetches (`RunContinuous`) has been removed. The channel/select/ticker pattern it used is documented in [GO_CONCEPTS.md](GO_CONCEPTS.md#channels-and-signals) as a Go reference — the pattern itself is valid Go; it is simply no longer exercised by production code.

### 7. JSON Struct Tags

```go
// internal/parser/parser.go - Mapping JSON to Go structs
type TelemetryResponse struct {
    LastReportedAggregateSOC string              `json:"last_reported_aggregate_soc,omitempty"`
    Intervals                []TelemetryInterval `json:"intervals"`
}
```

**Why:** Struct tags tell the JSON encoder/decoder how to map field names.
Go convention is PascalCase; JSON convention is often snake_case.

### 8. Package-Level Variables (Use Sparingly)

```go
// oauth.go - Caching tokens at package level (single-goroutine access)
var tokenCache *TokenCache  // Shared token cache (singleton)

// internal/cache/cache.go - Package-level state (set at startup, read-only during execution)
var (
    cacheDisabled bool
    debugMode     bool
)

func CacheDisabled() bool {
    return cacheDisabled
}
```

**Why:** Sometimes necessary for caching, but prefer dependency injection
when possible to improve testability.

**State Management:**
- OAuth token cache: Accessed from main goroutine only
- Cache state flags: Set once at startup, read-only during execution
- Use `ResetState()` in tests to ensure clean state between test cases

---

## File Descriptions

### Main Package

| File | Responsibility |
|------|----------------|
| [main.go](../main.go) | Application entry point (pure orchestration) |

### Internal Packages - Application Layer

| Package/File | Responsibility |
|--------------|----------------|
| [internal/app/setup.go](../internal/app/setup.go) | Application initialization & configuration |
| [internal/app/runner.go](../internal/app/runner.go) | RunOnce execution mode |
| [internal/app/trueup.go](../internal/app/trueup.go) | True-Up Mode: single-batch Lifetime Data query (QueryModeTrueUp) and report conversion |
| [internal/app/backfill.go](../internal/app/backfill.go) | Backfill Mode: live per-day fetch over a date range into `history/` |
| [internal/app/weather.go](../internal/app/weather.go) | Best-effort weather enrichment for Day-Mode reports |
| [internal/app/rawweather.go](../internal/app/rawweather.go) | Re-fetch past weather for the raw archive; rebuild record weather from archived payloads; weather provenance sidecar |
| [internal/app/verifyenphaseraw.go](../internal/app/verifyenphaseraw.go) | Offline re-derivation of every Enphase history day from `raw/enphase` |
| [internal/app/verifyenphase.go](../internal/app/verifyenphase.go) | Live one-day check that Enphase raw archiving works |

### Internal Packages - CLI Layer

| Package/File | Responsibility |
|--------------|----------------|
| [internal/cli/flags.go](../internal/cli/flags.go) | CLI flag parsing and definitions, grouped `--help`, validation of the maintenance commands |
| [internal/cli/cache_commands.go](../internal/cli/cache_commands.go) | Cache management command handlers |

### Internal Packages - Authentication

| Package/File | Responsibility |
|--------------|----------------|
| [internal/oauth/oauth.go](../internal/oauth/oauth.go) | OAuth token management & refresh |
| [internal/oauth/browser.go](../internal/oauth/browser.go) | Browser-driven OAuth authorization (auto-approves consent) |
| [internal/oauth/oauth_test.go](../internal/oauth/oauth_test.go) | OAuth tests |

### Internal Packages - Business Logic

| Package/File | Responsibility |
|--------------|----------------|
| [internal/aggregator/types.go](../internal/aggregator/types.go) | Metric data structures |
| [internal/aggregator/aggregator.go](../internal/aggregator/aggregator.go) | Multi-system aggregation with DI |
| [internal/display/display.go](../internal/display/display.go) | Terminal output formatting with colors |
| [internal/api/client.go](../internal/api/client.go) | HTTP client for Enphase Cloud API v4 |
| [internal/cache/*](../internal/cache/) | Disk-based response caching |
| [internal/parser/*](../internal/parser/) | JSON telemetry response parsing |
| [internal/config/*](../internal/config/) | Configuration types and utilities |
| [internal/timezone/*](../internal/timezone/) | Timezone handling and date boundaries |
| [internal/constants/*](../internal/constants/) | Centralized constants |
| [internal/history/*](../internal/history/) | Per-day History Record schema and writer (`history/`), dataset index |
| [internal/weather/*](../internal/weather/) | Open-Meteo client (daily/current), WMO code mapping, raw-payload parsing |
| [internal/pge/*](../internal/pge/) | PG&E Green Button: browser download, ESPI decoding, sanity checks, raw archive and its verification |
| [internal/rawstore/*](../internal/rawstore/) | Raw source archive (`raw/`), SHA-256 manifest, cache import, verification primitives |

### Internal Packages - Shared Types

| Package/File | Responsibility |
|--------------|----------------|
| [internal/types/types.go](../internal/types/types.go) | Shared type definitions (SystemConfig, APIConfig) |

---

## Next Steps for Learning

### Recommended Learning Path

1. **Understand Go Concepts First**
   - Read **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** to understand the Go patterns used
   - Focus on: error handling, pointers, struct methods, channels

2. **Start with Entry Point**
   - Read `main.go` - Understand the entry point and CLI structure
   - See how flags are parsed and execution modes are handled

3. **Follow the Data Flow**
   - Read `internal/config/config.go` - See how Go handles YAML and struct validation
   - Trace `internal/aggregator/aggregator.go` - Follow data from systems to output (see [GO_CONCEPTS.md](GO_CONCEPTS.md) for Go concepts)
   - Study `internal/api/client.go` - Learn HTTP client patterns and error handling

4. **Explore Advanced Features**
   - Explore `internal/cache/cache.go` - Understand caching and file I/O
   - Review `internal/display/display.go` - See terminal formatting techniques
   - Study `internal/oauth/oauth.go` - Understand token management and refresh patterns

5. **Go Concepts Reference**
   - Read **[GO_CONCEPTS.md](GO_CONCEPTS.md#channels-and-signals)** for a reference explanation of channels, signals, and the `select` statement (the pattern was used in the now-removed continuous-mode loop; the concepts are general Go)

### Key Files for Learning Go Patterns

| Pattern | Files to Study | What to Look For |
|---------|----------------|------------------|
| **Error Handling** | `internal/oauth/oauth.go`, `internal/api/client.go` | `%w` error wrapping, error propagation |
| **Channels & Select** | `main.go`, `internal/app/runner.go` | `select` statement, signal handling, graceful shutdown |            
| **Concurrency** | `main.go`, `internal/app/runner.go`  | Channels, select statement, signal handling (single-threaded execution) | 
| **Struct Methods** | All files | Pointer vs value receivers, method design |
| **JSON Parsing** | `internal/parser/parser.go` | Struct tags, JSON marshaling/unmarshaling |
| **Defer Usage** | Throughout | Resource cleanup, guaranteed execution |
| **Interfaces** | Throughout | Implicit satisfaction, dependency injection |

### Code Reading Strategy

Each file has package-level documentation explaining its purpose and design decisions. When reading code:

1. **Start with package comments** - They explain the file purpose
2. **Reference [GO_CONCEPTS.md](GO_CONCEPTS.md)** - Explains intermediate Go concepts used in the code
3. **Follow function calls** - Trace execution from `main()` through the call stack
4. **Study error handling** - See how errors are wrapped and propagated
5. **Understand data structures** - See how structs are designed and used

### Quality and CI

Before committing, run the linter and tests:

- **make lint** — Runs golangci-lint (errcheck, goimports, revive, govet, staticcheck). See [.golangci.yml](../.golangci.yml).
- **CI** — Not yet configured. Run `make lint` and `go test ./...` locally before committing.

See [README.md#lint-and-ci](../README.md#lint-and-ci) for details.

### Related Documentation

- **[CONTEXT.md](../CONTEXT.md)** - Domain glossary: the authoritative source for terminology used in code, docs, and conversation
- **[adr/](adr/)** - Architecture Decision Records for non-obvious design choices (e.g. True-Up Window end-date)
- **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** - Go concepts and patterns
- **[GO_CONCEPTS.md](GO_CONCEPTS.md)** - Go concepts including channels and signals
- **[OAUTH_SETUP.md](OAUTH_SETUP.md)** - OAuth authentication explained
- **[README.md](../README.md)** - User documentation and usage guide
