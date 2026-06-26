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
API responses from the `telemetry` and `energy_*_telemetry` endpoints. Returns exactly one calendar day of 15-minute intervals (96 per day). Single-day only — multi-day ranges are rejected by the API. Used for Day Mode queries.
_Avoid_: Interval endpoint, telemetry data

**Lifetime Data**:
API responses from the `energy_*_lifetime` endpoints. Returns cumulative daily totals for all completed past days, but never includes today's partial data. Used for Month, Year, and True-Up Mode queries.
_Avoid_: Lifetime endpoint, historical data

**API Budget**:
The number of live API requests available to a credential, tracked in two windows: a 60-second sliding window and the current calendar month. The Enphase Cloud API enforces 10 requests per minute **and** 1000 requests per month, per API key. With 2 Systems × 5 metrics = 10 calls per run, one full run consumes a single key's entire per-minute budget. The application tracks both windows locally per credential; when a credential is exhausted it falls back to cache — or, when a Credential Pool is configured, to another credential — rather than issuing a guaranteed-failed live call.
_Avoid_: Rate limit, API quota, request quota
_Why_: "Rate limit" describes the HTTP-layer enforcement mechanism (HTTP 429 from Enphase). "API Budget" describes the application-level concept — how many requests the app has available to spend in the current window. The distinction matters in code: constants and functions that deal with the HTTP 429 response correctly use "rate limit" (e.g. `RateLimitError`, `IsRateLimitError`), while functions that track the app's remaining capacity use "budget" (e.g. `RemainingBudget`, `BudgetWarningShown`).

**Credential Pool**:
The ordered set of API credential sets (each `{name, key, client_id, client_secret, refresh_token}`) the app rotates among to scale past a single key's limits. It spreads load round-robin across Systems (`ForSystem`), fails over to a spare when a credential hits a 429 or runs out of monthly budget (`Failover`/`MarkUnavailable`), and is built once at startup so cooldown state survives Continuous Mode ticks. Seeded from the developer portal with `--seed-credentials` and authorized with `--update-refresh-tokens`. Lives in `internal/credentials`.
_Avoid_: Key ring, account pool

**Monthly Quota Baseline**:
The per-credential count of API calls already spent this calendar month, persisted to `cache/monthly-quota.json`. Seeded from the developer portal stats page by `--init` (or resynced out-of-band by `--refresh-quota`) and incremented on every live call (`RecordAPICall`); it resets on the month boundary. Drives monthly rotation: a credential with no monthly budget left is skipped just like one in per-minute cooldown.
_Avoid_: Monthly rate limit

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
The user-configured number of seconds between re-fetches in Continuous Mode. Set via `refresh_interval` in `config.yaml`; defaults to 3600 (1 hour). Values below 60 seconds are clamped up to a 60-second floor (one API Budget window) to avoid exhausting the API Budget on every tick; the clamp is silent at load time and warns only when Continuous Mode actually starts (the only consumer of `refresh_interval`).
_Avoid_: Polling interval, refresh rate, refresh period

**Backfill Mode**:
A run mode activated by `--start-date <YYYY-MM-DD>` (with optional `--end-date <YYYY-MM-DD>`) that fetches each calendar day from the start date through the end date (or yesterday when `--end-date` is omitted), one day at a time, and writes a History Record per day into `history/`. By default pulls from both Enphase API and PG&E web; use `--enphase-api-only` or `--pge-web-only` to restrict the source. Always makes live API calls (cache disabled) so records are authoritative. Weather is an invariant for Enphase records: a day whose Weather Enrichment is unavailable is treated as a per-day failure (no record written) rather than persisting a weatherless record — so every Enphase History Record is usable for correlation, and the day is retried on a plain re-run. Idempotent for Enphase records — days already on disk are skipped unless `--force` overwrites them; PG&E records are always overwritten. Per-day failures are reported and skipped rather than aborting the range. At the end it refreshes the Backfill Index. Cannot be combined with `--continuous`, `--true-up`, or `--init`.
_Avoid_: Bulk fetch, history dump, sync mode

