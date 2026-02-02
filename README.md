# Enphase Monitor

A Go application for monitoring and aggregating data from multiple Enphase solar systems via the Enphase Enlighten Cloud API v4.

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
- [Caching and Rate Limits](#caching-and-rate-limits)
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

- **Multi-System Monitoring**: Query and combine metrics from multiple independent Enphase systems
- **Comprehensive Metrics**: Track production, consumption, battery usage, grid import/export, and net energy flow
- **Flexible Querying**: Query historical dates or monitor real-time with auto-refresh
- **Clean Display**: Formatted terminal output with customizable colors
- **API Caching**: Automatic caching of API responses to reduce API calls and enable offline validation
- **Color Customization**: Customize terminal colors using hex codes or ANSI escape codes
- **Cloud API v4**: Uses Enphase Enlighten Cloud API v4 for reliable data access
- **OAuth Support**: Full OAuth 2.0 support for developer plan authentication
- **Test Mode**: Validation mode for testing against expected values without making API calls

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

1. **Copy the example configuration file:**
   ```bash
   cp config.yaml.example config.yaml
   ```

2. **Edit `config.yaml` with your credentials:**

   ```yaml
   api:
     key: "YOUR_API_KEY"  # API Key from Enphase Developer Portal
     client_id: "YOUR_CLIENT_ID"  # OAuth Client ID
     client_secret: "YOUR_CLIENT_SECRET"  # OAuth Client Secret
     authorization_url: "https://api.enphaseenergy.com/oauth/token"
     redirect_uri: "http://localhost:8080/callback"
     refresh_token: "YOUR_REFRESH_TOKEN"  # From OAuth setup (see below)
   
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
     net_import: "#f63cb1"         # Net Energy Flow (Import)
     net_export: "#06b6de"         # Net Energy Flow (Export)
     headers: "#f37320"            # Report Headers
     charge: "#7acf38"             # Battery Charge
     total_consumed: "#f37320"     # Total Consumed
     secondary_text: "#808080"     # Secondary Text
     primary_text: "#ffffff"       # Primary Text
     error: "#ff0000"              # Error Text
   ```

> **💡 Implementation Details**: See [config.go](config.go) for color conversion logic (hex → ANSI) and [display.go](display.go) for color usage in terminal output.

### Configuration Sections

#### API Credentials

You will need these from the [Enphase Developer Portal](https://developer-v4.enphase.com/):
- `api.key`: Your API key
- `api.client_id`: OAuth client ID
- `api.client_secret`: OAuth client secret
- `api.refresh_token`: Obtained from OAuth setup (see [OAuth Setup](#oauth-setup) below)

#### System Configuration

Each system requires:
- `name`: A friendly name for display
- `id`: Your Enphase system ID (see [Finding Your System IDs](#finding-your-system-ids))

#### Optional Settings

- `refresh_interval`: How often to query the API in continuous mode (default: 3600 seconds = 1 hour)
  - **⚠️ Important**: Only applies when running in continuous mode (without `--once` flag)
  - **Recommended**: Use 3600 seconds (1 hour) to respect API rate limits
  - **Rate Limit Consideration**: The API allows 10 requests per minute. With multiple systems, a low `refresh_interval` (e.g., 5 seconds) can quickly exceed this limit. For 2 systems, you would make 24 requests per minute (2 systems × 12 queries/minute), which exceeds the 10/minute limit.
  - **Best Practice**: Use 3600 seconds (1 hour) or higher to stay well within rate limits
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
./enphase-monitor --setup-oauth
```
Run the interactive setup wizard that will guid you through:
1. Generating an authorization URL
2. Authorizing the application in your browser
3. Exchanging the authorization code for tokens
4. Adding the refresh token to your config

### Detailed Guide

For a comprehensive explanation of OAuth 2.0, what each component does, and how authentication works, see:

**[OAUTH_SETUP.md](docs/OAUTH_SETUP.md)** - Complete OAuth guide with:
- Explanation of OAuth 2.0 concepts
- What the API server expects for authentication
- How authorization works
- Step-by-step setup instructions
- Troubleshooting common issues

### After OAuth Setup

Once you have your `refresh_token`, add it to your `config.yaml`:

```yaml
api:
  refresh_token: YOUR_REFRESH_TOKEN  # From OAuth setup
```

The application will automatically use this token to get new access tokens when needed.

## API Configuration

This application uses the **Enphase Enlighten Cloud API v4** exclusively to fetch energy data.

### What the Cloud API Provides

- **Direct Daily Values**: The API returns pre-calculated daily totals - no need for state files or cumulative meter tracking
- **Historical Data**: Query any past date (not limited to today)
- **Real-time Updates**: Data is typically updated every 15 minutes
- **Reliable Access**: Works from anywhere with internet (no local network required)
- **Standardized Format**: Consistent JSON responses across all system types

### Rate Limits

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

### Run Once (Single Query)

Query today's data and exit:
```bash
./enphase-monitor --once
```

Query a specific historical date:
```bash
./enphase-monitor --once --date 2026-01-15
```

> **Note:** When querying a past date, the program automatically runs once (even without `--once` flag) since historical data doesn't change over time. You'll see a message like:
> ```
> Note: Running once for historical date 2026-01-15 (data won't change)
> ```

### Continuous Monitoring

Monitor with auto-refresh (uses `refresh_interval` from config):
```bash
./enphase-monitor
```

The application will query all systems at the configured `refresh_interval` (default: 3600 seconds = 1 hour) and display updated metrics.

Press `Ctrl+C` to stop.

### Command-Line Options

- `--config <path>` - Path to configuration file (default: `config.yaml`)
- `--once` - Run once and exit instead of continuous monitoring
- `--date <YYYY-MM-DD>` - Query specific date instead of today (e.g., `2026-01-15`)
- `--setup-oauth` - Run OAuth setup wizard (one-time for developer plan)
- `--test` - Test mode: use cache only, no live API calls, validate against expected values
- `--no-cache` - Bypass cache and make live API calls (falls back to cache on 429 rate limit)
- `--clear-cache` - Clear cached API responses for today's date only
- `--clear-all-cache` - Clear all cached API responses (all dates)
- `--list-cache` - List all cached API responses
- `--inspect-cache <hash|date>` - Inspect cached responses by hash or date (YYYY-MM-DD format)

### Examples

```bash
# Use custom config file
./enphase-monitor --config /path/to/my-config.yaml

# Query last week's data
./enphase-monitor --once --date 2026-01-12

# Continuous monitoring with default settings
./enphase-monitor
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

 COMBINED ENERGY REPORT (kWh)
---------------------------------------------------------
  Produced:                   33.4 kWh
  Consumed:                   48.6 kWh
  Net Energy Flow:            19.1 kWh (import)

 INDIVIDUAL SYSTEMS REPORT
---------------------------------------------------------

  [1] Right Subpanel (5525881)
      Imported from the Grid:         23.1 kWh
      Exported to the Grid:            3.8 kWh
      Captured from the Sun:          14.6 kWh
      Net Energy Flow:                19.3 kWh (import)
      Charged to Battery:              8.5 kWh
      Discharged from Battery:         6.8 kWh
      Battery Charge Percentage:           63%
      Total Consumed:                 32.1 kWh

  [2] Left Subpanel (5392556)
      Imported from the Grid:          7.5 kWh
      Exported to the Grid:            7.6 kWh
      Captured from the Sun:          18.9 kWh
      Net Energy Flow:                 0.2 kWh (export)
      Charged to Battery:              8.1 kWh
      Discharged from Battery:         5.4 kWh
      Battery Charge Percentage:           74%
      Total Consumed:                 16.4 kWh

=========================================================
```

Note: Colors are customizable via `config.yaml` (see Color Customization section below).

## Metrics Explained

### Combined Energy Report
- **Produced**: Total solar generation from all systems (kWh)
- **Consumed**: Total household consumption from all systems (kWh)
- **Net Energy Flow**: Net energy imported from or exported to the grid
  - Positive value with "(import)" suffix: More energy imported than exported
  - Negative value with "(export)" suffix: More energy exported than imported
  - Calculation: Grid Imported - Grid Exported

### Individual System Metrics
- **Imported from the Grid**: Energy purchased from utility for this system (kWh)
- **Exported to the Grid**: Energy sold back to utility from this system (kWh)
- **Captured from the Sun**: Solar generation for this system (kWh)
- **Net Energy Flow**: Net import/export for this system (kWh) with (import) or (export) suffix
- **Charged to Battery**: Energy stored in batteries for this system (kWh)
- **Discharged from Battery**: Energy used from batteries for this system (kWh)
- **Battery Charge Percentage**: Current state of charge (SOC) of the battery system, displayed as a percentage (0-100%). This metric is shown per-system only and is not aggregated across multiple systems.
- **Total Consumed**: Total consumption for this system (kWh)

## Color Customization

You can customize the colors used in the terminal output by adding a `colors` section to your `config.yaml`:

```yaml
colors:
  production: "#f0b57c"         # Solar Production
  discharge: "#7acf38"          # Battery Discharge
  import: "#f63cb1"             # Grid Import
  export: "#06b6de"             # Grid Export
  net_import: "#f63cb1"         # Net Energy Flow (Import)
  net_export: "#06b6de"         # Net Energy Flow (Export)
  headers: "#f37320"            # Report Headers
  charge: "#7acf38"             # Battery Charge
  total_consumed: "#f37320"     # Total Consumed
  secondary_text: "#808080"     # Secondary Text
  primary_text: "#ffffff"       # Primary Text
  error: "#ff0000"              # Error Text
```

**Color Format Options:**
- **Hex codes** (e.g., `#FF5733`): Automatically converted to ANSI color codes
- **ANSI escape codes** (e.g., `\033[38;5;208m`): Used directly as-is

**Note:** `Reset` and `Bold` are constants defined in `constants.go` and cannot be customized.

## Troubleshooting

### "API configuration required"
- Make sure you have copied `config.yaml.example` to `config.yaml`
- Verify you have filled in `api.key`, `api.client_id`, and `api.client_secret`
- For developer plan, complete OAuth setup with `--setup-oauth` to get `refresh_token`

### "API request failed with status 401"
- Your refresh token may have expired or been revoked
- Re-run OAuth setup: `./enphase-monitor --setup-oauth`
- Verify your API credentials are correct

### "API request failed with status 404"
- Verify your System IDs are correct (check Enlighten URLs for each system)
- Check that you have access to all systems in Enlighten
- Ensure the system IDs are strings (quoted in YAML)

### "rate limit exceeded (429)"
- The API has a rate limit of 10 calls per minute
- The program will display how many seconds to wait before retrying
- Consider increasing `refresh_interval` in your config
- Use `--test` mode to validate against cached data without making API calls

### "API request failed with status 422"
- This usually means the requested date is in the future
- The program automatically caps dates to the current time
- Try querying a past date: `--date 2026-01-15`

### No telemetry data returned
- Some data may not be available for very recent time periods (try querying yesterday's data)
- Ensure your systems are actively reporting to Enlighten
- Check cache with `--list-cache` to see what data is available

## Caching and Rate Limits

### Rate Limits

The Enphase Enlighten Cloud API v4 enforces strict rate limits:
- **10 requests per minute** per API key
- **1000 requests per month** total (free developer plan)

**⚠️ Refresh Interval Recommendation**: 

The `refresh_interval` setting controls how often the application queries the API in continuous mode. To respect rate limits:

- **Recommended**: `refresh_interval: 3600` (1 hour)
- **Why**: Each system requires multiple API calls. With 2 systems, you might make 8-10 requests per query cycle. At 3600 seconds, that is ~20 requests per hour, well within limits.
- **Not Recommended**: Values below 60 seconds (e.g., `refresh_interval: 5`) can quickly exceed the 10 requests/minute limit, especially with multiple systems.
- **Calculation**: If you have N systems and each requires M API calls, you make N×M requests per cycle. At `refresh_interval: 5`, that is N×M×12 requests per minute, which can easily exceed 10/minute.

### Caching Strategy

To respect these limits, the application implements intelligent caching:

- **Automatic Disk Caching**: All API responses are cached in `cache/` directory
- **Cache-First for Past Dates**: When querying historical dates, cached data is used if available (no API call)
- **Default Refresh Interval**: 1 hour (3600 seconds) - queries each system once per hour
- **429 Error Handling**: If rate limited, the program displays wait time and exits gracefully

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
# View all cached responses
./enphase-monitor --list-cache

# Inspect a specific cached response (by hash or date)
./enphase-monitor --inspect-cache 2026-01-15

# Clear today's cache only (preserves historical data)
./enphase-monitor --clear-cache

# Clear all cached data
./enphase-monitor --clear-all-cache

# Disable caching (always make live API calls)
./enphase-monitor --no-cache

# Test mode (use cache only, no live API calls)
./enphase-monitor --test --date 2026-01-15
```

### Best Practices

1. **Use Default Refresh Interval**: Use 1 hour to stay well within rate limits
2. **Leverage Caching**: Query past dates frequently - they use cached data (no API calls)
3. **Test Mode**: Use `--test` mode for validation against cached data without hitting API
4. **Monitor Cache**: Use `--list-cache` to see what data is available before querying

## Documentation

This project includes comprehensive documentation for different learning paths:

### For Getting Started
- **[QUICKSTART.md](QUICKSTART.md)** - Get up and running in 5 minutes
- **[OAUTH_SETUP.md](docs/OAUTH_SETUP.md)** - Complete OAuth 2.0 setup guide with detailed explanations

### For Understanding the Codebase
- **[ARCHITECTURE.md](docs/ARCHITECTURE.md)** - System architecture, execution flow, and design patterns
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
├── main.go                                # Application entry point (orchestration only)
├── internal/                              # Internal packages
│   ├── aggregator/                        # Multi-system data aggregation
│   │   ├── types.go                       # Metric data structures
│   │   ├── aggregator.go                  # Aggregation logic with dependency injection
│   │   ├── aggregator_test.go             # Aggregator tests with mock clients
│   │   └── aggregator_bench_test.go       # Benchmark tests
│   ├── api/                               # HTTP client for Cloud API v4
│   │   ├── client.go                      # Enlighten Cloud API client
│   │   ├── types.go                       # API request/response types
│   │   ├── interface.go                   # CloudClient interface for testability
│   │   ├── client_test.go                 # API client unit tests
│   │   └── client_functional_test.go      # Functional tests with mock HTTP servers
│   ├── app/                               # Application execution logic
│   │   ├── setup.go                       # App initialization & configuration
│   │   ├── setup_test.go                  # Setup tests
│   │   ├── runner.go                      # Execution modes (once/continuous)
│   │   └── runner_test.go                 # Runner tests
│   ├── cache/                             # Disk-based response caching
│   │   ├── cache.go                       # Cache implementation
│   │   ├── cache_test.go                  # Cache state management tests
│   │   ├── cache_functions_test.go        # Cache functionality tests
│   │   ├── cli.go                         # Cache inspection utilities
│   │   └── cli_test.go                    # CLI utilities tests
│   ├── cli/                               # Command-line interface
│   │   ├── flags.go                       # CLI flag parsing
│   │   ├── flags_test.go                  # Flag parsing tests
│   │   ├── cache_commands.go              # Cache management commands
│   │   └── cache_commands_test.go         # Cache commands tests
│   ├── config/                            # Configuration types
│   │   ├── config.go                      # YAML loading & validation (uses type aliases)
│   │   └── config_test.go                 # Configuration tests
│   ├── constants/                         # Centralized constants
│   │   ├── constants.go                   # Application-wide constants
│   │   └── constants_test.go              # Constants tests
│   ├── display/                           # Terminal output formatting
│   │   ├── display.go                     # Display with io.Writer injection
│   │   └── display_test.go                # Display output tests
│   ├── oauth/                             # OAuth 2.0 authentication
│   │   ├── oauth.go                       # Token management & refresh
│   │   ├── setup.go                       # Interactive OAuth wizard
│   │   ├── oauth_test.go                  # Basic unit tests
│   │   ├── oauth_functional_test.go       # Integration tests with mock servers
│   │   └── oauth_edge_cases_test.go       # Edge case tests
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
│   └── validation/                        # Test mode validation
│       ├── validation.go                  # Metrics validation logic (uses io.Writer for testability)
│       ├── validation_test.go             # Unit tests (tolerance calculations, edge cases)
│       └── validation_integration_test.go # Integration tests (real expected values)
├── docs/                                  # Project documentation
│   ├── ARCHITECTURE.md                    # Architecture documentation
│   ├── GO_BEST_PRACTICES.md               # Go best practices guide
│   ├── GO_CONCEPTS.md                     # Go concepts reference (channels, signals, and more)
│   ├── OAUTH_SETUP.md                     # OAuth setup documentation (detailed)
│   ├── TESTING.md                         # Testing patterns and guidelines
│   ├── go-documentation-review.md         # Go documentation style review
│   ├── go-linting-review.md               # Linting configuration review
│   └── go-style-core-review.md            # Go style guide compliance review
├── test-data/                             # Test validation data
│   └── expected_values_*.json             # Expected values for validation
├── config.yaml.example                    # Example configuration with all options
├── config.yaml                            # Your actual configuration (create from example)
├── cache/                                 # Cached API responses (created at runtime)
├── go.mod                                 # Go module definition
├── go.sum                                 # Go module checksums
├── scripts/                               # Utility scripts
│   ├── generate-pdfs.sh                   # Script to generate PDFs from markdown files
│   └── run-tests.sh                       # Test runner script
├── Makefile                               # Build automation
├── README.md                              # This file
├── QUICKSTART.md                          # Quick start guide
└── pdfs/                                  # Generated PDF documentation (created by generate-pdfs.sh)
```

## Testing

The project includes a comprehensive test suite with **70.4% code coverage** across all packages. The test suite validates both functionality and metrics against expected values, enabling rapid iteration without hitting API rate limits.

### Test Coverage by Package

| Package | Coverage | Status |
|---------|----------|--------|
| constants | 100.0% | ✅ |
| display | 100.0% | ✅ |
| urlbuilder | 100.0% | ✅ |
| validation | 96.6% | ✅ |
| timezone | 93.3% | ✅ |
| config | 82.4% | ✅ |
| parser | 80.8% | ✅ |
| aggregator | 80.0% | ✅ |
| app | 76.8% | ✅ |
| api | 74.4% | ✅ |
| cache | 66.9% | ✅ |
| cli | 47.6% | ✅ |
| oauth | 46.1% | ✅ |

**Total: 70.4% coverage** (exceeds typical Go project standards of 50-60%)

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

CI (GitHub Actions) runs on push and pull requests: it builds the project, runs `go vet` and tests, and runs `golangci-lint`. See [.github/workflows/ci.yml](.github/workflows/ci.yml).

### Test Mode (Cache Only)

Run in test mode using only cached responses (no live API calls):

```bash
./enphase-monitor --once --test --date 2026-01-20
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
  ./enphase-monitor --once

Then retry with --test.
```

**Missing expected values file:**
```
Validation failed: no expected values file found for 2026-01-01.

To run validation, create the file:
  test-data/expected_values_2026-01-01.json

Example format:
  { "date": "2026-01-01", "systems": [...] }

To skip validation and just use cache-only mode, omit the --test flag:
  ./enphase-monitor --once --date 2026-01-01
```

### Setting Up Test Data

**IMPORTANT:** The cache must contain data for the specific test date. Old cache files from different dates will cause incorrect results.

1. **Run all tests** (recommended):
   ```bash
   ./scripts/run-tests.sh
   ```
   This script will:
   - Check which test dates have cached responses
   - Generate missing cache by making live API calls (waiting 60 seconds between calls to respect rate limits)
   - Run validation tests for all dates (2026-01-14 through 2026-01-20)
   - Display a summary of all test results

2. **Or manually generate cache for a specific date**:
   ```bash
   ./enphase-monitor --once --date 2026-01-20
   ```
   This will make live API calls and cache the responses in `cache/`

3. **Update `expected_values_YYYY-MM-DD.json`** with the correct values from the Enphase app

4. **Now you can iterate rapidly using test mode** (uses cache only, no API calls):
   ```bash
   ./enphase-monitor --once --test --date 2026-01-20
   ```

**Note:** Test mode uses the cache from `cache/`. The `scripts/run-tests.sh` script ensures all test dates have cached responses available.

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
        "net_imported": 19.6,
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
- **API Response Caching**: Reduces redundant API calls and respects rate limits
- **Efficient JSON Parsing**: Optimized for 96 intervals/day (15-min intervals)
- **Multi-System Aggregation**: Scales linearly with number of systems
- **Zero External Dependencies**: Standard library only for performance and security

## Code Quality

This project follows Go best practices and coding standards:

- **Test Coverage**: 70.4% overall, 100% for critical packages (constants, display, urlbuilder)
- **Test Suite**: 24 test files across 13 packages with comprehensive unit, integration, and edge case tests
- **Go Modules**: Proper dependency management with go.mod/go.sum
- **Error Handling**: Comprehensive error wrapping with context
- **Documentation**: Extensive inline comments and dedicated guides
- **Type Safety**: Strict type checking with no unsafe operations
- **Linting**: Passes golangci-lint with recommended settings
- **Performance**: Benchmarks included for hot paths

**Code Metrics:**
- Total Lines: ~4,000 (excluding tests)
- Test Lines: ~3,500+ (comprehensive test suite)
- Packages: 13 internal packages
- Test Files: 24 (unit, integration, functional, edge case, and benchmark tests)
- External Dependencies: 1 (gopkg.in/yaml.v3)

## License

This is a personal utility project. Use and modify as needed for your own Enphase monitoring needs.
