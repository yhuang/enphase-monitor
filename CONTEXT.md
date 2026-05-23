# Enphase Monitor

A CLI application that monitors, aggregates, and reports energy metrics from one or more Enphase solar Systems at a physical address.

## Language

**Site**:
A single physical address that owns one or more Systems. The Site-level view aggregates metrics across all of its Systems.
_Avoid_: Installation, property, home

**System**:
One Enphase-managed unit at a Site: solar panels, microinverters, Enphase Controller, batteries, Enphase Gateway, and the subpanel whose electrical loads it backs up. Corresponds to exactly one Enphase system ID in Enlighten.
_Avoid_: Installation, sub-panel, unit

## Enphase API Constraints

These terms come from Enphase API documentation, not from the energy domain. They are not user-facing, but they shape the entire data-fetching and caching architecture.

**Interval Data**:
API responses from the `telemetry` and `energy_*_telemetry` endpoints. Returns exactly one calendar day of five-minute intervals (96 per day). Single-day only — multi-day ranges are rejected by the API. Used for Day Mode queries.
_Avoid_: Interval endpoint, telemetry data

**Lifetime Data**:
API responses from the `energy_*_lifetime` endpoints. Returns cumulative daily totals for all completed past days, but never includes today's partial data. Used for Month, Year, and True-Up Mode queries.
_Avoid_: Lifetime endpoint, historical data

**API Budget**:
The number of live API requests available in the current 60-second sliding window. The Enphase Cloud API enforces a limit of 10 requests per minute per API key. With 2 Systems × 5 metrics = 10 calls per run, one full run consumes the entire budget. The application tracks the budget locally; when exhausted, it falls back to cache rather than issuing a guaranteed-failed live call.
_Avoid_: Rate limit, API quota, request quota
_Why_: "Rate limit" describes the HTTP-layer enforcement mechanism (HTTP 429 from Enphase). "API Budget" describes the application-level concept — how many requests the app has available to spend in the current window. The distinction matters in code: constants and functions that deal with the HTTP 429 response correctly use "rate limit" (e.g. `RateLimitError`, `IsRateLimitError`), while functions that track the app's remaining capacity use "budget" (e.g. `RemainingBudget`, `BudgetWarningShown`).

## Cache

**Cache**:
A disk store of raw API responses, keyed by URL and date. Used to avoid redundant API calls and to serve data offline. Lives in `cache/` by default; overridable via `ENPHASE_CACHE_DIR`.
_Avoid_: API cache, response store, local cache

**Cache Mode**:
The user-selected cache behavior for a run. Three modes:
- **Auto** (default): use Cache when available, fall back to live API calls.
- **Cached** (`--cache`): serve the report entirely from Cache; list missing endpoints if incomplete. No live API calls.
- **Live** (`--no-cache`): bypass Cache entirely; always make live API calls.
_Avoid_: Cache-only mode, no-cache mode, cache flag

## Run Modes

**Validation Mode**:
A run mode activated by `--test --date <date>` that serves the report entirely from cache (no live API calls) and then compares each metric against a pre-recorded set of expected values stored in `test-data/`. Exits non-zero if any metric diverges. Used for regression testing after code changes to confirm that calculation logic has not drifted.
_Avoid_: Test mode, cache-validation mode, regression mode
_Why_: The CLI flag is `--test`, which makes "test mode" a natural derivation. But the codebase also has `_test.go` files throughout, and calling a run mode "test mode" creates ambiguity — a reader cannot tell whether "test mode" refers to this run mode or to unit testing. "Validation Mode" names what the mode actually does: it validates that metrics match recorded expected values. Code identifiers follow suit: `ValidationMode()`, `SetValidationMode()`, `ValidateValidationModeCache()`.

**Run-Once Mode**:
The default behavior — execute one query, display the report, and exit. Works with all Query Modes and Cache Modes.
_Avoid_: Single run, one-shot mode

**Continuous Mode**:
A run mode activated by `--continuous` that re-fetches and re-displays today's Day report on every Refresh Interval. Restricted to today's Day query — Month, Year, Past Period, and True-Up Mode queries are silently downgraded to Run-Once Mode (Month / Year / Past Period via the run-mode gate in `main`; True-Up Mode via its dedicated early-return branch). Intended for use with Auto Cache Mode: when the API Budget is temporarily exhausted at a refresh tick, the program gracefully serves the most recent cached data rather than terminating. Using `--no-cache` with Continuous Mode is not recommended — it bypasses the pre-call budget short-circuit, so exhaustion triggers live calls that are likely to 429; on a 429 the program falls back to cache if available (with a warning), or exits with an error if no cache exists.
_Avoid_: Loop mode, polling mode, watch mode

**Refresh Interval**:
The user-configured number of seconds between re-fetches in Continuous Mode. Set via `refresh_interval` in `config.yaml`; defaults to 3600 (1 hour). Must be at least 60 seconds to avoid exhausting the API Budget on every tick.
_Avoid_: Polling interval, refresh rate, refresh period