**Backfill Index**:
The manifest `history/.<prefix>-index.json` (e.g. `.enphase-index.json`, `.pge-index.json`), rewritten at the end of each Backfill Mode run by scanning the `history/` directory (so it reflects the whole dataset, not just the latest run). Records the covered date range, present/missing counts, and each missing day with its reason. A dotfile so it stays out of a `history/*.json` glob; best-effort as of the last run. Code identifiers: `history.Index`, `history.WriteIndex`, `history.Dataset.IndexFileName`.
_Avoid_: Manifest (use Backfill Index), catalog, registry

**Weather Code Legend**:
The reference `weather_codes.json` at the project root, written by `--init` (Initialization), decoding every `weather_code` carried by a report or History Record. It is the authoritative WMO interpretation table for the codes Open-Meteo emits — distinct from the lossy display label (a record's `condition`): the legend preserves intensity (61/63/65 = slight/moderate/heavy rain) that the label collapses. A general weather reference, not a dataset artifact, so it lives at the root rather than under `history/`. A fixed standard, regenerated on each `--init`. Code identifiers: `weather.WMOCodeLegend`, `weather.WriteCodeLegend`, `weather.CodeLegendFileName`.
_Avoid_: Condition map (that is the lossy display collapse, `conditionFromCode`), weather dictionary

**Initialization**:
The one-time `--init` step that resolves each System's location (one `/systems` call, geocoding the postal code), caches the coordinates for Weather Enrichment, and writes the Weather Code Legend to the project root. Required before any report mode: the program refuses to run a report until the location cache exists (the "init guard"), exempting only cache-management commands and `--update-refresh-tokens`. `--force` re-resolves even when a cached value exists. Location resolution is done out of band so it never competes with the per-minute telemetry budget on a live day.
_Avoid_: Setup mode, bootstrap, first-run

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
Energy stored into a System's batteries. Only fetched for today's live Day Mode query (`queryMode == QueryModeDay && testDate.IsZero()`); omitted for all other Query Modes and for past dates. Code identifier: `BatteryCharged`.

**Battery Discharge**:
Energy drawn from a System's batteries. Only fetched for today's live Day Mode query (`queryMode == QueryModeDay && testDate.IsZero()`); omitted for all other Query Modes and for past dates. Code identifier: `BatteryDischarged`.

**State of Charge (SOC)**:
The battery charge level of a System, expressed as a percentage (0–100). Only available for today's live Day Mode query — the exact gate in `internal/api/client.go` is `queryMode == QueryModeDay && testDate.IsZero()`, so an explicit `--date <today>` does *not* fetch SOC (testDate is non-zero). Code identifier: `BatterySOC`.
_Avoid_: Battery percentage, battery level, charge percentage

**Net Flow**:
Grid Import minus Grid Export for a System or Site (positive = net import from grid, negative = net export to grid). Code identifiers: `NetFlowToday` (per-day), `NetFlow` (period totals). Config color keys are directional — `net_import` / `net_export` set the foreground color, and `net_import_background` / `net_export_background` set the row-highlight truecolor background. Validation JSON key: `net_flow`.
_Avoid_: Net Import, Net Imported, Net Energy (as a substitute for Net Flow itself — the directional *color* keys `net_import` / `net_export` / `net_import_background` / `net_export_background` are the one exception, since they name the visual treatment, not the metric)

## Weather & Dataset

**Weather Enrichment**:
The best-effort annotation of a Day-Mode report with the day's weather (temperature high/low, WMO weather code, condition, cloud cover, precipitation, solar radiation) from the Open-Meteo API, plus current conditions overlaid for today's live report. Runs only for Day Mode (never Month, Year, True-Up, or cache-only reports) and only after Initialization has cached the location. Any failure leaves the energy report untouched. Code identifier: `enrichWithTemperature`; carrier type `aggregator.DailyWeather`.
_Avoid_: Temperature lookup, forecast (the daily values are observed/aggregated, not a forecast)

**History Record**:
The per-day JSON document written to `history/<YYYY-MM-DD>.json`, holding the day's Site totals, per-System energy values, and Weather Enrichment for offline analysis. Written only by Backfill Mode, which is its sole producer — a plain `--date` report never writes one (it stays a read-only terminal report). This single-writer rule keeps every record authoritative (live-sourced) and avoids a cache-sourced record silently shadowing a date that backfill would then skip. Deliberately excludes Battery Charge/Discharge/SOC — they are unavailable for historical dates. Code identifiers: `history.DayRecord`, `history.FromMetrics`, `history.WriteRecord`.
_Avoid_: Export, dump, snapshot

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
