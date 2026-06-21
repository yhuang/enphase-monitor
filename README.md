# Enphase Monitor

A Go application for monitoring and aggregating data from one or more Enphase solar Systems at a Site via the Enphase Enlighten Cloud API v4.

> **📚 New to Go?** This codebase follows Go best practices and includes comprehensive documentation. See **[GO_BEST_PRACTICES.md](docs/GO_BEST_PRACTICES.md)** for a guide to Go patterns and idioms, and **[GO_CONCEPTS.md](docs/GO_CONCEPTS.md)** for a reference of Go concepts used in the code.

## Table of Contents

- [Quick Start](#quick-start)
- [Features](#features)
- [Installation](#installation)
- [Configuration](#configuration)
- [OAuth Setup](#oauth-setup)
- [Usage](#usage)
- [Output Format](#output-format)
- [API Configuration](#api-configuration)
- [Caching and API Budget](#caching-and-api-budget)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)
- [Documentation](#documentation)
- [Project Structure](#project-structure)

## Quick Start

**New to this project?** Start here:

1. **[QUICKSTART.md](QUICKSTART.md)** - Get up and running in 5 minutes
2. **[OAUTH_SETUP.md](docs/OAUTH_SETUP.md)** - Complete OAuth authentication (required for first-time setup)
3. Return here for detailed usage and configuration

## Features

- **Multi-System Monitoring**: Query and combine metrics from multiple independent Systems at a Site
- **Comprehensive Metrics**: Track Production, Consumption, battery charge/discharge, Grid Import/Export, and Net Flow
- **Flexible Querying**: Query past dates or monitor real-time with auto-refresh
- **Weather Enrichment**: Day-Mode reports annotated with temperature, conditions, cloud cover, precipitation, and solar radiation (Open-Meteo; requires one-time `--init`)
- **Historical Backfill**: Build a per-day energy + weather dataset over a date range as JSON records for offline analysis (`--backfill-from`)
- **True-Up Report**: Energy metrics across a full utility True-Up Period (`--true-up`)
- **Clean Display**: Formatted terminal output with customizable colors
- **API Caching**: Automatic caching of API responses to reduce API calls and enable offline validation
- **Color Customization**: Customize terminal colors using hex codes or ANSI escape codes
- **Cloud API v4**: Uses Enphase Enlighten Cloud API v4 for reliable data access
- **OAuth Support**: Full OAuth 2.0 support for developer plan authentication
- **Validation Mode**: Validate metrics against expected values without making API calls (`--test` flag)

## Prerequisites

- Go 1.21 or higher
- **Enphase Developer Portal Account**: Register at https://developer-v4.enphase.com/
- **OAuth Credentials**: Get `api_key`, `client_id`, and `client_secret` from the Developer Portal
- **System IDs**: Your Enphase system IDs (find these from Enlighten URLs - see "Finding Your System IDs" section)

## Installation

### Step 1: Clone the Repository

```bash
git clone <repository-url>
cd enphase-monitor
```

Or download and extract the project files.

### Step 2: Install Dependencies

```bash
go mod download
```

### Step 3: Build the Application

```bash
go build -o enphase-monitor
```

## Configuration

### Initial Setup

Configuration is split across two files:
- **`config.yaml`** — non-secret settings (systems, refresh interval, timezone, colors). Safe to share/commit.
- **`credentials.yaml`** — your API key and OAuth secrets. Kept local (gitignored); never commit it.

1. **Copy the example files:**
   ```bash
   cp config.yaml.example config.yaml
   cp credentials.yaml.example credentials.yaml
   ```

2. **Edit `credentials.yaml` with your API credentials:**

   `credentials:` is a list of one or more credential sets (secrets only). A single
   entry is fine; add more keys to spread the per-key rate limit (10 req/min,
   1000/month) across systems and fail over when a key is throttled. Each entry
   needs a unique `name`. The non-secret `authorization_url` and `redirect_uri` are
   shared by all credentials and set once in `config.yaml` under `api:` (see below).

   ```yaml
   credentials:
     - name: enphase-monitor-001
       key: "YOUR_API_KEY"  # API Key from Enphase Developer Portal
       client_id: "YOUR_CLIENT_ID"  # OAuth Client ID
       client_secret: "YOUR_CLIENT_SECRET"  # OAuth Client Secret
       refresh_token: "YOUR_REFRESH_TOKEN"  # From OAuth setup (see below)
   ```

3. **Edit `config.yaml` with your systems and preferences:**

   ```yaml
   # Shared, non-secret OAuth settings applied to every credential set
   api:
     authorization_url: "https://api.enphaseenergy.com/oauth/token"
     redirect_uri: "http://localhost:8080/callback"  # Must match your Developer Portal app settings

   systems:
     - name: "Left Subpanel"
       id: "YOUR_SYSTEM_ID"  # Enlighten system ID

     - name: "Right Subpanel"
       id: "YOUR_OTHER_SYSTEM_ID"  # Different system ID

   refresh_interval: 3600  # Refresh interval in seconds (1 hour recommended)
   
   # Optional: Timezone for reporting/display
   # If not specified, uses OS system timezone (or US/Pacific if OS is UTC)
   timezone: "US/Pacific"  # Examples: "US/Pacific", "America/New_York", "Europe/London"
   
   # Optional: Color customization
   colors:
     production: "#f0b57c"         # Solar Production
     discharge: "#7acf38"          # Battery Discharge
     import: "#f63cb1"             # Grid Import
     export: "#06b6de"             # Grid Export
     net_import_background: "#010469"  # Net Flow line background highlight (import direction) — rendered as 24-bit truecolor
     net_export_background: "#7D0069"  # Net Flow line background highlight (export direction) — rendered as 24-bit truecolor
     net_import: "#f63cb1"         # Net Flow (foreground color when net is import)
     net_export: "#06b6de"         # Net Flow (foreground color when net is export)
     headers: "#f37320"            # Report Headers
     charge: "#7acf38"             # Battery Charge
     total_consumed: "#f37320"     # Total Consumed
     secondary_text: "#808080"     # Secondary Text
     primary_text: "#ffffff"       # Primary Text
     error: "#ff0000"              # Error Text
   ```

> **💡 Implementation Details**: See [internal/config/config.go](internal/config/config.go) for color conversion logic (hex → ANSI) and [internal/display/display.go](internal/display/display.go) for color usage in terminal output.

### Configuration Sections

#### API Credentials

These live in `credentials.yaml` (kept separate from `config.yaml` so secrets stay local) under a `credentials:` list of one or more credential sets. You will need the values from the [Enphase Developer Portal](https://developer-v4.enphase.com/). Each entry has:
- `name`: A unique label for the credential set (names a credential for `--update-refresh-token <name>` and is the token-cache key)
- `key`: Your API key
- `client_id`: OAuth client ID
- `client_secret`: OAuth client secret
- `refresh_token`: Obtained from OAuth setup (see [OAuth Setup](#oauth-setup) below)

The non-secret `authorization_url` and `redirect_uri` are **not** repeated per credential — they are configured once in `config.yaml` under `api:` and shared by every credential set (a credential may override them by setting its own). `authorization_url` defaults to the Enphase token endpoint if omitted.

Listing more than one credential set lets the app spread the per-key rate limit (10 requests/minute, 1000/month) round-robin across systems and fail over to a spare key when one is throttled (429).

#### System Configuration

Each system requires:
- `name`: A friendly name for display
- `id`: Your Enphase system ID (see [Finding Your System IDs](#finding-your-system-ids))

#### Optional Settings

- `refresh_interval`: How often to query the API in continuous mode (default: 3600 seconds = 1 hour)
  - **⚠️ Important**: Only applies when running in continuous mode (with `--continuous` flag)
  - **Recommended**: Use 3600 seconds (1 hour) to stay within the API Budget
  - **API Budget Consideration**: The API Budget is 10 calls per minute. For 2 systems that is 10 API calls per cycle (2 systems × 5 metrics) — exactly the limit. Values below 60 seconds are automatically clamped up to a 60-second floor (one API Budget window), so a configured `refresh_interval: 5` actually runs at 60 seconds; in continuous mode a warning is printed when this clamping happens. This prevents a low value from exhausting the budget on every tick.
  - **Best Practice**: Use 3600 seconds (1 hour) or higher to stay well within the API Budget
- `timezone`: Timezone for reporting and display (optional)
  - **Default**: Uses your OS system timezone, or US/Pacific if OS timezone is UTC
  - **Format**: IANA timezone identifier (e.g., `"US/Pacific"`, `"America/New_York"`, `"Europe/London"`)
  - **Examples**: 
    ```yaml
    timezone: "US/Pacific"        # Pacific Time
    timezone: "America/New_York"  # Eastern Time
    timezone: "Europe/London"     # GMT/BST
    ```
  - **Usage**: All date/time display and API queries use this timezone. If not specified, the application uses your OS system timezone, falling back to US/Pacific if the system timezone is UTC.
- `colors`: Customize terminal output colors (see [Color Customization](#color-customization))

### Finding Your System IDs

To find your System IDs:

1. Log into https://enlighten.enphaseenergy.com
2. Select one of your systems
3. Look at the URL - it will be: `https://enlighten.enphaseenergy.com/systems/SYSTEM_ID/overview`
4. The number in the URL is your System ID
5. Repeat for each system you want to monitor

## OAuth Setup

**⚠️ Required for First-Time Setup**

This application uses OAuth 2.0 for authentication. You must complete a one-time OAuth setup to get your refresh token.

### Quick Setup

```bash
./enphase-monitor --update-refresh-token
```
Run the interactive setup wizard that will:
1. Open your browser to the Enphase authorization page
2. Wait while you log in and authorize the application
3. Capture the authorization code automatically (a local listener on your
   `redirect_uri`, e.g. `http://localhost:8080/callback`) and exchange it for tokens
4. Write the refresh token straight into the matching entry in your `credentials.yaml`

You normally just authorize in the browser — no copy-paste. If your `redirect_uri`
isn't a localhost address (or its port is busy), the wizard falls back to asking you
to paste the redirect URL from your browser's address bar.

With more than one credential set configured, name the one to set up:

```bash
./enphase-monitor --update-refresh-token enphase-monitor-002
```

Or re-authorize every credential in turn (e.g. after they've all expired):

```bash
./enphase-monitor --update-refresh-token --all
```

### Detailed Guide

For a comprehensive explanation of OAuth 2.0, what each component does, and how authentication works, see:

**[OAUTH_SETUP.md](docs/OAUTH_SETUP.md)** - Complete OAuth guide with:
- Explanation of OAuth 2.0 concepts
- What the API server expects for authentication
- How authorization works
- Step-by-step setup instructions
- Troubleshooting common issues

### After OAuth Setup

The wizard writes the obtained `refresh_token` directly into the matching credential
entry in your `credentials.yaml` (preserving your comments and other entries) — the
secret is never displayed for copy-paste. From then on, the application automatically
uses this token to get new access tokens when needed.

## API Configuration

This application uses the **Enphase Enlighten Cloud API v4** exclusively to fetch energy data.

### What the Cloud API Provides

- **Direct Daily Values**: The API returns pre-calculated daily totals - no need for state files or cumulative meter tracking
- **Past Period Queries**: Query any past date (not limited to today)
- **Real-time Updates**: Data is typically updated every 15 minutes
- **Reliable Access**: Works from anywhere with internet (no local network required)
- **Standardized Format**: Consistent JSON responses across all system types

### API Limitations

The Enphase Cloud API v4 has the following technical limits:

- **Interval Data: Single-Day Only**: Interval Data endpoints (15-minute data) return exactly **one calendar day** of data per request — the API ignores the `end_at` parameter and always responds with `granularity=day`. Querying a full month via Interval Data endpoints would require 28–31 separate calls, which would exceed the API Budget (10 calls/minute).
  - The application automatically uses **daily aggregated `_lifetime` endpoints** (Lifetime Data) for Month and Year Mode queries
  - These endpoints return one value per calendar day and can span arbitrary date ranges in a single request
  - Example: Querying January 2026 (31 days) → **single API request** using `energy_lifetime` endpoint
  - Single-day queries still use Interval Data endpoints for better granularity (96 data points vs 1 per day)
- **Current Month/Year Data Coverage**: When querying the current (ongoing) month or year, data is reported through the **previous complete day** (yesterday). Today's partial data is excluded because the `_lifetime` endpoints only contain completed days. The query range displayed in the report reflects this — it ends at yesterday 11:59 PM, not the current time.
- **API Budget**: See [API Budget](#api-budget) section below

### API Budget

The free developer plan has the following limits:
- **10 requests per minute** per API key
- **1000 requests per month** total

The application automatically caches API responses to minimize API calls. The default refresh interval is 1 hour (3600 seconds) to stay well within these limits.

### Getting API Credentials

1. **Register**: Create an account at https://developer-v4.enphase.com/
2. **Create Application**: Register a new application to get API credentials
3. **Get Credentials**: You will receive:
   - `api_key`: Your API key (used in all requests)
   - `client_id`: OAuth client ID
   - `client_secret`: OAuth client secret
4. **Complete OAuth Setup**: See [OAuth Setup](#oauth-setup) section above

## Usage

### Initialization & Weather

Before running any report, initialize the systems' location once:
```bash
./enphase-monitor --init
```

`--init` makes a single `/systems` call, geocodes each system's postal code to coordinates, and caches the result. This enables **weather enrichment**: Day-Mode reports are annotated with the day's temperature high/low, conditions, cloud cover, precipitation, and solar radiation (from the [Open-Meteo](https://open-meteo.com/) API). Resolving the location out of band keeps it off the per-minute telemetry budget on a live day.

`--init` also writes `weather_codes.json` to the project root — the authoritative WMO legend decoding the `weather_code` field carried by reports and History Records (each of the codes Open-Meteo emits, defined individually so intensities are preserved). It is a general reference rather than a dataset artifact, regenerated on each `--init`.

**Initialization is required.** Every report mode (`--date`, `--continuous`, `--true-up`, `--backfill-from`, and the default today query) refuses to run until `--init` has populated the location cache, exiting with:
```
enphase-monitor: not initialized — run `enphase-monitor --init` first.
```
Cache-management commands (`--clear-cache`, etc.) and `--update-refresh-token` are exempt. Re-run `--init` if the cache is cleared; add `--force` to re-resolve even when a cached value exists.

### Run Once (Single Query, Default)

Query today's data and exit:
```bash
./enphase-monitor
```

Query a Past Period:
```bash
./enphase-monitor --date 2026-01-15
```

Query an entire month:
```bash
./enphase-monitor --date 2026-01
```

Query an entire year:
```bash
./enphase-monitor --date 2025
```

> **Note:** When querying a Past Period, the program automatically runs once since Past Period data is immutable.

### True-Up Report

Calculate the energy balance for a True-Up Period. Provide the True-Up Start Date shown on your PG&E (or other utility) account:

```bash
./enphase-monitor --true-up 2025-01-15
```

This covers the **True-Up Window**: full calendar months from the first day of the True-Up Start Date's month through the window end. The window end is `min(yesterday, start_month + 12 months − 1 day)` — for a Current Period the end moves daily; for a Past True-Up Period it is fixed at the last day of the 12-month window. For example, a True-Up Start Date of `2025-01-15` covers January 2025 through December 31 2025 once the True-Up Period closes.

**How the query works:**

A single API batch of 8 calls (2 Systems × 4 metrics: Grid Import, Grid Export, Production, Consumption) is made against the `_lifetime` endpoints (Lifetime Data) with `start_date` set to the first day of the True-Up Start Date's month. Battery data is not fetched in True-Up Mode. The API returns all daily values from that date onward; the application sums only the values within the True-Up Window client-side. No inter-batch waits are needed. Subsequent runs are instant because the response is served from the Cache.

**The `--true-up` flag takes precedence over `--date`** — if both are provided, `--date` is ignored.

### Continuous Monitoring

Monitor with auto-refresh (uses `refresh_interval` from config):
```bash
./enphase-monitor --continuous
```

The application will query all systems at the configured `refresh_interval` (default: 3600 seconds = 1 hour) and display updated metrics.

**Restrictions**: `--continuous` only applies to today's Day query. Month, Year, Past Period, and True-Up Mode queries are silently downgraded to run once and exit — Past Period data is immutable, and True-Up Mode uses a dedicated single-batch path that always runs once.

**Cache Mode recommendation**: Use the default Auto Cache Mode (no `--cache` or `--no-cache` flag). When the API Budget is temporarily exhausted at a refresh tick, Auto mode gracefully serves the most recent cached data. With `--no-cache`, budget exhaustion triggers a live call that is likely to 429; on a 429, the program falls back to cache if available (with a warning), or exits with an error if no cache exists.

Press `Ctrl+C` to stop.

### Historical Backfill

Build a local dataset of per-day energy + weather records for offline analysis (e.g. correlating production/consumption with weather):
```bash
# Fetch every day from this date through yesterday into history/
./enphase-monitor --backfill-from 2025-06-19

# Bound the range with an explicit end date
./enphase-monitor --backfill-from 2025-06-19 --date 2026-01-15

# Re-fetch and overwrite days already on disk
./enphase-monitor --backfill-from 2025-06-19 --force
```

Backfill walks the date range one calendar day at a time and writes one JSON file per day to `history/<YYYY-MM-DD>.json`. Key behaviors:

- **Always live.** Backfill disables the cache for the run and pulls fresh from the API, so the records are authoritative. With the credential pool's combined budget, a full year of daily queries is comfortably within limits.
- **Idempotent.** Days that already have a `history/` file are skipped (no API calls) unless `--force` is given.
- **Resilient.** A failure on one day is reported and skipped; the range continues.
- **Progress** redraws in place on a terminal (one advancing line), or falls back to plain lines when output is redirected.

Backfill is the **only** writer of History Records — a plain `./enphase-monitor --date 2026-01-15` stays a read-only terminal report and writes nothing. To capture a single day to `history/`, run `./enphase-monitor --backfill-from 2026-01-15 --date 2026-01-15`. This keeps every record authoritative (live-sourced) and avoids a casual cache-served `--date` view silently shadowing a date that backfill would then skip.

Each backfill also (re)writes a manifest, `history/.index.json`, by **scanning the directory** — so it reflects the whole dataset (this run plus prior runs), not just the latest run. It lists the covered date range, present/missing counts, and every missing day with its reason (the API error for days the run attempted, or "not attempted in last run" otherwise) so gaps in the dataset are visible without diffing the directory:
```json
{
  "updated_at": "2026-06-20T14:03:00-07:00",
  "range": { "from": "2025-06-19", "to": "2026-06-19" },
  "counts": { "present": 360, "missing": 5 },
  "missing": [ { "date": "2025-12-03", "error": "API request failed with status 503" } ]
}
```
The manifest is a dotfile so it stays out of a `history/*.json` glob when you feed the dataset to your analysis tool. It is best-effort as of the last backfill — hand-deleting a record afterward won't update it until the next run.

Each record's `weather_code` is the precise WMO interpretation code — the stable categorical feature for modeling, which (unlike the human `condition` label) distinguishes intensities (e.g. 61/63/65 = slight/moderate/heavy rain) and never drifts on rewording. Decode it with `weather_codes.json`, written at the project root by `--init` (see [Initialization & Weather](#initialization--weather)).

**Record schema** (`history/<date>.json`):
```json
{
  "date": "2026-01-15",
  "totals": { "production_kwh": 44.6, "consumption_kwh": 49.1, "grid_import_kwh": 34.7, "grid_export_kwh": 24.3, "net_flow_kwh": 10.4 },
  "systems": [
    { "id": "5525881", "name": "Right Subpanel", "production_kwh": 20.8, "consumption_kwh": 26.7, "grid_import_kwh": 18.4, "grid_export_kwh": 11.0, "net_flow_kwh": 7.4 }
  ],
  "weather": { "temp_high": 58.2, "temp_low": 42.1, "temp_unit": "°F", "weather_code": 2, "condition": "Partly Cloudy", "cloud_cover_pct": 34, "precipitation_mm": 0, "solar_radiation_kwh_m2": 2.4 }
}
```
Battery charge/discharge/SOC are intentionally omitted: they are unavailable for historical dates (the lifetime endpoints that serve past days carry no battery data). Every backfilled record carries a `weather` object — a day whose weather is unavailable is treated as a failure and is not written (it appears in the manifest's `missing` and is retried on a plain re-run), so the dataset stays usable for correlation. (The field is `omitempty` at the schema level, but Backfill Mode never emits a weatherless record.)

### Validation Mode

Validate cached metrics against a pre-recorded set of expected values:
```bash
./enphase-monitor --test --date 2026-01-19
```

Serves the report entirely from cache (no live API calls) and compares each metric against expected values stored in `test-data/`. Exits non-zero if any metric diverges. Requires `--date` to identify which expected-values file to load. Use this after code changes to confirm calculation logic has not drifted.

### Command-Line Options

- `--config <path>` - Path to configuration file (default: `config.yaml`)
- `--credentials <path>` - Path to credentials file (default: `credentials.yaml`)
- `--init` - Resolve and cache the systems' location for weather reporting. Run once before normal use; re-run if the cache is cleared. Required before any report mode (see [Initialization & Weather](#initialization--weather))
- `--force` - With `--init`, re-resolve the location from the API even if a cached value already exists. With `--backfill-from`, re-fetch and overwrite history records that already exist instead of skipping them
- `--continuous` - Run continuously with periodic refresh (default is run once and exit)
- `--date <YYYY-MM-DD|YYYY-MM|YYYY>` - Query specific date, month, or year (e.g., `2026-01-15`, `2026-01`, or `2025`)
- `--backfill-from <YYYY-MM-DD>` - Backfill Mode: fetch each day from this date through `--date` (or yesterday) with live API calls, writing one JSON record per day into `history/`. Skips days already written unless `--force` is given. Cannot be combined with `--continuous`, `--true-up`, or `--init` (see [Historical Backfill](#historical-backfill))
- `--true-up <YYYY-MM-DD>` - Activate True-Up Mode using this utility True-Up Start Date. Covers the 12-month True-Up Window (Current Period: through yesterday; Past True-Up Period: through last day of 12-month window). Takes precedence over `--date`
- `--update-refresh-token [name]` - Run OAuth setup wizard (one-time for developer plan); pass a credential name when more than one is configured (e.g. `--update-refresh-token enphase-monitor-002`)
- `--all` - With `--update-refresh-token`, re-authorize every configured credential in turn (e.g. `--update-refresh-token --all`)
- `--test` - Validation Mode: use cache only, no live API calls, validate against expected values
- `--no-cache` - Bypass cache and make live API calls (falls back to cache on 429)
- `--cache` - Serve report from cache only; print diagnostic listing missing endpoints if cache is incomplete
- `--clear-cache` - Clear cached API responses for today's date only
- `--clear-cache-date YYYY-MM-DD` - Clear cached API responses for a specific past date (matches the query start date exactly)
- `--clear-all-cache` - Clear all cached API responses (all dates)
- `--debug` - Print debug information: last run time, API budget remaining, and per-request cache/live decisions

### Examples

```bash
# Use custom config file
./enphase-monitor --config /path/to/my-config.yaml

# Query last week's data
./enphase-monitor --date 2026-01-12

# Continuous monitoring with periodic refresh
./enphase-monitor --continuous

# True-Up report starting January 15, 2025
./enphase-monitor --true-up 2025-01-15
```

## Output Format

The application displays:

```
=========================================================
  ENPHASE MULTI-SYSTEM MONITOR
=========================================================
  Query Range:    Tue Jan 20, 2026 12:00 AM
                          to
                  Tue Jan 20, 2026 11:59 PM

  Last Updated:   Sat Jan 24, 2026 09:52:18 PM (cached)
=========================================================

 COMBINED ENERGY REPORT
---------------------------------------------------------
    Net Flow:        15.3 kWh (import)
    Production:      33.4 kWh
    Consumption:     48.6 kWh
    Grid Import:     19.1 kWh
    Grid Export:      3.8 kWh

 INDIVIDUAL SYSTEMS REPORT
---------------------------------------------------------

  [1] Right Subpanel (5525881)
      Net Flow:                   19.3 kWh (import)
      Production:                 14.6 kWh
      Consumption:                32.1 kWh
      Grid Import:                23.1 kWh
      Grid Export:                3.8 kWh
      Battery Charge:             8.5 kWh
      Battery Discharge:          6.8 kWh
      Battery State of Charge:    63%

  [2] Left Subpanel (5392556)
      Net Flow:                   0.2 kWh (export)
      Production:                 18.9 kWh
      Consumption:                16.4 kWh
      Grid Import:                7.5 kWh
      Grid Export:                7.6 kWh
      Battery Charge:             8.1 kWh
      Battery Discharge:          5.4 kWh
      Battery State of Charge:    74%

=========================================================
```

Note: Colors are customizable via `config.yaml` (see Color Customization section below).

### True-Up Report Output

The `--true-up` flag produces a dedicated report:

```
=========================================================
  ENPHASE MULTI-SYSTEM MONITOR
=========================================================
     True-Up Start:  Wed Jan 15, 2025

       Query Range:  Wed Jan 1, 2025 12:00 AM
                             to
                     Fri Apr 24, 2026 11:59 PM

      Last Updated:  Mon May 18, 2026 10:24:41 PM (live)
=========================================================

 TRUE-UP ENERGY REPORT
---------------------------------------------------------
    Production:     3456.7 kWh
    Consumption:    2345.6 kWh
    Grid Import:    800.0 kWh
    Grid Export:    1911.1 kWh
    Net Flow:       1111.1 kWh (export)

 INDIVIDUAL SYSTEMS REPORT
---------------------------------------------------------

  [1] Right Subpanel (5525881)
      Net Flow:         600.0 kWh (export)
      Production:       1,700.0 kWh
      Consumption:      1,100.0 kWh
      Grid Import:      600.0 kWh
      Grid Export:      1,200.0 kWh

  [2] Left Subpanel (5392556)
      ...

=========================================================
```

The "True-Up Start" line shows the True-Up Start Date you provided. The "Query Range" starts from the first day of that month (full calendar months are always used) and ends at the True-Up Window end. Battery metrics are excluded from True-Up reports.

## Metrics Explained

### Combined Energy Report (Site totals)
- **Production**: Production summed across all Systems at the Site (kWh)
- **Consumption**: Consumption summed across all Systems (kWh)
- **Grid Import**: Grid Import summed across all Systems (kWh)
- **Grid Export**: Grid Export summed across all Systems (kWh)
- **Net Flow**: Site-level Net Flow = Grid Import − Grid Export
  - Positive value with `(import)` suffix: net direction is import
  - Negative value with `(export)` suffix: net direction is export

### Individual System Metrics (Standard Report)
- **Net Flow**: Net Flow for this System (kWh) with `(import)` or `(export)` direction suffix
- **Production**: Production for this System (kWh)
- **Consumption**: Consumption for this System (kWh)
- **Grid Import**: Grid Import for this System (kWh)
- **Grid Export**: Grid Export for this System (kWh)
- **Battery Charge**: Battery Charge for this System (kWh). Shown only for today's live Day Mode query (i.e., running without `--date`). Omitted for Past Period Day Mode queries and all Month, Year, and True-Up Mode queries.
- **Battery Discharge**: Battery Discharge for this System (kWh). Shown only for today's live Day Mode query (i.e., running without `--date`). Omitted for Past Period Day Mode queries and all Month, Year, and True-Up Mode queries.
- **State of Charge (SOC)**: Battery State of Charge for this System, 0–100 (percent). Shown per-System only (not aggregated) and **only for today's live Day Mode query** (i.e., running without `--date`). Past Period Day Mode queries and all Month, Year, and True-Up Mode queries omit battery metrics — SOC is a point-in-time reading that is not meaningful for past or multi-day periods.

### Individual System Metrics (True-Up Report)

The True-Up report shows the same per-System breakdown but **without battery metrics**, since Battery Charge and Discharge are not part of the utility True-Up calculation:
- **Production**, **Consumption**, **Grid Import**, **Grid Export**, **Net Flow**

## Color Customization

You can customize the colors used in the terminal output by adding a `colors` section to your `config.yaml`:

```yaml
colors:
  production: "#f0b57c"             # Solar Production
  discharge: "#7acf38"              # Battery Discharge
  import: "#f63cb1"                 # Grid Import
  export: "#06b6de"                 # Grid Export
  net_import_background: "#010469"  # Net Flow line background highlight (import direction) — 24-bit truecolor
  net_export_background: "#7D0069"  # Net Flow line background highlight (export direction) — 24-bit truecolor
  net_import: "#f63cb1"             # Net Flow (foreground color when net is import)
  net_export: "#06b6de"             # Net Flow (foreground color when net is export)
  headers: "#f37320"                # Report Headers
  charge: "#7acf38"                 # Battery Charge
  total_consumed: "#f37320"         # Total Consumed
  secondary_text: "#808080"         # Secondary Text
  primary_text: "#ffffff"           # Primary Text
  error: "#ff0000"                  # Error Text
```

**Color Format Options:**
- **Hex codes** (e.g., `#FF5733`): Automatically converted to ANSI color codes.
  - **Foreground colors** (every key except `net_import_background` / `net_export_background`) render through the ANSI 256-color cube (`\033[38;5;Nm`). 216 cells total — the eye tolerates this for text.
  - **Background colors** (`net_import_background`, `net_export_background`) render as 24-bit truecolor (`\033[48;2;R;G;Bm`) so a solid fill matches the requested hex exactly. Quantizing backgrounds to the 6×6×6 cube is visibly off.
- **ANSI escape codes** (e.g., `\033[38;5;208m` for foreground, `\033[48;2;R;G;Bm` for background): Used directly as-is. Use this form if you need a specific 24-bit foreground color (no quantization).

**Note:** `Reset` and `Bold` are constants defined in `constants.go` and cannot be customized.

## Troubleshooting

### "no credentials configured"
- Make sure you have copied `credentials.yaml.example` to `credentials.yaml`
- Verify each entry under `credentials:` has a unique `name` plus `key`, `client_id`, and `client_secret`
- For developer plan, complete OAuth setup with `--update-refresh-token` to get `refresh_token`

### "API request failed with status 401"
- Your refresh token may have expired or been revoked
- Re-run OAuth setup: `./enphase-monitor --update-refresh-token`
- Verify your API credentials are correct

### "API request failed with status 404"
- Verify your System IDs are correct (check Enlighten URLs for each system)
- Check that you have access to all systems in Enlighten
- Ensure the system IDs are strings (quoted in YAML)

### "rate limit exceeded (429)"
- The API Budget is 10 calls per minute
- The program will display how many seconds to wait before retrying
- Consider increasing `refresh_interval` in your config
- Use `--test` mode to validate against cached data without making API calls
- Run with `--debug` to see when the 60-second window resets and how much budget remains

### "API request failed with status 422"
- This usually means the requested date is in the future
- The program automatically caps dates to the current time
- Try querying a past date: `--date 2026-01-15`

### No Interval Data returned
- Some data may not be available for very recent time periods (try querying yesterday's data)
- Ensure your systems are actively reporting to Enlighten
- Use `--cache` to check what data is available in the cache without making API calls

## Caching and API Budget

### API Budget

The Enphase Enlighten Cloud API v4 enforces a strict API Budget:
- **10 requests per minute** per API key
- **1000 requests per month** total (free developer plan)

**⚠️ Refresh Interval Recommendation**: 

The `refresh_interval` setting controls how often the application queries the API in continuous mode. To stay within the API Budget:

- **Recommended**: `refresh_interval: 3600` (1 hour)
- **Why**: Each today's Day Mode query fetches 5 metrics per System (Production, Consumption, Grid Import, Grid Export, battery). With 2 Systems that is exactly 10 requests per cycle — right at the limit. At 3600 seconds, that is ~10 requests per hour, well within limits. (Past Period Day, Month, Year, and True-Up Mode queries omit battery and use 4 calls per System.)
- **Enforced floor**: Values below 60 seconds (e.g., `refresh_interval: 5`) are automatically clamped up to 60 seconds — one API Budget window — so they cannot exhaust the budget on every tick. In continuous mode a warning is printed when this clamping happens.
- **Calculation**: If you have N Systems, each today's Day Mode query makes N×5 requests. With 2 Systems that is 10/cycle — at the clamped 60-second floor, that is 10 requests/minute, exactly the limit. (Without the floor, `refresh_interval: 5` would have attempted 10×12 = 120 requests/minute.)

### Caching Strategy

To respect these limits, the application combines disk caching with a sliding-window API Budget counter and a live-first serving policy:

- **Automatic Disk Caching**: All API responses are cached as JSON files in the `cache/` directory.
- **Live-First for Current Periods, Cached for Past Periods**:
  - **Past Period Day / Month / Year / True-Up** — always served from Cache. The data is immutable; a live call would waste budget and return identical results.
  - **Current Period queries** (today's Day Mode query, month-to-date, year-to-date, Current Period True-Up) — a live API call is made whenever budget allows. Cache is the fallback only when the budget is exhausted.
- **Sliding-Window API Budget Counter**: Every live API response appends its timestamp to `cache/api_calls`. The available budget at any moment is `10 − count_of_timestamps_in_last_60s`. When budget reaches 0 the client falls back to cache (exact-URL match, then cross-endpoint same-system match at any age); if no cache exists, it short-circuits to the 429 "wait 60 seconds" message instead of issuing a guaranteed-failed call.
- **Default Refresh Interval**: 1 hour (3600 seconds) — queries each system once per hour in continuous mode.
- **429 / 503 Fallback**: If the API returns a rate-limit or service-unavailable error, any cached data is served — first an exact-URL match, then any prior cache for the same endpoint and system regardless of age. If no cache exists at all, the program surfaces the error to the user.

### Cache File Format and Naming

Cache files are stored in the `cache/` directory with the following structure:

**File Naming Scheme:**
- Each cached API response is stored as a JSON file
- Filenames are SHA-256 hashes of the normalized API URL (including query parameters)
- Format: `{hash}.json` where `{hash}` is a 64-character hexadecimal string
- Example: `a1b2c3d4e5f6...{64 chars}.json`

**Cache File Structure:**
Each cache file contains:
- `status_code`: HTTP status code from the API response
- `headers`: HTTP response headers (as key-value pairs)
- `body`: Raw API response body (JSON bytes)
- `cached_at`: Timestamp when the response was cached (ISO 8601 format)
- `queried_date`: The date that was queried (YYYY-MM-DD format), if applicable
- `endpoint`: API endpoint (e.g. `telemetry/production_meter`, `energy_lifetime`), used for cross-date lookup when the API Budget is exhausted
- `system_id`: System ID from the request URL, paired with `endpoint` for the cross-date lookup

**API Budget Counter:**
The cache directory also contains an `api_calls` file (no JSON extension) holding newline-separated RFC3339 timestamps of recent live API responses. Entries older than 60 seconds are pruned on each write. The remaining count is used to compute the available API Budget. The file is ignored by cache listing/clearing commands.

**Cache Key Generation:**
- Cache keys are generated from normalized API URLs
- URLs are normalized to ensure consistent caching (timestamps converted to date strings)
- The same API query will always produce the same cache key, regardless of when it's made

**Cache Lookup:**
- When making an API request, the application first checks for a cached response
- If a cache file exists and is valid, the cached response is used (no API call)
- For past dates, cached data is always preferred to avoid unnecessary API calls

### Cache Management Commands

```bash
# Clear today's cache only (preserves data for past dates)
./enphase-monitor --clear-cache

# Clear cache for a specific past date (forces a re-fetch of that day)
./enphase-monitor --clear-cache-date 2026-01-19

# Clear all cached data
./enphase-monitor --clear-all-cache

# Disable caching (always make live API calls)
./enphase-monitor --no-cache

# Validation Mode (use cache only, no live API calls)
./enphase-monitor --test --date 2026-01-15
```

### Debug Mode

The `--debug` flag prints a startup status line plus a trace of every cache/live decision to stderr:

```bash
./enphase-monitor --debug
```

Sample output:

```
[DEBUG] --- startup ---
[DEBUG] current time : 2026-05-19 09:14:22 PDT
[DEBUG] last API call: 09:13:55 PDT (27s ago)
[DEBUG] rate window resets in: 33s
[DEBUG] API budget   : 0/10 calls remaining
[DEBUG] ---
[DEBUG] serving cache (Past Period, age 6h12m0s): https://api.../energy_lifetime?...
[DEBUG] budget exhausted (0/10), falling back to cache: https://api.../telemetry/production_meter?...
[DEBUG] serving cache (budget exhausted, age 14m3s)
```

Use it when you want to understand *why* a query returned cached vs live data, *when* the API Budget window will reset, or *how much* of the 10 calls/60s budget remains. Debug mode also: prints a `CACHE MODE` banner when `--cache` is used; prints a `WARNING: Insufficient API budget` preflight message when the budget is low; and suppresses the screen-clearing escape sequence so the trace stays visible on subsequent runs.

### Cached Mode

The `--cache` flag serves a report entirely from on-disk Cache without making any live API calls. If the Cache is complete it runs the report; if any required endpoint is missing it prints a per-System diagnostic and exits non-zero:

```bash
./enphase-monitor --cache
./enphase-monitor --cache --date 2026-05
./enphase-monitor --cache --true-up 2025-01-15
```

**When cache is complete**, the report runs normally. Adding `--debug` also prints a `CACHE MODE: Serving report from cache, no live API calls` banner.

**When cache is incomplete**, the diagnostic lists each endpoint per system with its status and age:

```
CACHE INCOMPLETE for today:

  System "Left Subpanel" (5392556):
    ✓  energy_import_telemetry              cached 3h12m ago
    ✓  energy_export_telemetry              cached 3h12m ago
    ✗  telemetry/production_meter           not cached
    ✓  telemetry/consumption_meter          cached 3h12m ago
    -  telemetry/battery                    not cached (optional)

To populate the cache, run:
  ./enphase-monitor
```

`✓` = cached (age shown), `✗` = required but missing, `-` = optional and missing.

Use `--cache` when you want to verify what data is available locally before making API calls, or when you are offline and want to review data for past dates.

### Best Practices

1. **Use Default Refresh Interval**: Use 1 hour to stay well within the API Budget
2. **Leverage Caching**: Query past dates frequently - they use cached data (no API calls)
3. **Validation Mode**: Use `--test` flag for validation against cached data without hitting the API
4. **Monitor Cache**: Use `--cache` to check what's available before querying

## Documentation

This project includes comprehensive documentation for different learning paths:

### For Getting Started
- **[QUICKSTART.md](QUICKSTART.md)** - Get up and running in 5 minutes
- **[OAUTH_SETUP.md](docs/OAUTH_SETUP.md)** - Complete OAuth 2.0 setup guide with detailed explanations

### For Understanding the Codebase
- **[CONTEXT.md](CONTEXT.md)** - Domain glossary: the authoritative source of terminology (Site, System, Query Mode, True-Up Window, Net Flow, API Budget, etc.). Read this before contributing — it pins down what to call things and what to *avoid* calling them.
- **[ARCHITECTURE.md](docs/ARCHITECTURE.md)** - System architecture, execution flow, and design patterns
- **[docs/adr/](docs/adr/)** - Architecture Decision Records capturing significant design choices (e.g. the True-Up Window end-date rule)
- **[GO_BEST_PRACTICES.md](docs/GO_BEST_PRACTICES.md)** - Go concepts and patterns used in this codebase
- **[GO_CONCEPTS.md](docs/GO_CONCEPTS.md)** - Quick reference for Go concepts used in the code, including channels and signals

### Recommended Learning Path

1. **New Users**: Start with [QUICKSTART.md](QUICKSTART.md)
2. **OAuth Setup**: Follow [OAUTH_SETUP.md](docs/OAUTH_SETUP.md) for authentication
3. **Understanding Go**: Read [GO_BEST_PRACTICES.md](docs/GO_BEST_PRACTICES.md) for Go concepts
4. **System Design**: Study [ARCHITECTURE.md](docs/ARCHITECTURE.md) to understand the codebase structure
5. **Advanced Topics**: Explore [GO_CONCEPTS.md](docs/GO_CONCEPTS.md#channels-and-signals) for channels, signals, and concurrency patterns

## Project Structure

```
enphase-monitor/
├── main.go                                  # Application entry point (orchestration only)
├── internal/                                # Internal packages
│   ├── aggregator/                          # Multi-system data aggregation
│   │   ├── types.go                         # Metric data structures
│   │   ├── aggregator.go                    # Aggregation logic with dependency injection
│   │   ├── aggregator_test.go               # Aggregator tests with mock clients
│   │   └── aggregator_bench_test.go         # Benchmark tests
│   ├── api/                                 # HTTP client for Cloud API v4
│   │   ├── client.go                        # Enlighten Cloud API client
│   │   ├── types.go                         # API request/response types
│   │   ├── cache_check.go                   # Per-system/endpoint cache availability check (--cache mode)
│   │   ├── client_test.go                   # API client unit tests
│   │   ├── client_caching_test.go           # Characterization tests for makeCachedAPIRequest fallback branches (validation/no-cache modes, 429/503/network-error cache fallbacks)
│   │   ├── client_functional_test.go        # Functional tests with mock HTTP servers
│   │   ├── client_lifetime_test.go          # Lifetime Data tests (Month, Year, True-Up Mode queries)
│   │   ├── preflight_test.go                # Budget-exhaustion cache-fallback tests (all 8 Query Mode × Period combinations)
│   │   ├── query_cost_test.go               # QueryCost unit tests (all Query Mode × hasBattery combos)
│   │   └── testmain_test.go                 # TestMain: redirects cache I/O to temp dir for all api tests
│   ├── app/                                 # Application execution logic
│   │   ├── setup.go                         # App initialization & configuration
│   │   ├── setup_test.go                    # Setup tests
│   │   ├── runner.go                        # Execution modes (once/continuous)
│   │   ├── runner_test.go                   # Runner tests
│   │   ├── trueup.go                        # True-Up Mode: single-batch Lifetime Data query and report conversion
│   │   ├── trueup_test.go                   # True-up logic tests
│   │   ├── backfill.go                      # Backfill Mode: live per-day fetch over a date range into history/
│   │   ├── weather.go                       # Best-effort weather enrichment for Day-Mode reports
│   │   └── cache_report.go                  # --cache mode: completeness check and diagnostic output
│   ├── cache/                               # Disk-based response caching
│   │   ├── cache.go                         # Cache implementation + sliding-window budget
│   │   ├── cache_test.go                    # Cache state management tests (ValidationMode, CacheDisabled, BudgetWarningShown, ResetState)
│   │   ├── cache_functions_test.go          # Core caching tests (URL normalization, key generation, save/load, HasCacheForDate)
│   │   ├── api_budget_test.go               # Sliding-window API Budget counter tests (RecordAPICall, RemainingBudget, pruning)
│   │   ├── cli.go                           # Cache inspection utilities
│   │   └── cli_test.go                      # CLI utilities tests
│   ├── cli/                                 # Command-line interface
│   │   ├── flags.go                         # CLI flag parsing
│   │   ├── flags_test.go                    # Flag parsing tests
│   │   ├── cache_commands.go                # Cache management commands
│   │   └── cache_commands_test.go           # Cache commands tests
│   ├── config/                              # Configuration types
│   │   ├── config.go                        # YAML loading & validation (uses type aliases)
│   │   ├── config_test.go                   # Configuration tests
│   │   ├── credentials.go                   # Loads & validates the credentials: pool
│   │   └── credentials_test.go              # Credentials loading/validation tests
│   ├── constants/                           # Centralized constants
│   │   ├── constants.go                     # Application-wide constants
│   │   └── constants_test.go                # Constants tests
│   ├── credentials/                         # Credential pool: spread + 429 failover
│   │   ├── pool.go                          # Round-robin assignment, cooldown, failover
│   │   └── pool_test.go                     # Pool selection/failover tests
│   ├── display/                             # Terminal output formatting
│   │   ├── display.go                       # Display with io.Writer injection
│   │   └── display_test.go                  # Display output tests
│   ├── geocode/                             # ZIP/postal code → coordinates (Zippopotam.us)
│   │   ├── geocode.go                       # ZIP lookup for weather geolocation
│   │   └── geocode_test.go                  # Geocode tests
│   ├── history/                             # Per-day energy+weather JSON records (history/)
│   │   ├── history.go                       # DayRecord schema, FromMetrics, WriteRecord
│   │   └── history_test.go                  # History mapping and write tests
│   ├── location/                            # Resolve & cache systems' coordinates (--init)
│   │   ├── location.go                      # Location resolver with disk cache
│   │   └── location_test.go                 # Location resolver tests
│   ├── oauth/                               # OAuth 2.0 authentication
│   │   ├── oauth.go                         # Token management & refresh
│   │   ├── authorization.go                 # Interactive OAuth authorization wizard
│   │   ├── oauth_test.go                    # Basic unit tests
│   │   ├── oauth_functional_test.go         # Integration tests with mock servers
│   │   └── oauth_edge_cases_test.go         # Edge case tests
│   ├── parser/                              # JSON telemetry parsing
│   │   ├── parser.go                        # Response parsing utilities
│   │   ├── parser_test.go                   # Parser tests
│   │   └── parser_bench_test.go             # Benchmark tests
│   ├── timezone/                            # Timezone handling
│   │   ├── timezone.go                      # Timezone utilities
│   │   └── timezone_test.go                 # Timezone tests
│   ├── types/                               # Shared type definitions
│   │   └── types.go                         # SystemConfig, APIConfig (breaks circular deps)
│   ├── urlbuilder/                          # API URL construction
│   │   ├── urlbuilder.go                    # URL building helpers
│   │   └── urlbuilder_test.go               # URL builder tests
│   ├── validation/                          # Validation Mode (--test flag)
│   │   ├── validation.go                    # Metrics validation logic (uses io.Writer for testability)
│   │   ├── validation_test.go               # Unit tests (tolerance calculations, edge cases)
│   │   └── validation_integration_test.go   # Integration tests (real expected values)
│   └── weather/                             # Open-Meteo daily/current weather client
│       ├── weather.go                       # DailyWeather/CurrentWeather fetch + WMO code mapping
│       └── weather_test.go                  # Weather client tests
├── docs/                                    # Project documentation
│   ├── ARCHITECTURE.md                      # Architecture documentation
│   ├── GO_BEST_PRACTICES.md                 # Go best practices guide
│   ├── GO_CONCEPTS.md                       # Go concepts reference (channels, signals, and more)
│   ├── OAUTH_SETUP.md                       # OAuth setup documentation (detailed)
│   ├── TESTING.md                           # Testing patterns and guidelines
│   └── adr/                                 # Architecture Decision Records
│       ├── 0001-true-up-window-end-date.md  # ADR for the True-Up Window end-date rule
│       └── 0002-init-guard-requires-weather-init.md  # ADR: report modes require --init (weather)
├── test-data/                               # Test validation data
│   └── enphase_api_*.json                   # Expected values for validation
├── config.yaml.example                      # Example non-secret configuration (systems, colors, etc.)
├── config.yaml                              # Your actual configuration (create from example)
├── credentials.yaml.example                 # Example credentials (credentials: list)
├── credentials.yaml                         # Your actual API secrets (create from example; gitignored)
├── cache/                                   # Cached API responses (created at runtime)
├── history/                                 # Per-day energy+weather JSON records (created by Backfill Mode)
├── weather_codes.json                       # WMO weather-code legend (written by --init)
├── go.mod                                   # Go module definition
├── go.sum                                   # Go module checksums
├── scripts/                                 # Utility scripts
│   ├── run-tests.sh                         # Test runner script
│   └── history.py                           # Git history inspection helper
├── Makefile                                 # Build automation
├── CONTEXT.md                               # Domain glossary (project terminology, "avoid" terms)
├── README.md                                # This file
└── QUICKSTART.md                            # Quick start guide
```

## Testing

The project includes a comprehensive test suite with **70.6% code coverage** across all packages. The test suite validates both functionality and metrics against expected values, enabling rapid iteration without exhausting the API Budget.

### Test Coverage by Package

| Package | Coverage | Status |
|---------|----------|--------|
| urlbuilder | 100.0% | ✅ |
| constants | 100.0% | ✅ |
| display | 98.7% | ✅ |
| validation | 95.5% | ✅ |
| parser | 94.8% | ✅ |
| credentials | 94.6% | ✅ |
| timezone | 92.7% | ✅ |
| cli | 91.4% | ✅ |
| aggregator | 88.9% | ✅ |
| weather | 87.5% | ✅ |
| history | 83.3% | ✅ |
| config | 82.1% | ✅ |
| geocode | 81.5% | ✅ |
| location | 81.0% | ✅ |
| oauth | 80.1% | ✅ |
| api | 77.1% | ✅ |
| cache | 71.6% | ✅ |
| app | 40.1% | ⚠️ |

**Total: 70.4% coverage** (exceeds typical Go project standards of 50-60%; `app` covers orchestration glue — including `RunBackfill` — that is exercised more thoroughly via the api-package integration tests)

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Run specific package tests
go test ./internal/parser -v

# Run benchmarks (see Performance section below)
go test -bench=. ./internal/parser
go test -bench=. ./internal/aggregator
```

### Lint and CI

Run the linter (errcheck, goimports, revive, govet, staticcheck) before committing:

```bash
make lint
```

CI is not yet configured. Run `make lint` and `go test ./...` locally before committing.

### Validation Mode

Run in Validation Mode using only cached responses (no live API calls):

```bash
./enphase-monitor --test --date 2026-01-20
```

This will:
1. Use only cached API responses (no live calls)
2. Validate results against expected values
3. Show a detailed comparison report

#### Early Validation

The `--test` flag includes early validation to prevent confusing errors:

**Missing cache:**
```
ERROR: --test flag requires cached data, but no cache exists for 2026-01-30.

To populate the cache, run:
  ./enphase-monitor

Then retry with --test.
```

**Missing expected values file:**
```
Validation failed: no expected values file found for 2026-01-01.

To run validation, create the file:
  test-data/enphase_api_2026-01-01.json

Example format:
  { "date": "2026-01-01", "systems": [...] }

To run without validation, omit the --test flag:
  ./enphase-monitor --date 2026-01-01
```

### Setting Up Test Data

**IMPORTANT:** The cache must contain data for the specific test date. Old cache files from different dates will cause incorrect results.

1. **Run all tests** (recommended):
   ```bash
   ./scripts/run-tests.sh
   ```
   This script will:
   - Check which test dates have cached responses
   - Generate missing cache by making live API calls (waiting 60 seconds between calls to respect the API Budget)
   - Run validation tests for all dates (2026-01-14 through 2026-01-20)
   - Display a summary of all test results

2. **Or manually generate cache for a specific date**:
   ```bash
   ./enphase-monitor --date 2026-01-20
   ```
   This will make live API calls and cache the responses in `cache/`

3. **Update `enphase_api_YYYY-MM-DD.json`** with the correct values from the Enphase app

4. **Now you can iterate rapidly using Validation Mode** (uses cache only, no API calls):
   ```bash
   ./enphase-monitor --test --date 2026-01-20
   ```

**Note:** Validation Mode uses the cache from `cache/`. The `scripts/run-tests.sh` script ensures all test dates have cached responses available.

### Expected Values Format

The expected values JSON file should have this structure:

```json
{
  "date": "2026-01-20",
  "systems": [
    {
      "id": "5525881",
      "name": "Right Subpanel",
      "expected": {
        "grid_import": 23.4,
        "grid_export": 3.9,
        "production": 14.6,
        "battery_discharged": 6.8,
        "battery_charged": 8.6,
        "net_flow": 19.6,
        "consumption": 32.3
      }
    }
  ]
}
```

### Validation Tolerance

The validation function compares actual vs expected values with a tolerance of **±10% of the expected value**, with a minimum tolerance of **0.1 kWh** for small values. This means:
- For large values (e.g., 20 kWh): tolerance is ±2.0 kWh (10%)
- For small values (e.g., 0.5 kWh): tolerance is ±0.1 kWh (minimum)

Metrics that differ by more than the calculated tolerance will be marked as failed (❌). The validation report shows:
- Expected value
- Actual value
- Difference
- Percentage difference
- Pass/fail status

## Performance

The codebase includes comprehensive benchmarks for performance-critical paths. Run benchmarks with:

```bash
# Parser benchmarks (JSON parsing, interval summing)
go test -bench=. -benchmem ./internal/parser

# Aggregator benchmarks (multi-system aggregation)
go test -bench=. -benchmem ./internal/aggregator

# All benchmarks with CPU profiling
go test -bench=. -benchmem -cpuprofile=cpu.prof ./internal/...
```

**Key Performance Characteristics:**
- **API Response Caching**: Reduces redundant API calls and respects the API Budget
- **Efficient JSON Parsing**: Optimized for 96 intervals/day (15-min intervals)
- **Multi-System Aggregation**: Scales linearly with number of systems
- **Minimal Dependencies**: Only 1 external library (gopkg.in/yaml.v3 for config); all HTTP and I/O uses the standard library

## Code Quality

This project follows Go best practices and coding standards:

- **Test Coverage**: 70.4% overall, 100% for urlbuilder and constants, 99% for display, 95%+ for validation and parser, 90%+ for timezone, cli, and aggregator
- **Test Suite**: 38 test files across 18 tested packages with comprehensive unit, integration, and edge case tests
- **Go Modules**: Proper dependency management with go.mod/go.sum
- **Error Handling**: Comprehensive error wrapping with context
- **Documentation**: Extensive inline comments and dedicated guides
- **Type Safety**: Strict type checking with no unsafe operations
- **Linting**: Passes golangci-lint with recommended settings
- **Performance**: Benchmarks included for hot paths

**Code Metrics:**
- Total Lines: ~7,600 (excluding tests)
- Test Lines: ~11,600 (comprehensive test suite)
- Packages: 19 internal packages (18 with tests; `types` is a pure type-definition package)
- Test Files: 38 (unit, integration, functional, edge case, and benchmark tests)
- External Dependencies: 1 (gopkg.in/yaml.v3)

## License

This is a personal utility project. Use and modify as needed for your own Enphase monitoring needs.
