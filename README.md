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
- **Weather Enrichment**: Day-Mode reports annotated with temperature, conditions, cloud cover, relative humidity (mean/max/min), precipitation, and solar radiation (Open-Meteo; requires one-time `--init`)
- **Historical Backfill**: Build a per-day energy + weather dataset over a date range as JSON records for offline analysis (`--start-date`); pull PG&E utility-meter records from the web portal alongside Enphase data (`--pge-web-only`)
- **Raw Source Archive & Verification**: Every source response (Enphase API, Open-Meteo, PG&E Green Button) is kept untouched under `raw/` with a SHA-256 manifest, and offline `--verify-*-raw` commands re-derive `history/` from it, so the processing can be checked locally at any time
- **True-Up Report**: Energy metrics across a full utility True-Up Period (`--true-up`)
- **Clean Display**: Formatted terminal output with customizable colors
- **API Caching**: Automatic caching of API responses to reduce API calls
- **Color Customization**: Customize terminal colors using hex codes or ANSI escape codes
- **Cloud API v4**: Uses Enphase Enlighten Cloud API v4 for reliable data access
- **OAuth Support**: Full OAuth 2.0 support for developer plan authentication
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
     - name: enphase-monitor-01
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
- `name`: A unique label for the credential set (the token-cache key)
- `key`: Your API key
- `client_id`: OAuth client ID
- `client_secret`: OAuth client secret
- `refresh_token`: Obtained from OAuth setup (see [OAuth Setup](#oauth-setup) below)

The non-secret `authorization_url` and `redirect_uri` are **not** repeated per credential — they are configured once in `config.yaml` under `api:` and shared by every credential set (a credential may override them by setting its own). `authorization_url` defaults to the Enphase token endpoint if omitted.

Listing more than one credential set lets the app spread the per-key rate limit (10 requests/minute, 1000/month) across systems — assigning the least-used key first — and fail over to a spare key when one is throttled (429).

#### System Configuration

Each system requires:
- `name`: A friendly name for display
- `id`: Your Enphase system ID (see [Finding Your System IDs](#finding-your-system-ids))

#### Optional Settings

- `refresh_interval`: Present in the config schema (default: 3600 seconds = 1 hour). Currently unused — continuous mode was removed.
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

This application uses OAuth 2.0 for authentication. You must complete the OAuth setup to get your refresh tokens, and repeat it when Enphase rejects a stored token (refresh tokens expire after 30 days).

### Quick Setup

```bash
./enphase-monitor --update-refresh-tokens
```
Run the setup wizard, which opens one headed Chrome window and, for each credential in the file:
1. Opens that credential's Enphase authorization page
2. Waits for you to log in to Enlighten (once, on the first credential), then clicks "Allow Access" for you
3. Reads the authorization code from the redirect through the browser's DevTools protocol (no local listener, so your `redirect_uri` need not be reachable) and exchanges it for tokens
4. Writes the refresh token straight into the matching entry in your `credentials.yaml`

There is nothing to copy or paste.

The wizard always re-authorizes **every** credential in the credentials file in turn, logging in once. By default that is `credentials.yaml`; to update another file, name it:

```bash
./enphase-monitor --update-refresh-tokens --credentials-file credentials-mobile.yaml
./enphase-monitor --update-refresh-tokens --credentials-file credentials-desktop.yaml
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
uses this token to get new access tokens when needed. Enphase refresh tokens expire after
30 days and can be replaced when used; the application keeps a replacement in memory for
that run only and does not write it back to `credentials.yaml`, so re-run
`--update-refresh-tokens` when a run fails with a 401 (invalid or expired refresh token).

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

- **Interval Data: Single-Day Only**: Interval Data endpoints (15-minute data) return exactly **one calendar day** of data per request — the API ignores the `end_at` parameter and always responds with `granularity=day`. Querying a full month via Interval Data endpoints would require 28–31 separate calls, which would burn through the API Budget (10 calls/minute and 1000/month per key).
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

`--init` makes a single `/systems` call, geocodes each system's postal code to coordinates, and caches the result. This enables **weather enrichment**: Day-Mode reports are annotated with the day's temperature high/low, conditions, cloud cover, relative humidity (mean/max/min), precipitation, and solar radiation (from the [Open-Meteo](https://open-meteo.com/) API). Resolving the location out of band keeps it off the API Budget on a live reporting run.

`--init` also writes `weather_codes.json` to the project root — the authoritative WMO legend decoding the `weather_code` field carried by reports and History Records (each of the codes Open-Meteo emits, defined individually so intensities are preserved). It is a general reference rather than a dataset artifact, regenerated on each `--init`.

**Initialization is required.** Every report mode (`--date`, `--true-up`, `--start-date`, and the default today query) refuses to run until `--init` has populated the location cache, exiting with:
```
enphase-monitor: not initialized — run `enphase-monitor --init` first.
```
Cache-management commands (`--clear-cache`, etc.) and `--update-refresh-tokens` are exempt. Re-run `--init` if the cache is cleared; add `--force` to re-resolve even when a cached value exists.

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

### Historical Backfill

Build a local dataset of per-day energy + weather records for offline analysis (e.g. correlating production/consumption with weather):
```bash
# Fetch every day from this date through yesterday into history/
./enphase-monitor --start-date 2025-06-19

# Bound the range with an explicit end date
./enphase-monitor --start-date 2025-06-19 --end-date 2026-01-15

# Re-fetch and overwrite days already on disk
./enphase-monitor --start-date 2025-06-19 --force

# Pull only PG&E utility-meter records (no Enphase credentials needed)
./enphase-monitor --start-date 2025-06-19 --pge-web-only

# Pull only Enphase records (skip PG&E sources)
./enphase-monitor --start-date 2025-06-19 --enphase-api-only
```

By default, `--start-date` fetches from **both** Enphase (live API) and PG&E (browser download). Use `--pge-web-only` or `--enphase-api-only` to restrict to one source. The range runs from `--start-date` through `--end-date` (or yesterday when `--end-date` is omitted). Key behaviors:

- **PG&E is capped at one year per run.** PG&E's export form rejects ranges over a year, so a run that includes PG&E ends one year after `--start-date` (e.g. `--start-date 2025-10-01` ends 2026-09-30, and the Enphase backfill is aligned to that end). The tool prints the `--start-date` to continue from; run it again to extend coverage.
- **Always live.** Range pulls disable the cache and pull fresh from the API, so records are authoritative. With the credential pool's combined budget, a full year of daily Enphase queries is comfortably within limits.
- **Idempotent.** Days that already have a record in `history/` are skipped (no API calls) unless `--force` is given. PG&E records are always overwritten regardless of `--force`.
- **Resilient.** A failure on one day is reported and skipped; the range continues.
- **Progress** redraws in place on a terminal (one advancing line), or falls back to plain lines when output is redirected.

Range pull is the **only** writer of History Records — a plain `./enphase-monitor --date 2026-01-15` stays a read-only terminal report and writes nothing. To capture a single day to `history/`, run `./enphase-monitor --start-date 2026-01-15 --end-date 2026-01-15`. This keeps every record authoritative (live-sourced) and avoids a casual cache-served `--date` view silently shadowing a date that a range pull would then skip.

Each run also (re)writes a per-source index, the Backfill Index (`history/.enphase-index.json`, `history/.pge-index.json`), by **scanning the directory** — so it reflects the whole dataset (this run plus prior runs), not just the latest run. It lists the covered date range, present/missing counts, and every missing day with its reason (the API error for days the run attempted, or "not attempted in last run" otherwise) so gaps in the dataset are visible without diffing the directory:
```json
{
  "updated_at": "2026-06-20T14:03:00-07:00",
  "range": { "from": "2025-06-19", "to": "2026-06-19" },
  "counts": { "present": 360, "missing": 5 },
  "missing": [ { "date": "2025-12-03", "error": "API request failed with status 503" } ]
}
```
The index is a dotfile so it stays out of a `history/*.json` glob when you feed the dataset to your analysis tool (as is `history/.weather-provenance.json`, below). It is best-effort as of the last run — hand-deleting a record afterward won't update it until the next run. Not to be confused with the Raw Manifest, `raw/manifest.jsonl`, which lists archived source files.

Each record's `weather_code` is the precise WMO interpretation code — the stable categorical feature for modeling, which (unlike the human `condition` label) distinguishes intensities (e.g. 61/63/65 = slight/moderate/heavy rain) and never drifts on rewording. Decode it with `weather_codes.json`, written at the project root by `--init` (see [Initialization & Weather](#initialization--weather)).

**Enphase record schema** (`history/enphase-<date>.json`):
```json
{
  "date": "2026-01-15",
  "totals": { "production_kwh": 44.6, "consumption_kwh": 49.1, "grid_import_kwh": 34.7, "grid_export_kwh": 24.3, "net_flow_kwh": 10.4 },
  "systems": [
    { "id": "5525881", "name": "Right Subpanel", "production_kwh": 20.8, "consumption_kwh": 26.7, "grid_import_kwh": 18.4, "grid_export_kwh": 11.0, "net_flow_kwh": 7.4 }
  ],
  "weather": { "temp_high": 58.2, "temp_low": 42.1, "temp_unit": "°F", "weather_code": 2, "condition": "Partly Cloudy", "cloud_cover_pct": 34, "humidity_pct": 61, "humidity_max_pct": 88, "humidity_min_pct": 38, "precipitation_mm": 0, "solar_radiation_kwh_m2": 2.4 }
}
```
Battery charge/discharge/SOC are intentionally omitted from Enphase records: they are unavailable for historical dates (the lifetime endpoints that serve past days carry no battery data). Every Enphase record carries a `weather` object — a day whose weather is unavailable is treated as a failure and is not written (it appears in the manifest's `missing` and is retried on a plain re-run), so the dataset stays usable for correlation. (The field is `omitempty` at the schema level, but range pull never emits a weatherless Enphase record.) PG&E records have a different schema and are stored as `history/pge-<date>.json`; they do not carry weather data.

**PG&E record schema** (`history/pge-<date>.json`):
```json
{
  "date": "2025-11-08",
  "import_kwh": 29.724, "export_kwh": 25.675, "net_kwh": 4.049,
  "intervals": [
    { "start": "2025-11-08T00:00:00-08:00", "end": "2025-11-08T00:14:59-08:00", "import_kwh": 0.518, "export_kwh": 0 }
  ]
}
```
Semantics (all verified against real Green Button exports, and pinned by tests):
- **Units are true kWh.** `import_kwh` is *gross* energy delivered from the grid and `export_kwh` is gross energy received back; both are non-negative, and `net_kwh = import_kwh - export_kwh` (negative for a net exporter). No interval has negative import.
- **Timestamps are real local instants** with the UTC offset in force at the time (`-08:00` PST, `-07:00` PDT). A day is local midnight to midnight. PG&E's own stamps are local wall-clock time encoded as UTC; the parser detects that and converts it (see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#pge-feed-decoding)).
- **Intervals run 899 s** (`00:00:00`–`00:14:59`), PG&E's own interval length.
- **Daylight-saving days:** a spring-forward day has 92 intervals; a fall-back day has only 96 of its 100 real slots because PG&E's feed omits the repeated hour (that day is slightly understated, and the pull warns).
- **Feed registers:** an export carries a signed net register, a received register, a demand register in watts and a gross-delivered register. Only gross delivered and received are used; the net register is never added. If a feed does not satisfy `net = delivered − received` the pull fails with an error rather than guessing.

#### Raw source archive (`raw/`)

`history/` is derived data. To make it checkable at any time, every external response is also kept untouched under `raw/<source>/` (gitignored: it contains account data):

| Directory | Contents | Written by |
|---|---|---|
| `raw/enphase/` | Enphase API response bodies, named `<system>_<endpoint>_<date>_<fetch time>.json` | every live Enphase call; `--raw-import-cache` for responses that were already in `cache/` |
| `raw/weather/` | Open-Meteo daily payloads, `daily_<date>_<fetch time>.json` | every live weather fetch; `--raw-refetch-weather` |
| `raw/pge/` | PG&E Green Button downloads as received (the original ZIP), `pge-<from>_<to>.zip` | every PG&E download (`--pge-web-only`, or `--start-date` without a source flag) |
| `raw/manifest.jsonl` | one JSON line per file: `fetched_at`, `source`, `file`, `sha256`, `bytes`, `request` (API keys removed) and an optional `note` | all of the above |

Files are written once and never overwritten: a repeat fetch gets its own timestamped name (a repeated PG&E range gets `-1`, `-2`). Only live fetches archive — a cache hit makes no request and archives nothing, and days a backfill skips as already present add nothing. The `cache/` directory holds the same Enphase bodies but is a cache (clearable); no `--clear-*-cache` command ever touches `raw/`. Back it up outside the project, because git ignores it: `tar czf ~/enphase-data-$(date +%Y%m%d-%H%M).tar.gz raw/ history/`.

**Re-collecting PG&E after a parser change or a suspected bad pull** (PG&E caps one export at a year, so a long span needs two runs):
```bash
tar czf ~/enphase-data-$(date +%Y%m%d-%H%M).tar.gz raw/ history/      # back up first: pulls overwrite pge-*.json
./enphase-monitor --pge-web-only --start-date 2025-10-06 --end-date 2026-10-05
./enphase-monitor --pge-web-only --start-date 2026-10-06              # end defaults to yesterday
./enphase-monitor --verify-pge-raw && ./enphase-monitor --verify-enphase-raw && ./enphase-monitor --verify-weather-raw
```
For a parser fix alone no new pull is needed: `./enphase-monitor --pge-reprocess raw/pge/pge-2025-10-06_2026-10-05.zip` rebuilds the records from the archived download.

The maintenance and verification flags for the archive are described under [Command-Line Options](#command-line-options). Two behaviors worth knowing here:

- **Weather provenance.** Open-Meteo revises recent history, and older records could hold zeros for fields that were not yet fetched, so a cached past day is not guaranteed final. `--raw-apply-weather` rewrites a record's weather from the archived payload and records, per day in `history/.weather-provenance.json`, whether it was `rebuilt_from_raw` (with the replaced values under `previous`) or already `matches_raw`, plus the raw file, its SHA-256 and whether radiation is exactly 0. Either way the stored weather equals the newest payload in `raw/weather/`, which `--verify-weather-raw` confirms.
- **What "verified" means.** Each `--verify-*-raw` command checks integrity (every manifest line re-hashed; altered, missing and unlisted files reported) and reproduction (history re-derived from the raw files with the same code the live run uses, and compared). Passing means `history/` is exactly what the current code derives from what the sources sent, not that the sources were right — see the caveats below.

#### Known data caveats

- **Enphase day totals omit the last 15 minutes.** The Day Mode interval endpoints return one interval fewer than the day's real length (95 on a normal day): the slot ending at midnight falls outside the response window and appears in neither that day's nor the next day's response. Measured against PG&E, that slot is a median 1.4% of a day's grid import and about 0% of export, so Enphase import reads slightly low.
- **Enphase consumption reads about 3 kWh/day below** `grid_import + production - grid_export` (median 3.0, mean 3.4 over the history). This is a metering-scope effect (where the consumption CTs sit), not a parsing error; it is why consumption does not reconcile exactly with the other three metrics.
- **2026-03-08 Enphase is unreliable.** One system's export meter returned zeros for 09:00-16:00 while its production was normal (the response in `raw/enphase/` is that way; the parser reproduces it), giving 20.1 kWh export against PG&E's 47.8 and an identity gap of 36.6 kWh. Prefer PG&E for that day. Other days with large Enphase/PG&E gaps (2025-11-07, 2026-03-18, 2026-09-07, 2026-09-26) are not explained by the raw data; 2025-11-07 is the panel-install day.
- **PG&E daylight-saving days:** 2026-03-08 has 92 intervals (the correct length of that 23-hour day). 2025-11-02 has only 96 of its 100 real slots because PG&E's feed omits the repeated hour, so that day is slightly understated; the pull warns about it.

The PG&E pull prints a warning before writing when a day's import is negative, an interval has import < 0 and export > 0, the interval count differs from the local day's real length (92/96/100), or PG&E's net differs from Enphase's by more than 10%.

### Command-Line Options

- `--config-file <path>` - Path to configuration file (default: `config.yaml`)
- `--init` - Resolve and cache the systems' location for weather reporting. Run once before normal use; re-run if the cache is cleared. Required before any report mode (see [Initialization & Weather](#initialization--weather))
- `--force` - With `--init`, re-resolve the location from the API even if a cached value already exists. With `--start-date`, re-fetch and overwrite Enphase history records that already exist instead of skipping them; has no effect on PG&E records, which are always overwritten
- `--date <YYYY-MM-DD|YYYY-MM|YYYY>` - Query specific date, month, or year (e.g., `2026-01-15`, `2026-01`, or `2025`)
- `--start-date <YYYY-MM-DD>` - Start a range pull from this date through `--end-date` (or yesterday) with live API calls, writing one JSON record per day into `history/`. Fetches from both Enphase API and PG&E web by default; use `--enphase-api-only` or `--pge-web-only` to restrict the source. Skips Enphase days already written unless `--force` is given. Cannot be combined with `--true-up` or `--init` (see [Historical Backfill](#historical-backfill))
- `--end-date <YYYY-MM-DD>` - End date (inclusive) for `--start-date` range pulls. Defaults to yesterday. When PG&E is included, the run is capped at one year after `--start-date` (PG&E's export limit) and the tool prints the `--start-date` to continue from
- `--pge-web-only` - With `--start-date`, pull only from the PG&E website (Green Button browser download). Does not require Enphase credentials
- `--enphase-api-only` - With `--start-date`, pull only from the Enphase API. Skips all PG&E data sources
- Raw archive, reprocessing and verification (grouped first in `--help`; run **one at a time** — combining two is rejected — and none takes `--start-date`/`--end-date` except the two raw weather commands, where either bound may be given alone). See [Raw source archive](#raw-source-archive-raw):
  - `--raw-import-cache` - copy the raw Enphase responses held in `cache/` into `raw/enphase/`, keeping their original fetch times. Offline, no API calls; safe to repeat
  - `--raw-refetch-weather` - re-fetch Open-Meteo for every history day (or a `--start-date`/`--end-date` range) with the weather cache bypassed, archiving each payload in `raw/weather/` and reporting days that differ from the stored record. No key or quota; needs `--init`
  - `--raw-apply-weather` - rebuild the weather block of each `history/enphase-*.json` from the newest archived payload (never touches energy fields) and record what changed in `history/.weather-provenance.json`
  - `--pge-reprocess <file>` - rebuild PG&E history records offline from an archived download (e.g. `raw/pge/pge-2025-10-01_2026-09-30.zip`); the range comes from the file name. Use after a parser fix instead of re-pulling
  - `--verify-pge-raw` - re-hash `raw/pge` against the manifest, re-derive each archive with the current parser and diff against `history/pge-*.json`
  - `--verify-weather-raw` - re-hash `raw/weather` and confirm every record's weather equals what its newest payload yields
  - `--verify-enphase-raw` - re-hash `raw/enphase` and re-derive every day's per-system import/export/production/consumption from the archived responses
  - `--verify-enphase-archive <YYYY-MM-DD>` - the only one that uses the API (about 10 calls from a key's monthly budget): fetch one completed past day, confirm each response was archived intact and the stored record reproduces from it; never writes `history/`
  - The offline commands exit non-zero on any finding. There is deliberately no "verify everything" flag, so nothing spends API quota unless you ask for `--verify-enphase-archive`
- `--true-up <YYYY-MM-DD>` - Activate True-Up Mode using this utility True-Up Start Date. Covers the 12-month True-Up Window (Current Period: through yesterday; Past True-Up Period: through last day of 12-month window). Takes precedence over `--date`
- `--seed-credentials` - Seed `credentials.yaml` from the Enphase developer portal: opens Chrome to log in, then scrapes each application's `name`, `key`, `client_id`, and `client_secret`. New entries are appended with an empty `refresh_token` (fill it with `--update-refresh-tokens`); existing entries are resynced in place, preserving their refresh tokens. Filter with `--quota-prefix`
- `--update-refresh-tokens` - Run the OAuth wizard to re-authorize **every** credential in the credentials file, saving each refresh token back into it (one login, no per-app clicking)
- `--credentials-file <path>` - Credentials file to use instead of `credentials_file` from config.yaml (default `credentials.yaml`), e.g. `--credentials-file credentials-mobile.yaml`. Applies to every mode
- `--refresh-quota` - Out-of-band resync of each credential's monthly API-usage baseline from the developer portal stats page (opens Chrome to log in). Filter with `--quota-prefix`. The same baseline is also seeded by `--init`
- `--quota-prefix <prefix>` - Application/credential name prefix filter (default `enphase-monitor-`). Scopes which applications `--seed-credentials` pulls and which credentials `--init`/`--refresh-quota` sync
- `--no-cache` - Bypass cache and make live API calls. Propagates API errors (429/503/network) instead of serving stale cache. No effect with `--start-date`, which is always live
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
- For developer plan, complete OAuth setup with `--update-refresh-tokens` to get `refresh_token`

### "API request failed with status 401"
- Your refresh token may have expired or been revoked
- Re-run OAuth setup: `./enphase-monitor --update-refresh-tokens`
- Verify your API credentials are correct

### "API request failed with status 404"
- Verify your System IDs are correct (check Enlighten URLs for each system)
- Check that you have access to all systems in Enlighten
- Ensure the system IDs are strings (quoted in YAML)

### "rate limit exceeded (429)"
- Enphase allows 10 calls per minute (and 1000 per month) per API key; a 429 means a key was throttled
- With a credential pool the program puts that key in cooldown and retries the System on a spare key; you only see this error once every key has been tried
- Wait a minute and re-run, or add more credentials to the pool (`--seed-credentials`)
- Run with `--debug` to see the credential pool's monthly quota summary and each live/cached decision

### "API request failed with status 422"
- This usually means the requested date is in the future
- The program automatically caps dates to the current time
- Try querying a past date: `--date 2026-01-15`

### No Interval Data returned
- Some data may not be available for very recent time periods (try querying yesterday's data)
- Ensure your systems are actively reporting to Enlighten

### PG&E pull prints `WARNING:` lines
The pull checks each day before writing it (the records are still written):
- `daily import is negative` or `interval(s) have import < 0 and export > 0` — the gross and signed net registers were mixed. This should not happen with the current parser; if it does, keep the raw ZIP (`raw/pge/`) and re-run `--verify-pge-raw`.
- `N intervals, expected M for this local day` — 92 (spring-forward) and 100 (fall-back) are the real lengths of those days. PG&E's feed omits the repeated hour on fall-back days, so that day arrives with 96 and is slightly understated; any other count means missing data.
- `PG&E net … vs Enphase net …` — the day's net differs from Enphase's by more than 10% of the larger flow. Check which source is wrong (for example, Enphase's 2026-03-08 export meter reported zeros midday; see [Known data caveats](#known-data-caveats)).

### `--verify-*-raw` reports a finding
- `hash differs from manifest` / `listed in manifest but missing` — a file under `raw/` was altered or deleted; restore it from your backup. `not listed in manifest` — a file was added by hand.
- `history record differs from the one re-derived from raw` (PG&E), `history weather differs from the newest raw payload`, or a `record … vs raw …` mismatch (Enphase) — `history/` no longer matches what the current code derives from the archive. For PG&E, `--pge-reprocess <archive>` rebuilds the records; for weather, `--raw-apply-weather`. A mismatch you did not cause means a record was edited or produced by older code.
- `raw responses missing` (Enphase) — a day in `history/` has no archived body for some system/endpoint (for example a day fetched before archiving existed and no longer in `cache/`).

## Caching and API Budget

### API Budget

The Enphase Enlighten Cloud API v4 enforces a strict API Budget:
- **10 requests per minute** per API key
- **1000 requests per month** total (free developer plan)

### Credential Pool & Monthly Quota

Because each API key is capped at **10 requests/minute AND 1000 requests/month**, the app can hold a **pool** of credential sets (a `credentials:` list of more than one entry) and spread load across them:

- **Spread.** Each System is assigned the credential with the lowest monthly usage (least-used-first), so a run's calls are distributed across keys without a persisted rotation counter. The monthly counts in `cache/monthly-quota.json` are the authoritative state.
- **Failover.** When a key returns 429 (per-minute) or has spent its monthly budget, it is put in cooldown and the System retries on a spare; the run only errors when every key is exhausted.
- **Monthly quota tracking.** Each credential's calls-this-month are persisted in `cache/monthly-quota.json`. The baseline is seeded from the developer portal by `--init` (or resynced with `--refresh-quota`) and incremented on every live call, resetting on the month boundary. A key with no monthly budget left is skipped just like one in per-minute cooldown — so the pool rotates off a key approaching its 1000/month cap, not only on a 429.

**Setting up a pool** (one-time): run `--seed-credentials` to scrape every portal application's `name`/`key`/`client_id`/`client_secret` into `credentials.yaml`, then `--update-refresh-tokens` to authorize the newly-seeded entries. Adding more apps later repeats the same two steps.

### Caching Strategy

To respect these limits, the application combines disk caching with a live-first serving policy for current data:

- **Automatic Disk Caching**: All API responses are cached as JSON files in the `cache/` directory.
- **Cached for Past Periods, Live for Current Periods**:
  - **Past Period Day / Month / Year / True-Up** — served from Cache when an entry exists. The data is immutable; a live call would waste budget and return identical results.
  - **Current Period queries** (today's Day Mode query, month-to-date, year-to-date, Current Period True-Up) — a live API call is made, because the data changes throughout the day.
  - **`--no-cache`, Backfill Mode and `--verify-enphase-archive`** — always live; the cache is neither read nor trusted.
- **Errors are never masked by stale cache.** A 429, 503 or network error on a live call is propagated. A 429 reaches the aggregator, which puts that credential in cooldown and retries the System on a spare credential from the pool; only when every credential has been tried does the System report a rate-limit error. If every credential has spent its monthly budget, the run stops with that message.
- **Budget is the monthly count per credential** (`cache/monthly-quota.json`, see above). There is no per-minute counter: Enphase's 10/minute limit is handled reactively, through the 429 failover.
- **Cache is not the audit trail.** It can be cleared (`--clear-cache`, `--clear-cache-date`, `--clear-all-cache`) or overwritten. Every live response is additionally kept untouched under `raw/` (see [Raw source archive](#raw-source-archive-raw)), which no cache command touches.
- **Default Refresh Interval**: 1 hour (3600 seconds) — the default value for the `refresh_interval` config field (currently unused).

### Debug Mode

The `--debug` flag prints a startup status block plus a trace of every cache/live decision to stderr:

```bash
./enphase-monitor --debug
```

Sample output:

```
[DEBUG] --- startup ---
[DEBUG] current time : 2026-05-19 09:14:22 PDT
[DEBUG] <credential pool monthly quota summary>
[DEBUG] ---
[DEBUG] serving cache (Past Period, age 6h12m0s): https://api.../energy_lifetime?...
[DEBUG] live API call: https://api.../telemetry/production_meter?...
[DEBUG] live API call succeeded: https://api.../telemetry/production_meter?...
```

Use it when you want to understand *why* a query returned cached vs live data, or how much of each credential's monthly budget is spent. The API key never appears in the trace (URLs are redacted). Debug mode also suppresses the screen-clearing escape sequence so the trace stays visible on subsequent runs.

### Best Practices

1. **Leverage Caching**: Query past dates frequently - they use cached data (no API calls)
2. **Use `--debug`**: Run with `--debug` to see cache/live decisions and API Budget state

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
│   │   ├── client.go                        # Enlighten Cloud API client; archives each live response to raw/enphase; DayKWh for offline re-derivation
│   │   ├── types.go                         # API request/response types
│   │   ├── client_test.go                   # API client unit tests
│   │   ├── client_caching_test.go           # Characterization tests for makeCachedAPIRequest branches (no-cache mode, 429/503/network-error propagation)
│   │   ├── client_functional_test.go        # Functional tests with mock HTTP servers
│   │   ├── client_lifetime_test.go          # Lifetime Data tests (Month, Year, True-Up Mode queries)
│   │   └── testmain_test.go                 # TestMain: redirects cache I/O to temp dir for all api tests
│   ├── app/                                 # Application execution logic
│   │   ├── setup.go                         # App initialization & configuration
│   │   ├── setup_test.go                    # Setup tests
│   │   ├── runner.go                        # RunOnce execution mode
│   │   ├── runner_test.go                   # Runner tests
│   │   ├── trueup.go                        # True-Up Mode: single-batch Lifetime Data query and report conversion
│   │   ├── trueup_test.go                   # True-up logic tests
│   │   ├── backfill.go                      # Backfill Mode: live per-day fetch over a date range into history/
│   │   ├── backfill_test.go                 # Backfill range/skip/overwrite tests
│   │   ├── weather.go                       # Best-effort weather enrichment for Day-Mode reports
│   │   ├── weather_test.go                  # Weather enrichment tests
│   │   ├── rawweather.go                    # HistoryDates, RefetchWeatherRaw, ApplyRawWeather (+ .weather-provenance.json sidecar)
│   │   ├── rawweather_test.go               # Raw-weather, provenance, and offline Enphase-verification tests
│   │   ├── verifyenphaseraw.go              # VerifyEnphaseRaw: re-derive each history day from raw/enphase (--verify-enphase-raw)
│   │   └── verifyenphase.go                 # VerifyEnphaseArchive: one-day live archiving check (--verify-enphase-archive)
│   ├── browser/                             # Headed Chrome launcher (chromedp) for portal automation
│   │   ├── chrome.go                        # LaunchHeaded (disposable profile) + LaunchHeadedWithProfile (persistent profile)
│   │   └── chrome_test.go                   # Chrome launcher tests
│   ├── cache/                               # Disk-based response caching
│   │   ├── cache.go                         # Cache implementation (URL normalization, Past Period handling, endpoint/system tagging)
│   │   ├── cache_test.go                    # Cache state management tests (CacheDisabled, ResetState)
│   │   ├── cache_functions_test.go          # Core caching tests (URL normalization, key generation, save/load, ClearCacheForDate)
│   │   ├── cli.go                           # Cache inspection utilities
│   │   └── cli_test.go                      # CLI utilities tests
│   ├── cli/                                 # Command-line interface
│   │   ├── flags.go                         # CLI flag parsing, grouped --help, MaintenanceCommands/ValidateMaintenance
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
│   ├── credentials/                         # Credential pool: spread + 429 failover + monthly quota
│   │   ├── pool.go                          # Round-robin assignment, cooldown, failover
│   │   ├── pool_test.go                     # Pool selection/failover tests
│   │   ├── quota.go                         # Per-credential monthly API budget (monthly-quota.json)
│   │   ├── quota_test.go                    # Quota counting, month-rollover, and budget tests
│   │   └── quota_portal_test.go             # Portal-seeded monthly baseline (ApplyPortalMonthlyUsage) tests
│   ├── display/                             # Terminal output formatting
│   │   ├── display.go                       # Display with io.Writer injection
│   │   └── display_test.go                  # Display output tests
│   ├── enphase/                             # Developer-portal scraping (no management API)
│   │   ├── login.go                         # Headed-Chrome portal login + session cookie capture
│   │   ├── portal.go                        # Scrape app name/key/client_id/client_secret
│   │   ├── seed.go                          # --seed-credentials: scrape + merge into credentials.yaml
│   │   ├── stats.go                         # Monthly hit totals from the portal stats page
│   │   ├── stats_browser.go                 # chromedp driver for the stats page UI
│   │   ├── login_test.go                    # Login/cookie-capture tests
│   │   ├── portal_test.go                   # Portal HTML/JSON scrape tests
│   │   └── stats_test.go                    # Stats parsing/date-range tests
│   ├── geocode/                             # ZIP/postal code → coordinates (Zippopotam.us)
│   │   ├── geocode.go                       # ZIP lookup for weather geolocation
│   │   └── geocode_test.go                  # Geocode tests
│   ├── history/                             # Per-day energy+weather and utility-meter JSON records (history/)
│   │   ├── history.go                       # Dataset type (Enphase/PGE sentinels), DayRecord schema, FromMetrics, WriteRecord, WriteIndex
│   │   └── history_test.go                  # History mapping, write, index, and Dataset tests
│   ├── location/                            # Resolve & cache systems' coordinates (--init)
│   │   ├── location.go                      # Location resolver with disk cache
│   │   └── location_test.go                 # Location resolver tests
│   ├── oauth/                               # OAuth 2.0 authentication (Enphase developer-plan)
│   │   ├── oauth.go                         # Token management & refresh
│   │   ├── browser.go                       # Browser-driven OAuth authorization (auto-approves consent)
│   │   ├── oauth_test.go                    # Basic unit tests
│   │   ├── browser_test.go                  # Browser-OAuth flow tests
│   │   ├── oauth_functional_test.go         # Integration tests with mock servers
│   │   └── oauth_edge_cases_test.go         # Edge case tests
│   ├── pge/                                 # PG&E Green Button integration: browser download, ESPI decoding, sanity checks, raw-archive verification
│   │   ├── doc.go                           # Package documentation: the PG&E data flow and the commands that reach it
│   │   ├── browserpull.go                   # BrowserPull: Green Button download via headed Chrome session; archives the ZIP; ReprocessRaw (--pge-reprocess)
│   │   ├── espi.go                          # ParseReadings (register selection, Wh scale), AggregateReadingsByDay (local-clock decoding, DST) → DayUsage intervals
│   │   ├── sanity.go                        # CheckDays: write-time warnings (negative import, interval count, net vs Enphase)
│   │   ├── verify.go                        # VerifyPGERaw: raw/pge hash check + re-derive and diff against history (--verify-pge-raw)
│   │   ├── history.go                       # WriteHistory: aggregate DayUsage → pge-YYYY-MM-DD.json records
│   │   ├── download.go                      # downloadGreenButton: drive the export form (autoform) and capture the new download by watching the download directory
│   │   ├── login.go                         # openLoggedInSession: headed Chrome on a persistent profile; blocks until the user has signed in to PG&E
│   │   ├── autoform.go                      # formDriver: auto-drive the Green Button export form inside PG&E's Visualforce iframe
│   │   ├── autoform_test.go                 # Frame lookup and formDriver debug-buffer tests
│   │   ├── download_test.go                 # Download-directory snapshot and completed-file detection tests
│   │   ├── browserpull_test.go              # One-year export-range clamp and raw-archive naming tests
│   │   ├── espi_test.go                     # ESPI parser/aggregation tests on real redacted feeds (registers, clock, DST days), CheckDays
│   │   ├── verify_test.go                   # VerifyPGERaw round-trip: clean, edited record, tampered raw file; archive ordering by fetch time
│   │   └── testdata/                        # Real PG&E exports with identifiers removed (2025-11-08, 2026-03-08, 2025-11-02)
│   ├── parser/                              # JSON telemetry parsing
│   │   ├── parser.go                        # Response parsing utilities
│   │   ├── parser_test.go                   # Parser tests
│   │   └── parser_bench_test.go             # Benchmark tests
│   ├── rawstore/                            # Raw source archive: raw/<source>/ files + raw/manifest.jsonl (SHA-256)
│   │   ├── rawstore.go                      # Save, Record, RedactURL; disabled unless rawstore.Dir is set
│   │   ├── importcache.go                   # ImportEnphaseCache: copy cache/ bodies into raw/enphase (--raw-import-cache)
│   │   ├── verify.go                        # VerifySource, manifest readers, VerifyEntry, PrintReport
│   │   └── rawstore_test.go                 # Save/manifest, import idempotence, verification tests
│   ├── timezone/                            # Timezone handling
│   │   ├── timezone.go                      # Timezone utilities
│   │   └── timezone_test.go                 # Timezone tests
│   ├── types/                               # Shared type definitions
│   │   └── types.go                         # SystemConfig, APIConfig (breaks circular deps)
│   ├── urlbuilder/                          # API URL construction
│   │   ├── urlbuilder.go                    # URL building helpers
│   │   └── urlbuilder_test.go               # URL builder tests
│   └── weather/                             # Open-Meteo daily/current weather client
│       ├── weather.go                       # DailyWeather/CurrentWeather fetch + WMO mapping; archives live payloads to raw/weather; ParseDailyPayload; BypassCache
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
├── test-data/                               # Recorded fixtures for the geocode and location tests
│   ├── zippopotam_api_*.json                # Zippopotam.us responses
│   └── enphase_systems.json                 # Recorded systems response
├── config.yaml.example                      # Example non-secret configuration (systems, colors, etc.)
├── config.yaml                              # Your actual configuration (create from example)
├── credentials.yaml.example                 # Example credentials (credentials: list)
├── credentials.yaml                         # Your actual API secrets (create from example; gitignored)
├── cache/                                   # Cached API responses (created at runtime; clearable)
├── raw/                                     # Untouched source responses + manifest.jsonl (created at runtime; gitignored — holds account data)
├── history/                                 # Per-day energy+weather and utility-meter JSON records (created by range pull and PG&E pull), plus dotfile sidecars (.<prefix>-index.json, .weather-provenance.json)
├── tmp/                                     # Scratch downloads (PG&E ZIP/XML before extraction); gitignored
├── weather_codes.json                       # WMO weather-code legend (written by --init)
├── go.mod                                   # Go module definition
├── go.sum                                   # Go module checksums
├── scripts/                                 # Utility scripts
│   ├── migrate-history-layout.sh            # One-time migration: renames plain <date>.json records to enphase-<date>.json
│   └── history.py                           # Git history inspection helper
├── Makefile                                 # Build automation
├── CONTEXT.md                               # Domain glossary (project terminology, "avoid" terms)
├── README.md                                # This file
└── QUICKSTART.md                            # Quick start guide
```

## Testing

The project includes a comprehensive test suite with **56.7% code coverage** across all packages. Tests run against mock servers, recorded fixtures and real redacted PG&E exports, so the suite never spends API Budget.

### Test Coverage by Package

| Package | Coverage | Status |
|---------|----------|--------|
| urlbuilder | 100.0% | ✅ |
| display | 99.3% | ✅ |
| cli | 96.5% | ✅ |
| parser | 94.8% | ✅ |
| timezone | 92.7% | ✅ |
| aggregator | 89.2% | ✅ |
| weather | 88.5% | ✅ |
| credentials | 85.0% | ✅ |
| config | 84.5% | ✅ |
| history | 82.6% | ✅ |
| api | 81.9% | ✅ |
| geocode | 81.5% | ✅ |
| location | 81.0% | ✅ |
| cache | 74.8% | ✅ |
| constants | 72.7% | ✅ |
| rawstore | 70.4% | ✅ |
| oauth | 61.7% | ✅ |
| app | 52.5% | ⚠️ |
| pge | 37.4% | ⚠️ |
| browser | 16.3% | ⚠️ |
| enphase | 14.3% | ⚠️ |

**Total: 56.7% coverage** (the low-coverage packages are headed-Chrome/portal-scraping glue — `app` orchestration of the live paths is exercised via the api-package integration tests, while `browser`, `enphase`, and the `pge` browser-pull path drive a real Chrome session and are verified manually. The `pge` ESPI decoding, aggregation, sanity checks and raw-archive verification are unit-tested against real redacted feeds.)

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
- **Minimal Dependencies**: Only 3 direct external libraries (gopkg.in/yaml.v3 for config; chromedp + cdproto for portal automation); all HTTP and I/O uses the standard library

## Code Quality

This project follows Go best practices and coding standards:

- **Test Coverage**: 56.7% overall, 100% for urlbuilder, 99% for display, 95%+ for cli and parser, 90%+ for timezone, 89% for aggregator (the headed-Chrome `browser`/`enphase` portal-scraping packages and the `pge` browser-pull path are verified manually and pull the average down)
- **Test Suite**: 47 test files across 21 tested packages with comprehensive unit, functional, and edge case tests
- **Go Modules**: Proper dependency management with go.mod/go.sum
- **Error Handling**: Comprehensive error wrapping with context
- **Documentation**: Extensive inline comments and dedicated guides
- **Type Safety**: Strict type checking with no unsafe operations
- **Linting**: Passes golangci-lint with recommended settings
- **Performance**: Benchmarks included for hot paths

**Code Metrics:**
- Total Lines: ~12,100 (excluding tests)
- Test Lines: ~11,200 (comprehensive test suite)
- Packages: 22 internal packages (21 with tests; `types` is a pure type-definition package)
- Test Files: 47 (unit, functional, edge case, and benchmark tests)
- External Dependencies: 3 (gopkg.in/yaml.v3 and github.com/chromedp/chromedp + cdproto for portal automation)

## License

This is a personal utility project. Use and modify as needed for your own Enphase monitoring needs.
