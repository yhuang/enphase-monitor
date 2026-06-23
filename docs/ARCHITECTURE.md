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
5. [API Budget Checks](#api-budget-checks)
6. [Key Go Patterns Used](#key-go-patterns-used)
7. [File Descriptions](#file-descriptions)
8. [Next Steps for Learning](#next-steps-for-learning)

---

## Project Overview

The **enphase-monitor** is a CLI application that monitors energy metrics from one or more Enphase solar Systems at a Site via the Enphase Enlighten Cloud API v4.

### Core Capabilities

- **Multi-System Monitoring**: Query and aggregate data from multiple independent Systems at a Site
- **Cloud API Integration**: Uses Enphase Enlighten Cloud API v4 exclusively (no local network access required)
- **Intelligent Caching**: Disk-based response caching to stay within the API Budget (10 calls/minute)
- **Past Period Queries**: Query any past date with `--date` flag (auto-runs once since Past Period data is immutable)
- **True-Up Report**: Energy metrics across a full utility True-Up Period with `--true-up`
- **Real-time Monitoring**: Continuous mode with configurable refresh interval (default: 1 hour)
- **Color Customization**: Customize terminal output colors via YAML configuration
- **Validation Mode**: Test against expected values without making API calls

### Tech Stack

- **Language**: Go 1.21+
- **Configuration**: YAML (`gopkg.in/yaml.v3`)
- **HTTP Client**: Standard library `net/http` (no frameworks)
- **Authentication**: OAuth 2.0 with Bearer tokens
- **Caching**: File-based (JSON responses stored on disk)

---

## Module Structure

```
enphase-monitor/
├── main.go                                # Entry point (~720 lines) - orchestration only
├── internal/
│   ├── aggregator/                        # Multi-system data aggregation
│   │   ├── types.go                       # Metric data structures (AggregatedMetrics, SystemMetrics)
│   │   ├── aggregator.go                  # Aggregation logic with dependency injection
│   │   ├── aggregator_test.go             # Aggregator tests with mock clients
│   │   └── aggregator_bench_test.go       # Benchmark tests
│   ├── api/                               # HTTP client for Cloud API v4
│   │   ├── client.go                      # Enlighten Cloud API client
│   │   ├── types.go                       # API request/response types
│   │   ├── cache_check.go                 # Per-system/endpoint cache availability check (--cache mode)
│   │   ├── client_test.go                 # API client unit tests
│   │   ├── client_caching_test.go         # Characterization tests for makeCachedAPIRequest fallback branches (validation/no-cache modes, 429/503/network-error cache fallbacks)
│   │   ├── client_functional_test.go      # Functional tests with mock HTTP servers
│   │   ├── client_lifetime_test.go        # Lifetime Data tests (Month, Year, True-Up Mode queries)
│   │   ├── preflight_test.go              # Budget-exhaustion cache-fallback tests (all 8 Query Mode × Period combinations)
│   │   ├── query_cost_test.go             # QueryCost unit tests (all QueryMode × hasBattery combos)
│   │   └── testmain_test.go               # TestMain: redirects cache I/O to temp dir for all api tests
│   ├── app/                               # Application execution logic
│   │   ├── setup.go                       # App initialization & configuration
│   │   ├── setup_test.go                  # Setup tests
│   │   ├── runner.go                      # Execution modes (once/continuous)
│   │   ├── runner_test.go                 # Runner tests
│   │   ├── trueup.go                      # True-Up Mode: single-batch Lifetime Data query, True-Up Window end-date logic, report conversion
│   │   ├── trueup_test.go                 # True-up Window end-date logic (trueUpWindowEnd) and report conversion tests
│   │   ├── backfill.go                    # Backfill Mode: live per-day fetch over a date range into history/
│   │   ├── backfill_test.go               # Backfill range/skip/overwrite tests
│   │   ├── weather.go                     # Best-effort weather enrichment for Day-Mode reports
│   │   ├── weather_test.go                # Weather enrichment tests
│   │   └── cache_report.go                # --cache mode: completeness check and diagnostic output
│   ├── browser/                           # Headed Chrome launcher (chromedp) for portal automation
│   │   ├── chrome.go                      # LaunchHeaded: disposable-profile Chrome session
│   │   └── chrome_test.go                 # Chrome launcher tests
│   ├── cache/                             # Disk-based response caching
│   │   ├── cache.go                       # Cache implementation + sliding-window budget
│   │   ├── cache_test.go                  # Cache state management tests (ValidationMode, CacheDisabled, BudgetWarningShown, ResetState)
│   │   ├── cache_functions_test.go        # Core caching tests (URL normalization, save/load, HasCacheForDate)
│   │   ├── api_budget_test.go             # Sliding-window API Budget counter tests (RecordAPICall, RemainingBudget, pruning)
│   │   ├── cli.go                         # Cache inspection utilities
│   │   └── cli_test.go                    # CLI utilities tests
│   ├── cli/                               # Command-line interface
│   │   ├── flags.go                       # CLI flag parsing
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
│   │   ├── quota.go                       # Per-credential minute + monthly API budget (monthly-quota.json)
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
│   ├── history/                           # Per-day energy+weather JSON records (history/)
│   │   ├── history.go                     # DayRecord schema, FromMetrics, WriteRecord
│   │   └── history_test.go                # History mapping and write tests
│   ├── location/                          # Resolve & cache systems' coordinates (--init)
│   │   ├── location.go                    # Location resolver with disk cache
│   │   └── location_test.go               # Location resolver tests
│   ├── oauth/                             # OAuth 2.0 authentication
│   │   ├── oauth.go                       # Token management & refresh
│   │   ├── browser.go                     # Browser-driven OAuth authorization (auto-approves consent)
│   │   ├── oauth_test.go                  # Basic unit tests
│   │   ├── browser_test.go                # Browser-OAuth flow tests
│   │   ├── oauth_functional_test.go       # Integration tests with mock servers
│   │   └── oauth_edge_cases_test.go       # Edge case and error path tests
│   ├── parser/                            # JSON telemetry parsing
│   │   ├── parser.go                      # Response parsing utilities
│   │   ├── parser_test.go                 # Parser tests
│   │   └── parser_bench_test.go           # Benchmark tests
│   ├── timezone/                          # Timezone handling
│   │   ├── timezone.go                    # Timezone utilities
│   │   └── timezone_test.go               # Timezone tests
│   ├── types/                             # Shared type definitions
│   │   └── types.go                       # SystemConfig, APIConfig (breaks circular deps)
│   ├── urlbuilder/                        # API URL construction
│   │   ├── urlbuilder.go                  # URL building helpers
│   │   └── urlbuilder_test.go             # URL builder tests
│   ├── validation/                        # Validation Mode (--test flag)
│   │   ├── validation.go                  # Metrics validation logic (uses io.Writer for testability)
│   │   ├── validation_test.go             # Unit tests (tolerance calculations, edge cases)
│   │   └── validation_integration_test.go # Integration tests (real expected values)
│   └── weather/                           # Open-Meteo daily/current weather client
│       ├── weather.go                     # DailyWeather/CurrentWeather fetch + WMO code mapping
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
├── test-data/                             # Test validation data (expected values)
├── config.yaml                            # User configuration: non-secret settings (not in git)
├── config.yaml.example                    # Configuration template
├── credentials.yaml                       # API secrets: credentials: pool (not in git)
├── credentials.yaml.example               # Credentials template
└── cache/                                 # Cached API responses (created at runtime)
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
│     └─► Build credentials.Pool from cfg.Credentials                 │
│     └─► If --update-refresh-tokens: select credential (positional name),       │
│         signal context, oauth.Authorize(ctx, cred); Ctrl+C cancels it │
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
│  ├─► Parse start date        │  │     └─► app.RunOnce(ctx, ...) or app.RunContinuous(ctx, ...)       │
│  ├─► Normalize to month-1    │  │     └─► RunContinuous: synchronous for/select (ticker.C, ctx.Done) │
│  ├─► GetAggregatedMetrics    │  │         (no goroutines spawned)                                    │
│  │   (QueryModeTrueUp,       │  │     └─► fetchAndDisplay(ctx, ...) calls aggregator                 │
│  │   4 metrics/system,       │  └────────────────────────────────────────────────────────────────────┘
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
│         ├─► Past periods: always served from cache (data immutable)  │
│         ├─► Current periods: live call when budget allows;           │
│         │   cache is fallback only when budget exhausted             │
│         ├─► Budget exhausted: exact-URL cache → cross-endpoint       │
│         │   same-system cache (any age) → RateLimitError             │
│         ├─► Make HTTP request if Current Period and budget > 0       │
│         └─► Save response to cache + append timestamp to api_calls   │
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
│  - Sliding-window API Budget counter (api_calls, 10/60s)           │
│  - Past Periods: always from cache; Current Periods: live-first    │
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

## API Budget Checks

The Enphase Cloud API enforces a budget of **10 requests per 60-second sliding window**. With two systems each making a full day query (5 endpoints each), that is exactly 10 calls — right at the limit. To stay within it, the client uses three cooperating layers: a cost estimator, a preflight warning, and a per-request gate.

### Layer 1: `QueryCost` — call-count estimator

`QueryCost(queryMode, hasBattery)` in [internal/api/client.go](../internal/api/client.go) returns the number of live API calls that `GetMetricsFromCloud` will make for a **single System**:

| Query mode | hasBattery=false | hasBattery=true |
|------------|------------------|-----------------|
| Day | 4 | 5 |
| Month | 4 | 4 |
| Year | 4 | 4 |
| True-Up | 4 | 4 |

The base of 4 covers: Grid Import, Grid Export, Production, and Consumption. Battery telemetry adds a 5th call **only for today's Day query** (`testDate.IsZero() && QueryModeDay`) — all other cases skip it because:
1. The battery telemetry endpoint returns per-15-minute intervals; fetching it for Past Period or multi-day queries would require one call per day, far exceeding the budget.
2. Battery SOC is a point-in-time reading that is not meaningful as a Past Period or multi-day aggregate, so the call is omitted for any non-today query.

This means 2 systems × 5 (day + battery) = **exactly 10** — the documented architectural limit. Adding a third system or a supplementary call would exceed it.

### Layer 2: Preflight check in `GetMetricsFromCloud`

Before making any API calls, `GetMetricsFromCloud` compares the query cost against the remaining budget:

```go
// internal/api/client.go (inside GetMetricsFromCloud)
if cache.DebugMode() && !timezone.IsPastPeriod(testDate, queryMode, c.timezone) {
    needed := QueryCost(queryMode, queryMode == constants.QueryModeDay)
    remaining := cache.RemainingBudget()
    if remaining < needed {
        fmt.Printf("WARNING: ... Insufficient API budget: need %d call(s), %d/%d remaining ...\n", ...)
    }
}
```

Key design decisions:
- **Past Periods are skipped entirely.** A Past Period Day / Month / Year / True-Up always comes from immutable Cache and never consumes any budget, so a preflight check would be misleading noise.
- **Day queries use `hasBattery=true` (5 calls) as the conservative count.** At this point the client does not know whether the hardware has a battery, so it assumes the worst case. This ensures the warning fires early enough to be useful.
- **The preflight only warns — it does not abort.** Each individual call will still try cache before giving up, so partial data is still possible even when the budget is tight.
- **The warning is debug-only.** When `--debug` is off the check is skipped entirely so that normal report output is not cluttered with diagnostic noise.

### Layer 3: Per-request gate in `makeCachedAPIRequest`

The actual enforcement happens inside `makeCachedAPIRequest` for every URL. The decision tree (simplified):

```
Is this a Past Period with a valid cache entry?
  YES → serve immutable cache, no budget consumed, done
  NO  ↓  (Current Period: live-first — data changes throughout the day)
Is budget exhausted (RemainingBudget() <= 0)?
  YES → try exact-URL cache (any age)
      → try cross-endpoint same-system cache (any age)
      → return RateLimitError
  NO  ↓
Make live API call → record timestamp → save to cache → return
  → on 429/503: serve any-age cache if available, else RateLimitError
```

The cross-endpoint fallback (step 3, middle branch) lets the client surface *some* recent data for the same endpoint+system even when the URL differs (e.g. a new `--date` value), rather than failing outright.

### Interaction between layers

The preflight (Layer 2) fires once per `GetMetricsFromCloud` call and is a forward-looking warning. The per-request gate (Layer 3) fires once per endpoint and is the actual enforcement. Together:

- If budget is full when the run starts and is exhausted mid-run (e.g. the first 4 calls succeed but the 5th lands after the window), Layer 3 handles it gracefully with a cache fallback.
- If budget is already low before the run, Layer 2 warns the user up front (debug mode only) so they understand why the output may show stale numbers.
- If budget is zero and no cache exists for the endpoint, `RateLimitError` is returned and the metric is shown as 0 in the output (non-fatal for optional metrics like Grid Import/Export; fatal for Production which is required).

### Observability: `--debug` mode

When run with `--debug`, the application emits structured logs to stderr that expose Layer 2 and Layer 3 decisions:

- **Startup banner** (`printDebugStartup` in [main.go](../main.go)) shows the current time, the most recent recorded API call (`cache.LastAPICallTime()`), how long until the 60-second window resets, and the remaining budget. This is the single best diagnostic for "why am I getting 429s?" because it answers "is the window still active?" before any work begins.
- **Preflight warning** (Layer 2, in `GetMetricsFromCloud`) prints `WARNING: … Insufficient API budget …` to stderr when the remaining budget is smaller than the query cost for a current-period run. Only emitted in debug mode to avoid cluttering normal report output.
- **Cached mode banner** (`RunCacheReport`) prints `CACHE MODE: Serving report from cache, no live API calls` when `--cache` finds a complete cache. Only emitted in debug mode for the same reason.
- **Per-request trace** (`cache.Debugf` from `makeCachedAPIRequest` in [internal/api/client.go](../internal/api/client.go)) emits one line per URL describing the decision taken: serving Past Period immutable cache, falling back due to budget exhaustion, making a live call, or hitting the 429/503 fallback paths. Each line includes the redacted URL and the cache age so traces are reproducible without leaking the API key.

Debug mode also suppresses the terminal-clearing escape sequence in `fetchAndDisplay` so the trace remains visible after the report is printed. The `cache.Debugf` helper is a no-op when debug mode is off — callers do not need to guard the call sites.

### Tests

| Test File | What it covers |
|-----------|---------------|
| [internal/api/query_cost_test.go](../internal/api/query_cost_test.go) | All 8 `QueryCost` combinations; asserts 2 Systems × Day Mode with battery = exactly 10 calls |
| [internal/api/preflight_test.go](../internal/api/preflight_test.go) | Budget-exhaustion cache fallback for all 8 Query Mode × Period combinations; preflight warning emitted/suppressed correctly |

`preflight_test.go` follows the same **prime → exhaust → probe** pattern for every Query Mode × Period combination: a first live call populates the cache, the budget is manually drained to zero via `cache.RecordAPICall()`, and the probe call must serve from cache with zero additional server hits. Past Period probes (scenarios 2, 4, 6, 8) never even reach the budget check — they short-circuit at the `isPast` branch — making them a useful control group that confirms immutable-cache behaviour is budget-free.

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

This pattern is used **only in continuous monitoring mode** (with `--continuous`). It is not about parallelism—the work is completely serial. It solves a specific problem: **how to wait for a timer but respond instantly to Ctrl+C**.

```go
// main.go - Signal context for graceful shutdown
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

// internal/app/runner.go (RunContinuous) - Ticker and interruptible wait loop
ticker := time.NewTicker(time.Duration(rc.Cfg.RefreshIntervalSeconds) * time.Second)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        // Timer fired - do periodic work
        fetchAndDisplay(ctx, rc)
    case <-ctx.Done():
        // Signal received - exit gracefully
        rc.Disp.ShowInfo("Shutting down gracefully...")
        return nil  // defer ticker.Stop() runs here
    }
}
```

#### Why Not Just Use `time.Sleep()`?

```go
// ❌ Simple but unresponsive
for {
    fetchAndDisplay(...)
    time.Sleep(1 * time.Hour)  // Blocked here - Ctrl+C kills process abruptly
}
```

With `time.Sleep()`:
- Ctrl+C during sleep terminates the process immediately
- `defer` statements do not run (no cleanup)
- No graceful shutdown message

#### What `select` Actually Does

`select` blocks until ONE of its cases is ready, then executes that case. It is **multiplexed waiting**—the program sleeps (zero CPU) but listens for multiple wake-up events:

```
time.Sleep() approach:
    [work] [====== BLOCKED, DEAF TO SIGNALS ======] [work] ...
                     ↑ Ctrl+C here = abrupt death

select {} approach:
    [work] [====== BLOCKED BUT LISTENING ======] [work] ...
                     ↑ Ctrl+C here = instant graceful exit
```

#### No Goroutines Spawned

This code does not create any goroutines. The signal handler runs in a Go runtime goroutine (created by `signal.NotifyContext`), but that is invisible to us. Our code remains single-threaded—`select` just provides interruptible waiting.

> **📖 For a comprehensive explanation of channels, signals, and the `select` statement, including detailed flow diagrams, code walkthroughs, and real-world examples, see [GO_CONCEPTS.md](GO_CONCEPTS.md#channels-and-signals)**.

#### When You Need This Pattern

| Requirement | `time.Sleep()` | `select` with channels |
|-------------|----------------|------------------------|
| Wait for fixed duration | ✅ | ✅ |
| Respond instantly to signals | ❌ | ✅ |
| Graceful shutdown | ❌ | ✅ |
| `defer` cleanup guaranteed | ❌ | ✅ |
| Zero CPU while waiting | ✅ | ✅ |

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
    validationMode     bool
    cacheDisabled      bool
    debugMode          bool
    budgetWarningShown bool
)

func ValidationMode() bool {
    return validationMode
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
| [main.go](main.go) | Application entry point (pure orchestration) |

### Internal Packages - Application Layer

| Package/File | Responsibility |
|--------------|----------------|
| [internal/app/setup.go](../internal/app/setup.go) | Application initialization & configuration |
| [internal/app/runner.go](../internal/app/runner.go) | Execution modes (once/continuous) |
| [internal/app/trueup.go](../internal/app/trueup.go) | True-Up Mode: single-batch Lifetime Data query (QueryModeTrueUp) and report conversion | 
| [internal/app/cache_report.go](../internal/app/cache_report.go) | --cache mode: per-system endpoint check, diagnostic output, and cached run |

### Internal Packages - CLI Layer

| Package/File | Responsibility |
|--------------|----------------|
| [internal/cli/flags.go](../internal/cli/flags.go) | CLI flag parsing and definitions |
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
| [internal/api/cache_check.go](../internal/api/cache_check.go) | Per-system/endpoint cache availability probe (used by --cache mode) |
| [internal/cache/*](../internal/cache/) | Disk-based response caching |
| [internal/parser/*](../internal/parser/) | JSON telemetry response parsing |
| [internal/config/*](../internal/config/) | Configuration types and utilities |
| [internal/timezone/*](../internal/timezone/) | Timezone handling and date boundaries |
| [internal/validation/*](../internal/validation/) | Validation Mode with tolerance-based checks |
| [internal/constants/*](../internal/constants/) | Centralized constants |

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

5. **Deep Dive into Concurrency**
   - Read **[GO_CONCEPTS.md](GO_CONCEPTS.md#channels-and-signals)** for detailed explanation of channels and signal handling
   - See how `select` statement enables graceful shutdown

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

- **make lint** — Runs golangci-lint (errcheck, goimports, revive, govet, staticcheck). See [.golangci.yml](.golangci.yml).
- **CI** — Not yet configured. Run `make lint` and `go test ./...` locally before committing.

See [README.md#lint-and-ci](README.md#lint-and-ci) for details.

### Related Documentation

- **[CONTEXT.md](../CONTEXT.md)** - Domain glossary: the authoritative source for terminology used in code, docs, and conversation
- **[adr/](adr/)** - Architecture Decision Records for non-obvious design choices (e.g. True-Up Window end-date)
- **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** - Go concepts and patterns
- **[GO_CONCEPTS.md](GO_CONCEPTS.md)** - Go concepts including channels and signals
- **[OAUTH_SETUP.md](OAUTH_SETUP.md)** - OAuth authentication explained
- **[README.md](../README.md)** - User documentation and usage guide
