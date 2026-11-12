# Testing Guide

This document provides a comprehensive explanation of all testing approaches, patterns, tools, and conventions used in the Enphase Monitor codebase.

## Table of Contents

1. [Testing Philosophy](#testing-philosophy)
2. [Test Organization](#test-organization)
3. [Testing Patterns](#testing-patterns)
4. [Tools and Frameworks](#tools-and-frameworks)
5. [Testing Conventions](#testing-conventions)
6. [Test Files Explained](#test-files-explained)
7. [Running Tests](#running-tests)

---

## Testing Philosophy

### Coverage Achievement

The project follows a pragmatic approach to testing:

| Package | Coverage | Status |
|---------|----------|--------|
| urlbuilder | 100.0% | ✅ Full coverage - URL construction |
| display | 99.3% | ✅ Near-full - output formatting |
| cli | 96.5% | ✅ Near-full - CLI interface, grouped `--help`, maintenance-flag validation |
| parser | 94.8% | ✅ Near-full - JSON parsing |
| timezone | 92.7% | ✅ High - timezone handling |
| aggregator | 89.2% | ✅ High - data aggregation |
| weather | 88.5% | ✅ High - Open-Meteo client |
| credentials | 85.0% | ✅ High - credential pool + monthly quota |
| config | 84.5% | ✅ High - YAML parsing |
| history | 82.6% | ✅ High - per-day record schema/write/index |
| api | 81.9% | ✅ High - HTTP client |
| geocode | 81.5% | ✅ High - ZIP geolocation |
| location | 81.0% | ✅ High - location resolver/cache |
| cache | 74.8% | ✅ Adequate - file caching |
| constants | 72.7% | ✅ Adequate - pure constants (some portal/quota constants untested) |
| rawstore | 70.4% | ✅ Adequate - raw archive writer, cache import, manifest verification |
| oauth | 61.7% | ✅ Adequate - OAuth flows (browser-driven path verified manually) |
| app | 52.5% | ⚠️ Moderate - application glue: range-pull orchestration (exercised end-to-end via api tests), raw-weather/provenance and offline Enphase verification (unit-tested); the live `VerifyEnphaseArchive` call path is not exercised |
| pge | 37.4% | ⚠️ Low - ESPI decoding, DST handling, sanity checks, raw reprocess/verify are unit-tested against real redacted feeds; the browser-pull path is verified manually |
| browser | 16.3% | ⚠️ Low - headed-Chrome launcher, verified manually |
| enphase | 14.3% | ⚠️ Low - developer-portal scraping, verified manually |
| main.go | 0.0% | ✅ **Acceptable** - entry point |

**Overall**: 56.7% coverage (the `browser`, `enphase`, and `pge` browser-pull packages drive a real Chrome session against live portals and are verified manually rather than in CI, which pulls the total down; `app` live paths are covered end-to-end through the `internal/api` tests rather than direct unit tests)

### Why main.go Has 0% Coverage

The `main.go` file is pure orchestration (~940 lines) and has 0% coverage by design. This is an **industry standard** because:

1. **Cannot unit test**: `main()` function, `os.Exit()`, signal handling
2. **All logic tested**: All functions `main.go` calls are tested in internal packages
3. **Industry examples**: Docker CLI, kubectl, Terraform, Hugo all have 0% coverage for main files
4. **Best practice**: Extract testable logic to internal packages (which we do)

### Testing Priorities

1. **Critical paths**: Business logic (aggregation, parsing, PG&E feed decoding)
2. **Complex logic**: OAuth flows, HTTP clients, caching
3. **Public APIs**: Exported functions and methods
4. **Edge cases**: Error handling, boundary conditions
5. **Performance**: Benchmarks for hot paths

---

## Test Organization

### File Naming Patterns

This codebase uses two test file organization patterns:

#### Pattern 1: Standard 1:1 Mapping

Most packages follow the simple convention where each source file has one corresponding test file:

| Source File | Test File | Purpose |
|-------------|-----------|---------|
| `config.go` | `config_test.go` | Configuration tests |
| `constants.go` | `constants_test.go` | Constants validation |
| `display.go` | `display_test.go` | Display formatting tests (includes `ShowTrueUpReport` tests) |
| `timezone.go` | `timezone_test.go` | Timezone handling tests |
| `trueup.go` | `trueup_test.go` | True-Up Window end-date logic (`trueUpWindowEnd`): Current Period vs Past True-Up Period; `buildTrueUpReport` field mapping, Net Flow, per-System entries |

#### Pattern 2: Complex 1:Many Mapping

For packages with extensive functionality or different test concerns, tests are split by category:

**Cache Package** (3 test files):
- `cache.go` → 3 test files:
  - `cache_test.go` - State management tests (CacheDisabled getter/setter, ResetState)
  - `cache_functions_test.go` - Core functionality tests (RedactURLKey, normalize, save/load, ClearCacheForDate)
  - `cli_test.go` - CLI utilities tests (ListCacheEntries, ClearTodayCache, parseCacheResponse)

The per-credential monthly budget is covered in `internal/credentials/quota_test.go`, and the per-request cache/live decision (cache disabled, Past Period cache hit, 429/503/network errors propagating instead of serving stale cache) is pinned by [`internal/api/client_caching_test.go`](#integration-test-files).

**OAuth Package** (4 test files):
- `oauth.go` / `browser.go` → 4 test files:
  - `oauth_test.go` - Basic unit tests (272 lines)
  - `oauth_functional_test.go` - Integration tests with mock HTTP servers (679 lines)
  - `oauth_edge_cases_test.go` - Edge case and error path tests (441 lines)
  - `browser_test.go` - Browser-driven OAuth authorization flow tests (39 lines)

**Benefits of 1:Many Pattern**:
- ✅ **Clarity**: Test file name indicates test category
- ✅ **Maintainability**: Related tests grouped together
- ✅ **Readability**: Smaller files easier to navigate (210-652 lines vs 516-1310 lines)
- ✅ **History**: Shows evolution (original → functional → edge cases)
- ✅ **Focus**: Can run specific test categories independently

### Test Categories

The codebase includes four types of tests:

| Category | Suffix | Purpose | Example |
|----------|--------|---------|---------|
| **Unit Tests** | `*_test.go` | Test individual functions/methods | `config_test.go` |
| **Functional Tests** | `*_functional_test.go` | Test end-to-end flows | `oauth_functional_test.go` |
| **Edge Case Tests** | `*_edge_cases_test.go` | Test error paths and boundaries | `oauth_edge_cases_test.go` |
| **Benchmark Tests** | `*_bench_test.go` | Measure performance | `parser_bench_test.go` |

---

## Testing Patterns

This section describes the comprehensive testing patterns used throughout the codebase. Each pattern is illustrated with real examples from the project.

### Pattern 1: Table-Driven Tests

**What**: Define test cases as a slice of structs, iterate and run each case.

**Why**: 
- Reduces code duplication
- Makes it easy to add new test cases
- Clear separation between test data and test logic
- Self-documenting test cases

**Example** (`parser_test.go`):
```go
func TestParseTelemetryResponse(t *testing.T) {
    tests := []struct {
        name        string
        jsonData    string
        wantErr     bool
        wantCount   int
        validateSum float64
        fieldName   string
    }{
        {
            name: "valid production response",
            jsonData: `{"intervals": [{"end_at": 1234567890, "wh_del": 100.5}]}`,
            wantErr:     false,
            wantCount:   1,
            validateSum: 100.5,
            fieldName:   constants.FieldWhDel,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test logic here
        })
    }
}
```

**Used in**: All test files (`parser_test.go`, `config_test.go`, `flags_test.go`, `oauth_test.go`)

### Pattern 2: Mock Objects for Dependency Injection

**What**: Create mock implementations of interfaces to test components in isolation.

**Why**:
- Avoid external dependencies (network, filesystem, API calls)
- Control return values and errors for edge case testing
- Fast test execution (no I/O operations)
- Predictable and repeatable results

**Example** (`aggregator_test.go`):
```go
// MockCloudClient implements aggregator.CloudClient interface
type MockCloudClient struct {
    Metrics   *api.LocalMetrics
    CacheUsed bool
    Err       error
}

func (m *MockCloudClient) GetMetricsFromCloud(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (*api.LocalMetrics, bool, error) {
    if m.Err != nil {
        return nil, false, m.Err
    }
    return m.Metrics, m.CacheUsed, nil
}
```

**Used in**: `aggregator_test.go` (mock API client), `display_test.go` (mock writer)

### Pattern 3: Subtests with t.Run()

**What**: Use `t.Run()` to create named subtests within a test function.

**Why**:
- Better test isolation (each subtest runs independently)
- Clearer test output (shows which specific case failed)
- Can run specific subtests: `go test -run TestName/SubtestName`
- Parallel execution support with `t.Parallel()`

**Example** (`parser_test.go`):
```go
func TestSumIntervalValues(t *testing.T) {
    intervals := []TelemetryInterval{
        {EndAt: 1234567890, WhDel: 100.5, WhImported: 50.0, WhExported: 10.0, Enwh: 75.0},
        // ...
    }

    tests := []struct {
        name      string
        fieldName string
        expected  float64
    }{
        {name: "sum wh_imported", fieldName: constants.FieldWhImported, expected: 150.0},
        {name: "unknown field", fieldName: "unknown", expected: 0.0},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := SumIntervalValues(intervals, tt.fieldName)
            if result != tt.expected {
                t.Errorf("SumIntervalValues(%s) = %.1f, want %.1f", tt.fieldName, result, tt.expected)
            }
        })
    }
}
```

**Used in**: All test files with table-driven tests

### Pattern 4: Mock HTTP Servers for Integration Tests

**What**: Use `httptest.NewServer` to create fake HTTP servers for testing API clients.

**Why**:
- Test real HTTP interactions without external dependencies
- Control response codes, headers, and body content
- Test error handling (timeouts, malformed responses, etc.)
- No API Budget constraints or network issues

**Example** (`oauth_functional_test.go`):
```go
func TestExchangeCodeForToken_Success(t *testing.T) {
    // Create mock server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Validate request
        if r.Method != "POST" {
            t.Errorf("Expected POST, got %s", r.Method)
        }
        
        // Send mock response
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(200)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "access_token":  "test-access-token",
            "refresh_token": "test-refresh-token",
            "expires_in":    3600,
        })
    }))
    defer server.Close()

    // Test against mock server
    apiConfig := &config.APIConfig{
        AuthorizationURL: server.URL,
        // ...
    }
    // Run test...
}
```

**Used in**: `oauth_functional_test.go`, `client_functional_test.go`

### Pattern 5: Writer Injection for Output Testing

**What**: Inject `io.Writer` (like `bytes.Buffer`) to capture and verify output.

**Why**:
- Test output formatting without printing to stdout
- Verify exact output content (headers, colors, metrics)
- Fast execution (in-memory buffer vs terminal I/O)
- No manual visual inspection needed

**Example** (`display_test.go`):
```go
func TestShowMetrics_ContainsHeader(t *testing.T) {
    var buf bytes.Buffer
    tz := mustLoadLocation(t, "US/Pacific")  // test helper; fails test if timezone invalid
    
    // Inject buffer as writer
    d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)
    
    // Generate output
    d.ShowMetrics(metrics)
    
    // Verify output
    output := buf.String()
    if !strings.Contains(output, "ENPHASE") {
        t.Error("Expected header not found in output")
    }
}
```

**Used in**: `display_test.go` (all output tests), `verify_test.go`, `rawweather_test.go`

**Additional Example** (`internal/pge/verify_test.go`): the verifier reports its findings to a writer, so the test asserts on them.
```go
var out bytes.Buffer
res, err := VerifyPGERaw(archiveDir, hist, loc, &out)

// A hand-edited history record must be reported with both totals in the message
if err != nil || res.OK() || len(res.Differ) != 1 || !strings.Contains(out.String(), "99.900") {
    t.Fatalf("edited-record verify = %+v, %v\n%s", res, err, out.String())
}
```

### Pattern 6: Test Fixtures with Helper Functions

**What**: Create helper functions to generate common test data structures.

**Why**:
- Reduce duplication in test setup
- Provide sensible defaults for test data
- Make tests more readable (focus on what's being tested)
- Easy to modify test data in one place

**Example** (`aggregator_test.go`):
```go
// Helper function to create test metrics
func makeTestMetrics(production, consumption, gridImport, gridExport float64) *api.LocalMetrics {
    return &api.LocalMetrics{
        ProductionToday:        production,
        ConsumptionToday:       consumption,
        GridImportToday:        gridImport,
        GridExportToday:        gridExport,
        BatteryChargedToday:    0,
        BatteryDischargedToday: 0,
        BatterySOC:             50,
        Timestamp:              time.Now(),
    }
}
```

**Used in**: `aggregator_test.go`, `display_test.go`, `rawweather_test.go`

### Pattern 7: Error Path Testing

**What**: Deliberately cause errors to test error handling code paths.

**Why**:
- Verify errors are handled gracefully
- Test error messages and context wrapping
- Ensure cleanup happens even on error (defer statements)
- Validate error propagation through the call stack

**Example** (`oauth_edge_cases_test.go`):
```go
func TestRefreshToken_NetworkError(t *testing.T) {
    // Create server that simulates network failure
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Close connection immediately
        hj, ok := w.(http.Hijacker)
        if ok {
            conn, _, _ := hj.Hijack()
            conn.Close()
        }
    }))
    defer server.Close()
    
    // Test should handle network error gracefully
    token, err := RefreshToken(ctx, apiConfig)
    if err == nil {
        t.Error("Expected error from network failure")
    }
    if token != nil {
        t.Error("Expected nil token on error")
    }
}
```

**Used in**: `oauth_edge_cases_test.go`, `client_test.go`, `aggregator_test.go`

### Pattern 8: Benchmark Tests

**What**: Use `testing.B` to measure and compare performance.

**Why**:
- Identify performance bottlenecks
- Verify optimizations actually improve performance
- Track performance over time (regression detection)
- Compare different implementation approaches

**Example** (`parser_bench_test.go`):
```go
func BenchmarkParseTelemetryResponse(b *testing.B) {
    jsonData := []byte(`{"intervals": [{"end_at": 123, "wh_del": 100}]}`)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := ParseTelemetryResponse(jsonData)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

**Used in**: `parser_bench_test.go`, `aggregator_bench_test.go`

### Pattern 9: State Reset for Test Isolation

**What**: Reset package-level state before/after tests to ensure isolation.

**Why**:
- Prevent test pollution (one test affecting another)
- Make tests order-independent
- Enable parallel test execution
- Clear caches and state between test runs

**Example** (`cache_test.go`):
```go
func TestCacheDisabledGetterSetter(t *testing.T) {
    // Reset to known state
    ResetState()

    // Run test
    SetCacheDisabled(true)
    if !CacheDisabled() {
        t.Error("CacheDisabled should be true after SetCacheDisabled(true)")
    }

    // Clean up (or use defer)
    defer ResetState()
}
```

**Used in**: `cache_test.go`, `oauth_test.go`

### Pattern 10: Context Cancellation Testing

**What**: Test behavior when context is cancelled (timeout, interrupt).

**Why**:
- Verify graceful cancellation of long-running operations
- Test cleanup on cancellation
- Validate error handling when context expires
- Ensure no resource leaks on cancellation

**Example** (`aggregator_test.go`):
```go
func TestGetAggregatedMetrics_ContextCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())

    // Create slow mock that checks context
    mockClient := &SlowMockClient{delay: 1 * time.Second}

    // Cancel immediately
    cancel()

    // Should return context error
    _, err := aggregator.GetAggregatedMetrics(ctx, systems, apiConfig, time.Time{}, constants.QueryModeDay, tz)
    if err == nil || !errors.Is(err, context.Canceled) {
        t.Error("Expected context.Canceled error")
    }
}
```

**Used in**: `aggregator_test.go`, `client_test.go`

---

### Pattern 11: Real Redacted Fixtures with Known Answers

**What**: Build fixtures from real source data, strip every identifier, and assert answers that were checked against an independent source.

**Why**: A synthetic, sandbox-shaped feed hides the defects that matter (a doubled import register, a local-clock-as-UTC encoding, a Wh/kWh scale error), and a test that only asserts "N readings were kept" pins buggy behavior. Real feeds carry the quirks that matter (four registers with two sharing a direction, 899-second intervals, `powerOfTenMultiplier` 3 on Wh values, a gap-labelled slot on the spring-forward day).

**Example** (`internal/pge/espi_test.go`, fixtures in `internal/pge/testdata/`):
```go
// pge_real_2025-11-08.xml is a real export trimmed to one day, with account,
// meter and UUID identifiers removed. Known answer: delivered 29.724 kWh,
// received 25.675 kWh (Enphase's own meters: 28.4 / 25.2).
func TestParseReadings_RealFeedUsesGrossAndReceivedOnly(t *testing.T) { /* 192 readings, no negative import, totals */ }

// Two more real days pin the daylight-saving behavior: 2026-03-08 is 92 intervals
// with no slot in the nonexistent 02:00 hour; 2025-11-02 is 96 of 100 and CheckDays warns.
func TestAggregate_RealSpringForwardDay(t *testing.T) { /* ... */ }
func TestAggregate_RealFallBackDay(t *testing.T)      { /* ... */ }
```

Guidelines: keep the real structure and values, redact only identifiers (grep the fixture for `urn:`/`Subscription` before committing), state in the fixture header what it is and what the known answer is, and prefer asserting a total that was cross-checked (against Enphase or a statement) over one merely observed. Fixtures that were cross-checked are `pge_real_2025-11-08.xml`; the two DST fixtures pin observed counts that are explained by the source (the raw feed itself has 96 labels on the fall-back day).

**Used in**: `espi_test.go`

### Pattern 12: Archive Round-Trip and Tamper Tests

**What**: Exercise the raw-archive code against a temporary directory, then deliberately corrupt the data and assert the verifier notices.

**Why**: A verifier that only ever sees good data proves nothing. Each test builds a clean state that must verify, then breaks one thing (edits a history record, alters a raw file, deletes one, drops a stray file) and asserts the specific finding.

**Example** (`internal/pge/verify_test.go`):
```go
rawstore.Dir = filepath.Join(root, "raw")
t.Cleanup(func() { rawstore.Dir = "" })      // global state: always reset
// archive -> ReprocessRaw into history -> VerifyPGERaw is clean
// hand-edit import_kwh                      -> Differ finding, with both totals in the message
// append a byte to the archived download    -> Manifest.Mismatched finding
```

Notes: `rawstore.Dir` is package-level state that is empty by default (so ordinary tests never write files); a test that sets it must reset it in `t.Cleanup`. Tests that call `CheckDays` must pass local-midnight bounds, not UTC midnight: a UTC midnight converts to the previous local date and the day is silently skipped, so the test would pass vacuously.

**Used in**: `internal/rawstore/rawstore_test.go`, `internal/pge/verify_test.go`, `internal/app/rawweather_test.go`

---

## Tools and Frameworks

### Standard Library Testing

The project uses Go's built-in `testing` package exclusively (no external test frameworks). This provides:

- **`testing.T`**: Basic unit testing
- **`testing.B`**: Benchmarking
- **`t.Run()`**: Subtests and parallel execution
- **`t.Helper()`**: Mark helper functions (cleaner error traces)
- **`httptest`**: Mock HTTP servers
- **`bytes.Buffer`**: Capture output for verification

### Test Execution Commands

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific package
go test ./internal/parser -v

# Run specific test
go test -run TestParseTelemetryResponse ./internal/parser

# Run specific subtest
go test -run TestSumIntervalValues/sum_wh_imported ./internal/parser

# Run benchmarks
go test -bench=. ./internal/parser
go test -bench=. -benchmem ./...

# Run with race detector
go test -race ./...

# Run tests in parallel
go test -parallel 4 ./...
```

### Coverage Analysis

```bash
# Generate coverage profile
go test -coverprofile=coverage.out ./...

# View coverage by package
go tool cover -func=coverage.out

# View HTML coverage report
go tool cover -html=coverage.out

# Check if coverage meets threshold
go test -cover ./... | grep -E "coverage: [0-9]+\.[0-9]+%" | awk '{print $2}'
```

---

## Testing Conventions

### Go testing practices (go-testing skill)

Tests in this repo follow the [go-testing](https://github.com/golang/go/wiki/CodeReviewComments#useful-test-failures) style so that failures are diagnosable without reading the test source:

1. **Failure message format**  
   Include: what failed, function inputs, **got**, **want**. Prefer:  
   `YourFunc(%v) = %v, want %v`  
   Always print **got before want**.

2. **No assertion libraries**  
   Use `t.Error`/`t.Fatalf` and standard comparisons. For structs/slices use `cmp.Diff(want, got)` and report:  
   `t.Errorf("Func() mismatch (-want +got):\n%s", diff)`.

3. **t.Error vs t.Fatal**  
   Use `t.Error` to report and continue (e.g. multiple checks in one test). Use `t.Fatal` only when continuing is meaningless (e.g. setup failure, nil dependency).

4. **Test helpers**  
   Helpers that take `*testing.T` must call `t.Helper()` first so failures point to the caller. Use `t.Fatal` for setup failures in helpers; use `t.Cleanup()` for teardown.

5. **Error semantics**  
   Prefer testing error semantics (e.g. `errors.Is(err, ErrX)`) over comparing `err.Error()` strings.

6. **Environments that block network**  
   Some tests use `httptest.NewServer` and need to bind a port. In sandboxes or restricted CI you may see "bind: operation not permitted". Run `go test ./...` where binding is allowed (see [Environment note](#environment-note-tests-that-need-a-network-listener) above).

### Naming Conventions

1. **Test Files**: `*_test.go` suffix
2. **Test Functions**: `Test<FunctionName>` prefix
3. **Benchmark Functions**: `Benchmark<FunctionName>` prefix
4. **Helper Functions**: No special prefix, but use `t.Helper()`
5. **Mock Types**: `Mock<InterfaceName>` prefix
6. **Test Cases**: Use descriptive `name` field in struct

### Test Organization

1. **One package, one test package**: Tests live in same package (white-box testing)
2. **Imports**: Use package imports, not relative imports
3. **Setup/Teardown**: Use `defer` for cleanup, not separate functions
4. **Test Data**: Keep test data close to tests (inline or helper functions) or under the `test-data/` directory

### Error Handling in Tests

1. **Fatal vs Error**:
   - Use `t.Fatal()` for setup failures (test cannot continue)
   - Use `t.Error()` for assertion failures (test can continue to check more cases)

2. **Error Messages**: Include context and expected/actual values
   ```go
   if got != want {
       t.Errorf("GetValue() = %v, want %v", got, want)
   }
   ```

3. **Helper Functions**: Mark with `t.Helper()` for better error traces
   ```go
   func assertEqual(t *testing.T, got, want interface{}) {
       t.Helper()
       if got != want {
           t.Errorf("got %v, want %v", got, want)
       }
   }
   ```

### Test Coverage Guidelines

| Coverage Level | Expectation | Examples |
|----------------|-------------|----------|
| **100%** | Pure logic, no I/O | `urlbuilder` (and `display`, near-full) |
| **80-100%** | Core business logic | `parser`, `aggregator`, `weather` |
| **60-80%** | Complex I/O operations | `cache`, `api`, `oauth`, `rawstore` |
| **40-60%** | Glue code | `app` |
| **0%** | Entry points only | `main.go` |

---

## Test Files Explained

### Unit Test Files (Standard Pattern)

These files test individual functions and methods in isolation. Per-package coverage figures are listed in the [Coverage Achievement](#coverage-achievement) table at the top of this document.

| Test File | Source File | Key Tests |
|-----------|-------------|-----------|
| `constants_test.go` | `constants.go` | Constant values, helper functions |
| `display_test.go` | `display.go` | Output formatting, color codes |
| `urlbuilder_test.go` | `urlbuilder.go` | URL construction, parameter encoding |
| `timezone_test.go` | `timezone.go` | Timezone loading, date boundaries |
| `config_test.go` | `config.go` | YAML parsing, validation, color conversion |
| `parser_test.go` | `parser.go` | JSON parsing, interval summing |
| `flags_test.go` | `flags.go` | CLI flag parsing and defaults; `TestPrintUsage_ListsEveryFlagOnceAndGroupsRaw` (every registered flag appears exactly once in the grouped `--help`, the raw/verify flags under their own heading); `TestValidateMaintenance` (at most one maintenance command; only the raw weather commands take a date range) |
| `cache_commands_test.go` | `cache_commands.go` | Cache management command handlers (clear today, clear all) |
| `runner_test.go` | `runner.go` | RunOnce execution mode |
| `setup_test.go` | `setup.go` | `FormatDateForQueryMode`, `CreateOAuthAdapter`, `SetupDisplay`, `ParseTestDate`, `ConfigureModes`, `GetAggregatorTypes`, plus `RunOnce` context-cancellation guard |
| `trueup_test.go` | `trueup.go` | `buildTrueUpReport` field mapping, net flow, per-system entries |
| `aggregator_test.go` | `aggregator.go` | Multi-system aggregation with mock clients |
| `espi_test.go` | `espi.go`, `sanity.go` | PG&E ESPI parsing and aggregation against real redacted feeds: gross/received register selection (a net register never double-counts; ambiguous or mismatched registers fail), local-clock decoding (wall-clock and true-UTC accepted, anything else rejected), the spring-forward gap label, the real DST days (92, and 96 of 100 with a warning), `CheckDays`, `expectedIntervals` |
| `verify_test.go` | `verify.go` | `VerifyPGERaw` round-trip: clean, hand-edited record, tampered raw archive; `orderArchives` sorts by manifest fetch time, not file name |
| `browserpull_test.go` | `browserpull.go` | One-year export-range clamp; `archiveRawDownload` keeps the bytes and never overwrites |
| `rawstore_test.go` | `rawstore/*.go` | Save/manifest, `RedactURL`, `ImportEnphaseCache` (byte-exact, idempotent, skips non-responses), `VerifySource` (altered/missing/unlisted), `EntriesSince`/`VerifyEntry` |
| `rawweather_test.go` | `app/rawweather.go`, `verifyenphase*.go` | `HistoryDates` range uses calendar dates (not timezone-mixed instants), `ApplyRawWeather` (weather only, only when changed, provenance kept across re-runs), `diffRecords`, `VerifyEnphaseRaw` (reproduces a day, catches a wrong record) |

### Integration Test Files

These files test component interactions:

| Test File | Tests | Purpose |
|-----------|-------|---------|
| `client_functional_test.go` | API client tests | HTTP interactions with mock server |
| `client_caching_test.go` | Characterization tests | Pin `makeCachedAPIRequest` branches (no-cache mode; 429/503/network errors propagate instead of serving stale cache) via a switchable mock server with one stable cache key |
| `client_lifetime_test.go` | Lifetime Data tests | Month, Year, and True-Up Mode queries via `_lifetime` API endpoints |
| `oauth_functional_test.go` | OAuth flows | Token exchange with mock auth server |
| `testmain_test.go` | (no test functions) | `TestMain`: package-level setup redirecting all cache I/O to a temp dir so api tests never touch the production cache (`rawstore.Dir` stays empty there, so the raw archive is never written either) |

### OAuth Test Files

These files test OAuth authentication flows:

| Test File | Focus | Coverage |
|-----------|-------|----------|
| `oauth_test.go` | Basic unit tests | Token refresh, URL generation |
| `oauth_functional_test.go` | Integration tests | Full OAuth flows with mock HTTP server |
| `oauth_edge_cases_test.go` | Error handling | Network failures, malformed responses, validation errors |

### Mock-Heavy Test Files

These files use dependency injection extensively:

| Test File | Mocks | Purpose |
|-----------|-------|---------|
| `aggregator_test.go` | MockCloudClient | Test aggregation without real API calls |
| `display_test.go` | bytes.Buffer | Test output without printing to stdout |

### Performance Test Files

These files measure and optimize performance:

| Test File | Benchmarks | Measures |
|-----------|------------|----------|
| `parser_bench_test.go` | JSON parsing | Parsing speed, memory allocation |
| `aggregator_bench_test.go` | Aggregation | Multi-system aggregation performance |

---

## Running Tests

### Environment note: tests that need a network listener

Some tests use `httptest.NewServer` (e.g. `internal/api`, `internal/oauth`) and must be able to bind a local port. In restricted environments (e.g. sandboxes, some CI) you may see:

```text
panic: httptest: failed to listen on a port: listen tcp6 [::1]:0: bind: operation not permitted
```

Run `go test ./...` in an environment that allows binding (e.g. your machine or CI with network permissions). There is no build tag or skip for these tests; they are standard unit/integration tests.

### Test helpers (timezone and errors)

Tests that need a timezone use a helper so invalid timezones fail the test instead of being ignored: `tz := mustLoadLocation(t, "US/Pacific")`. The helper is defined in each test file that needs it (e.g. `internal/api/client_functional_test.go`, `internal/display/display_test.go`, `internal/aggregator/aggregator_test.go`). This follows the go-code-review guideline to avoid discarding errors in tests.

### Unit Tests

Run all unit tests with coverage:

```bash
# Run all tests
make test

# Run tests for a specific package
go test -v ./internal/cache/

# Run a specific test
go test -v ./internal/app/ -run TestRunOnce
```

#### Testing Refactor Helpers

Unit tests cover helpers introduced during go-style-core refactoring (reduce nesting, default+override):

| Test File | Test(s) | Helper(s) | Purpose |
|-----------|---------|-----------|---------|
| `client_test.go` | `TestTryLoadPastDateCache_InvalidURL`, `TestTryLoadPastDateCache_NoMatch` | `tryLoadPastDateCache` | Past-date cache fallback when primary lookup fails (invalid URL, no matching cache). |

---

## Related Documentation

- **[README.md](../README.md)** - User guide and project overview
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System architecture and design
- **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** - Go patterns used in codebase
