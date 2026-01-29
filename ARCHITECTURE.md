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
5. [Key Go Patterns Used](#key-go-patterns-used)
6. [File Descriptions](#file-descriptions)
7. [Next Steps for Learning](#next-steps-for-learning)

---

## Project Overview

The **enphase-monitor** is a CLI application that monitors energy metrics from one or more Enphase solar systems via the Enphase Enlighten Cloud API v4.

### Core Capabilities

- **Multi-System Monitoring**: Query and aggregate data from multiple independent Enphase systems
- **Cloud API Integration**: Uses Enphase Enlighten Cloud API v4 exclusively (no local network access required)
- **Intelligent Caching**: Disk-based response caching to respect API rate limits (10 calls/minute)
- **Historical Data**: Query any past date with `--date` flag
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
├── main.go              # Entry point, CLI flags, execution control
├── config.go            # Configuration loading and validation
├── aggregator.go        # Multi-system data aggregation orchestrator
│
├── cloud_client.go      # Enlighten Cloud API v4 client
├── oauth.go             # OAuth 2.0 token management
├── setup_oauth.go       # Interactive OAuth setup wizard
│
├── api_cache.go         # Disk-based response caching & cache utilities
├── cache_cli.go         # Cache management CLI commands
├── response_parser.go   # Telemetry JSON parsing utilities
│
├── display.go           # Terminal formatting and color output
├── constants.go         # ANSI formatting constants
├── timezone.go          # Timezone utilities
├── url_builder.go       # Telemetry URL construction
│
├── config.yaml          # User configuration (not in git)
├── config.yaml.example  # Configuration template
└── test-data/           # Cache files and validation data
```

---

## Execution Flow

### Primary Path: Report Generation

```
┌───────────────────────────────────────────────────────────────────┐
│  1. ENTRY POINT (main.go)                                         │
│     └─► Parse CLI flags (flag package)                            │
│         └─► Dispatch to appropriate handler                       │
└───────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌───────────────────────────────────────────────────────────────────┐
│  2. CONFIGURATION (config.go)                                     │
│     └─► LoadConfig() reads YAML file                              │
│         └─► Validates system configurations                       │
│             └─► Converts hex colors to ANSI codes                 │
└───────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌───────────────────────────────────────────────────────────────────┐
│  3. AGGREGATION (aggregator.go)                                   │
│     └─► GetAggregatedMetrics() loops through systems              │
│         └─► Creates EnlightenCloudClient for each system          │
│             └─► Fetches production, consumption, grid, battery    │
└───────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌───────────────────────────────────────────────────────────────────┐
│  4. API CALLS (cloud_client.go)                                   │
│     └─► Each call goes through caching layer (api_cache.go)       │
│         ├─► Check cache first (for rate limit protection)         │
│         ├─► Make HTTP request if cache miss                       │
│         └─► Save response to cache                                │
└───────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌───────────────────────────────────────────────────────────────────┐
│  5. RESPONSE PARSING (response_parser.go)                         │
│     └─► Parse JSON telemetry data                                 │
│         └─► Sum interval values for daily totals                  │
│             └─► Convert Wh to kWh                                 │
└───────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌───────────────────────────────────────────────────────────────────┐
│  6. DISPLAY (display.go)                                          │
│     └─► ShowMetrics() formats output                              │
│         ├─► printHeader() - Query range and timestamp             │
│         ├─► printTodayEnergy() - Combined totals                  │
│         └─► printIndividualSystems() - Per-system breakdown       │
└───────────────────────────────────────────────────────────────────┘
```

---

## Data Flow Diagram

```
                    ┌──────────────────┐
                    │   config.yaml    │
                    └────────┬─────────┘
                             │
                             ▼
┌───────────────────────────────────────────────────────────────────┐
│                        LoadConfig()                               │
│   Parses YAML, validates systems, converts colors                 │
└────────────────────────────┬──────────────────────────────────────┘
                             │
                             ▼
┌───────────────────────────────────────────────────────────────────┐
│                     Cloud Systems                                 │
│              (system.ID required)                                 │
└────────────────────────────┬──────────────────────────────────────┘
                             │
                             ▼
┌───────────────────────────────────────────────────────────────────┐
│                  EnlightenCloudClient                             │
│  - OAuth authentication                                           │
│  - Telemetry endpoints                                            │
│  - 15-min intervals                                               │
└────────────────────────────┬──────────────────────────────────────┘
                             │
                             ▼
┌───────────────────────────────────────────────────────────────────┐
│                     API Cache Layer                               │
│  - Rate limit tracking                                            │
│  - Disk-based storage                                             │
└────────────────────────────┬──────────────────────────────────────┘
                             │
                             ▼
┌───────────────────────────────────────────────────────────────────┐
│                    AggregatedMetrics                              │
│   - Sums production, consumption across systems                   │
│   - Aggregates battery charge/discharge across systems            │
│   - Tracks cache usage flag                                       │
└──────────────────────────┬────────────────────────────────────────┘
                           │
                           ▼
┌───────────────────────────────────────────────────────────────────┐
│                        Display                                    │
│   - ANSI color formatting                                         │
│   - Structured report output                                      │
└───────────────────────────────────────────────────────────────────┘
```

---

## Key Go Patterns Used

### 1. Struct Method Receivers

```go
// cloud_client.go - Methods bound to struct
// See cloud_client.go lines 117-140 for the actual EnlightenCloudClient implementation
type EnlightenCloudClient struct {
    systemID    string
    apiKey      string
    accessToken string
    timezone    *time.Location  // Timezone for reporting/queries
    httpClient  *http.Client
    cacheUsed   bool            // Tracks if cache was used for the last request
}

func (c *EnlightenCloudClient) GetMetricsFromCloud(ctx context.Context, testDate time.Time) (*LocalMetrics, bool, error) {
    // 'c' is the receiver - access struct fields via c.systemID, etc.
    // ctx is used for request cancellation and timeout handling
}
```

**Why:** Go does not have classes, but struct methods provide similar encapsulation.
The receiver acts like `this` or `self` in other languages.

**See also:** [cloud_client.go](cloud_client.go) for the complete implementation.

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

**See also:** [oauth.go](oauth.go) and [aggregator.go](aggregator.go) for error handling patterns throughout the codebase.

### 3. Constructor Overloading with Defaults

This codebase uses a simplified approach to optional configuration:

```go
// display.go - Constructor with required timezone and optional colors
// See display.go lines 80-100 for the actual implementation
func NewDisplayWithColorsAndTimezone(colors ColorConfig, tz *time.Location) *Display {
    // Merge with defaults
    colors.mergeWithDefaults(defaultColors)
    return &Display{colors: colors, timezone: tz}
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
// cloud_client.go - Guaranteed cleanup even on error
resp, err := c.httpClient.Do(req)
if err != nil {
    return nil, err
}
defer resp.Body.Close()  // Always executes when function returns
```

**Why:** Prevents resource leaks. `defer` executes in LIFO order.

### 6. Channels and Signal Handling (Interruptible Waiting)

This pattern is used **only in continuous monitoring mode** (without `--once`). It is not about parallelism—the work is completely serial. It solves a specific problem: **how to wait for a timer but respond instantly to Ctrl+C**.

```go
// main.go - Graceful shutdown with interruptible wait
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

ticker := time.NewTicker(1 * time.Hour)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        // Timer fired - do periodic work
        fetchAndDisplay(ctx, ...)
    case <-ctx.Done():
        // Signal received - exit gracefully
        display.ShowInfo("Shutting down gracefully...")
        return  // defer ticker.Stop() runs here
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
// cloud_client.go - Mapping JSON to Go structs
type TelemetryResponse struct {
    LastReportedAggregateSOC string              `json:"last_reported_aggregate_soc,omitempty"`
    Intervals                []TelemetryInterval `json:"intervals"`
}
```

**Why:** Struct tags tell the JSON encoder/decoder how to map field names.
Go convention is PascalCase; JSON convention is often snake_case.

### 8. Package-Level Variables (Use Sparingly)

```go
// oauth.go - Caching tokens at package level
var (
    tokenCache     *TokenCache  // Shared token cache (singleton)
    // Note: In this codebase, token cache is accessed from main goroutine only,
    // so explicit mutex locking is not needed. If accessed from multiple goroutines,
    // you would add: tokenCacheLock sync.Mutex
)
```

**Why:** Sometimes necessary for caching, but prefer dependency injection
when possible to improve testability. In this codebase, the token cache is
only accessed from the main goroutine, so no mutex is needed.

---

## File Descriptions

### Core Application Files

| File                           | Responsibility                                         | Key Lines                              |
|--------------------------------|--------------------------------------------------------|----------------------------------------|
| [main.go](main.go)             | CLI entry, flag parsing, run modes (once/continuous)   | Lines 73-352                           |
| [config.go](config.go)         | YAML loading, validation, hex-to-ANSI conversion       | Lines 159-260                          |
| [aggregator.go](aggregator.go) | Orchestrates data collection, combines system metrics  | Lines 26-209 (tables), 264-370 (logic) |

### API Client Files

| File                               | Responsibility                             | Key Lines                                    |
|------------------------------------|--------------------------------------------|----------------------------------------------|
| [cloud_client.go](cloud_client.go) | Enlighten Cloud API v4, telemetry fetching | Lines 145-157 (helper), 397-660 (main logic) |
| [oauth.go](oauth.go)               | OAuth 2.0 token refresh, caching           | Lines 73-200                                 |
| [setup_oauth.go](setup_oauth.go)   | Interactive browser-based OAuth setup      | Full file                                    |

### Data Processing Files

| File                                     | Responsibility                                    | Key Lines    |
|------------------------------------------|---------------------------------------------------|--------------|
| [api_cache.go](api_cache.go)             | Disk caching, cache management, URL normalization | Lines 85-230 |
| [cache_cli.go](cache_cli.go)             | Cache inspection and management CLI commands      | Full file    |
| [response_parser.go](response_parser.go) | JSON parsing helpers for telemetry data           | Lines 11-60  |

### Utility Files

| File                                | Responsibility                                  | Key Lines      |
|-------------------------------------|-------------------------------------------------|----------------|
| [display.go](display.go)            | ANSI formatting, report structure               | Lines 114-260  |
| [constants.go](constants.go)        | `Reset` and `Bold` ANSI constants               | Full file      |
| [timezone.go](timezone.go)          | Timezone loading and date boundary calculations | Lines 7-50     |
| [url_builder.go](url_builder.go)    | Telemetry URL construction helpers              | Full file      |
| [validation.go](validation.go)      | Test mode validation (--test flag)              | Lines 30-150   |

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
   - Read `config.go` - See how Go handles YAML and struct validation
   - Trace `aggregator.go` - Follow data from systems to output (see [GO_CONCEPTS.md](GO_CONCEPTS.md) for Go concepts)
   - Study `cloud_client.go` - Learn HTTP client patterns and error handling

4. **Explore Advanced Features**
   - Explore `api_cache.go` - Understand caching and file I/O
   - Review `display.go` - See terminal formatting techniques
   - Study `oauth.go` - Understand token management and refresh patterns

5. **Deep Dive into Concurrency**
   - Read **[GO_CONCEPTS.md](GO_CONCEPTS.md#channels-and-signals)** for detailed explanation of channels and signal handling
   - See how `select` statement enables graceful shutdown

### Key Files for Learning Go Patterns

| Pattern                | Files to Study                           | What to Look For                                                      |
|------------------------|------------------------------------------|-----------------------------------------------------------------------|
| **Error Handling**     | `oauth.go`, `cloud_client.go`            | `%w` error wrapping, error propagation                                |
| **Channels & Select**  | `main.go`                                | `select` statement, signal handling, graceful shutdown                |
| **Concurrency**        | `main.go`                                | Channels, select statement, signal handling (single-threaded execution) |
| **Struct Methods**     | All files                                | Pointer vs value receivers, method design                             |
| **JSON Parsing**       | `response_parser.go`, `cloud_client.go`  | Struct tags, JSON marshaling/unmarshaling                             |
| **Defer Usage**        | Throughout                               | Resource cleanup, guaranteed execution                                |
| **Interfaces**         | Throughout                               | Implicit satisfaction, dependency injection                           |

### Code Reading Strategy

Each file has package-level documentation explaining its purpose and design decisions. When reading code:

1. **Start with package comments** - They explain the file purpose
2. **Reference [GO_CONCEPTS.md](GO_CONCEPTS.md)** - Explains intermediate Go concepts used in the code
3. **Follow function calls** - Trace execution from `main()` through the call stack
4. **Study error handling** - See how errors are wrapped and propagated
5. **Understand data structures** - See how structs are designed and used

### Related Documentation

- **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** - Go concepts and patterns
- **[GO_CONCEPTS.md](GO_CONCEPTS.md)** - Go concepts including channels and signals
- **[OAUTH_SETUP.md](OAUTH_SETUP.md)** - OAuth authentication explained
- **[README.md](README.md)** - User documentation and usage guide