## Query Modes

**Query Mode**:
How the app determines what data to fetch and display. Inferred from the CLI flags the user supplies — never stated explicitly. Four modes: Day, Month, Year, and True-Up. Code identifier: `QueryMode`.
_Avoid_: Granularity, query type, report type

**Day Mode** / **Month Mode** / **Year Mode**:
The three date-granularity modes, inferred from the format of the `--date` flag: `YYYY-MM-DD` → Day, `YYYY-MM` → Month, `YYYY` → Year.
_Avoid_: Daily query, monthly query, yearly query

**True-Up Mode**:
A distinct Query Mode activated by the `--true-up` flag with a `YYYY-MM-DD` date set by the utility (PG&E). Unlike the date-granularity modes, the date is not inferred — it is the utility-defined start of the True-Up Period. Code identifier: `QueryModeTrueUp`.
_Avoid_: True-up query, annual query

**Current Period**:
A query date that includes today — today's Day query, the current calendar month, or the current calendar year. Data is still accumulating, so cache entries expire (1 hour for today's Day query; 24 hours for the current month or year) and re-fetches are meaningful. SOC is only available for today's Day query (a Current Period Day query). Continuous Mode is restricted to Current Period Day queries.
_Avoid_: Active period, open period, in-progress period

**Past Period**:
A query date entirely before today — a completed day, month, or year. Data is immutable; cache entries never expire. Code identifier: `IsPastPeriod`.
_Avoid_: Historical period, closed period, completed period

## True-Up

**True-Up Period**:
A 12-month solar billing cycle defined by PG&E (or another utility), starting on the anniversary of the solar interconnection date. At the end of the period, PG&E reconciles net metering credits at wholesale rates and resets the balance to zero.
_Avoid_: Billing year, solar year, NEM period

**True-Up Start Date**:
The user-supplied date marking the beginning of a True-Up Period (YYYY-MM-DD). The software snaps this to the first day of the specified month for data retrieval purposes.
_Avoid_: Start date, anniversary date

**True-Up Window**:
The actual date range covered by a True-Up report: from the first day of the True-Up Start Date's month through either yesterday (if the cycle is still in progress) or the last day of the 12-month window (if the cycle has closed). Computed as `min(yesterday, first_day_of_start_month + 12 months - 1 day)`.
_Avoid_: Date range, reporting period

## Energy Flows

**Production**:
Solar energy generated by a System's panels and microinverters. Matches Enphase's own terminology.
_Avoid_: Generation, solar output

**Consumption**:
Total electrical load drawn by a System's subpanel, regardless of source (solar, battery, or grid).
_Avoid_: Load, demand, grid consumption

**Grid Import**:
Energy drawn from the utility grid into a System. Code identifier: `GridImport`.
_Avoid_: Import, energy import, grid draw

**Grid Export**:
Energy pushed from a System into the utility grid. Code identifier: `GridExport`.
_Avoid_: Export, energy export, grid feed

**Battery Charge**:
Energy stored into a System's batteries. Code identifier: `BatteryCharged`.

**Battery Discharge**:
Energy drawn from a System's batteries. Code identifier: `BatteryDischarged`.

**State of Charge (SOC)**:
The battery charge level of a System, expressed as a percentage (0–100). Only available for today's live Day Mode query — the exact gate in `internal/api/client.go` is `queryMode == QueryModeDay && testDate.IsZero()`, so an explicit `--date <today>` does *not* fetch SOC (testDate is non-zero). Code identifier: `BatterySOC`.
_Avoid_: Battery percentage, battery level, charge percentage

**Net Flow**:
Grid Import minus Grid Export for a System or Site (positive = net import from grid, negative = net export to grid). Code identifiers: `NetFlowToday` (per-day), `NetFlow` (period totals). Config color keys are directional — `net_import` / `net_export` set the foreground color, and `net_import_background` / `net_export_background` set the row-highlight truecolor background. Validation JSON key: `net_flow`.
_Avoid_: Net Import, Net Imported, Net Energy (as a substitute for Net Flow itself — the directional *color* keys `net_import` / `net_export` / `net_import_background` / `net_export_background` are the one exception, since they name the visual treatment, not the metric)

## Example Dialogue

> "The True-Up report is showing way more Grid Import than I expected."
>
> "Which System? Left Subpanel or Right Subpanel?"
>
> "The Site total — I ran `--true-up 2025-01-15`."
>
> "Is that cycle still open, or has it closed? If you're past January 2026, the True-Up Window should have capped at December 31st, not run through yesterday."
>
> "Oh — it closed in January. So the True-Up Window ends at the last day of the 12-month window, not today?"
>
> "Right. `min(yesterday, start_month + 12 months - 1 day)`. For a closed cycle the end is fixed. For an in-progress one it moves every day."
>
> "That explains it. The Lifetime Data for that System has a lot of Grid Import in early 2026 that shouldn't be in this True-Up Period at all."
