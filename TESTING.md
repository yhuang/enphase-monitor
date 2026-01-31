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
| constants | 100.0% | ✅ Full coverage - pure constants |
| display | 100.0% | ✅ Full coverage - output formatting |
| urlbuilder | 100.0% | ✅ Full coverage - URL construction |
| validation | 96.6% | ✅ Near-full - metrics validation |
| timezone | 93.3% | ✅ Near-full - timezone handling |
| config | 82.4% | ✅ High - YAML parsing |
| parser | 80.8% | ✅ High - JSON parsing |
| aggregator | 80.0% | ✅ High - data aggregation |
| app | 76.8% | ✅ Good - application setup |
| api | 74.4% | ✅ Good - HTTP client |
| cache | 66.9% | ✅ Good - file caching |
| cli | 47.6% | ✅ Adequate - CLI interface |
| oauth | 46.1% | ✅ Adequate - OAuth flows |
| main.go | 0.0% | ✅ **Acceptable** - entry point |

**Overall**: 70.4% coverage (20% above 50-60% typical Go project standard)

### Why main.go Has 0% Coverage

The `main.go` file is pure orchestration (171 lines) and has 0% coverage by design. This is an **industry standard** because:

1. **Cannot unit test**: `main()` function, `os.Exit()`, signal handling
2. **All logic tested**: All functions `main.go` calls are tested in internal packages
3. **Industry examples**: Docker CLI, kubectl, Terraform, Hugo all have 0% coverage for main files
4. **Best practice**: Extract testable logic to internal packages (which we do)

### Testing Priorities

1. **Critical paths**: Business logic (aggregation, parsing, validation)
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
| `display.go` | `display_test.go` | Display formatting tests |
| `timezone.go` | `timezone_test.go` | Timezone handling tests |

#### Pattern 2: Complex 1:Many Mapping

For packages with extensive functionality or different test concerns, tests are split by category:

**Cache Package** (3 test files):
- `cache.go` → 3 test files:
  - `cache_test.go` - State management tests
  - `cache_functions_test.go` - Functionality tests (516 lines)
  - `cli_test.go` - CLI utilities tests (119 lines)

**OAuth Package** (3 test files):
- `oauth.go` → 3 test files:
  - `oauth_test.go` - Basic unit tests (270 lines)
  - `oauth_functional_test.go` - Integration tests with mock HTTP servers (598 lines)
  - `oauth_edge_cases_test.go` - Edge case and error path tests (442 lines)

**Benefits of 1:Many Pattern**:
- ✅ **Clarity**: Test file name indicates test category
- ✅ **Maintainability**: Related tests grouped together
- ✅ **Readability**: Smaller files easier to navigate (161-598 lines vs 516-1310 lines)
- ✅ **History**: Shows evolution (original → functional → edge cases)
- ✅ **Focus**: Can run specific test categories independently

### Test Categories

The codebase includes five types of tests:

| Category | Suffix | Purpose | Example |
|----------|--------|---------|---------|
| **Unit Tests** | `*_test.go` | Test individual functions/methods | `config_test.go` |
| **Integration Tests** | `*_integration_test.go` | Test component interactions | `validation_integration_test.go` |
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

**Used in**: All test files (`parser_test.go`, `config_test.go`, `validation_test.go`, `oauth_test.go`)

### Pattern 2: Mock Objects for Dependency Injection

**What**: Create mock implementations of interfaces to test components in isolation.

**Why**:
- Avoid external dependencies (network, filesystem, API calls)
- Control return values and errors for edge case testing
- Fast test execution (no I/O operations)
- Predictable and repeatable results

**Example** (`aggregator_test.go`):
```go
// MockCloudClient implements api.CloudClient interface
type MockCloudClient struct {
    Metrics   *api.LocalMetrics
    CacheUsed bool
    Err       error
}

func (m *MockCloudClient) GetMetricsFromCloud(ctx context.Context, testDate time.Time) (*api.LocalMetrics, bool, error) {
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

**Example** (`validation_test.go`):
```go
func TestValidationTolerance(t *testing.T) {
    tests := []struct {
        name       string
        expected   float64
        actual     float64
        shouldPass bool
    }{
        {name: "within 10% tolerance", expected: 100.0, actual: 105.0, shouldPass: true},
        {name: "exceeds tolerance", expected: 100.0, actual: 115.0, shouldPass: false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test logic for this specific case
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
- No API rate limits or network issues

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
    tz, _ := time.LoadLocation("US/Pacific")
    
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

**Used in**: `display_test.go` (all output tests)

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

**Used in**: `aggregator_test.go`, `display_test.go`, `validation_test.go`

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
func TestCacheState_SetAndGet(t *testing.T) {
    // Reset to known state
    ResetState()
    
    // Run test
    SetTestMode(true)
    if !TestMode() {
        t.Error("TestMode should be true")
    }
    
    // Clean up (or use defer)
    defer ResetState()
}
```

**Used in**: `cache_test.go`, `oauth_test.go`

### Pattern 11: Validation Against Golden Data

**What**: Compare test output against known-good reference data.

**Why**:
- Test against real-world data
- Validate calculations against manual verification
- Integration testing with actual API responses (cached)
- Tolerance-based comparisons for floating-point values

**Example** (`validation_integration_test.go`):
```go
func TestValidateMetrics_RealData(t *testing.T) {
    // Load expected values from JSON file
    expected := loadExpectedValues("test-data/expected_values_2026-01-20.json")
    
    // Load cached API responses and aggregate
    metrics := aggregateFromCache("2026-01-20")
    
    // Validate with tolerance
    err := ValidateMetrics(metrics, "2026-01-20")
    if err != nil {
        t.Errorf("Validation failed: %v", err)
    }
}
```

**Used in**: `validation_integration_test.go`, `run-tests.sh`

### Pattern 12: Context Cancellation Testing

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
    _, err := aggregator.GetAggregatedMetrics(ctx, systems, apiConfig, time.Time{}, tz)
    if err == nil || !errors.Is(err, context.Canceled) {
        t.Error("Expected context.Canceled error")
    }
}
```

**Used in**: `aggregator_test.go`, `client_test.go`

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
go test -run TestValidation/within_tolerance ./internal/validation

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
| **100%** | Pure logic, no I/O | `constants`, `display`, `urlbuilder` |
| **80-100%** | Core business logic | `parser`, `aggregator`, `validation` |
| **60-80%** | Complex I/O operations | `cache`, `api`, `oauth` |
| **40-60%** | CLI/glue code | `cli`, `app` |
| **0%** | Entry points only | `main.go` |

---

## Test Files Explained

### Unit Test Files (Standard Pattern)

These files test individual functions and methods in isolation:

| Test File | Source File | Coverage | Key Tests |
|-----------|-------------|----------|-----------|
| `constants_test.go` | `constants.go` | 100% | Constant values, helper functions |
| `display_test.go` | `display.go` | 100% | Output formatting, color codes |
| `urlbuilder_test.go` | `urlbuilder.go` | 100% | URL construction, parameter encoding |
| `validation_test.go` | `validation.go` | 96.6% | Tolerance calculations, metric comparison, edge cases |
| `timezone_test.go` | `timezone.go` | 93.3% | Timezone loading, date boundaries |
| `config_test.go` | `config.go` | 82.4% | YAML parsing, validation, color conversion |
| `parser_test.go` | `parser.go` | 80.8% | JSON parsing, interval summing |

### Integration Test Files

These files test component interactions:

| Test File | Tests | Purpose |
|-----------|-------|---------|
| `validation_integration_test.go` | 7 test functions | End-to-end validation with real cached data and expected values files |
| `client_functional_test.go` | API client tests | HTTP interactions with mock server |
| `oauth_functional_test.go` | OAuth flows | Token exchange with mock auth server |

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

### Unit Tests

Run all unit tests with coverage:

```bash
# Run all tests
make test

# Run tests for a specific package
go test -v ./internal/cache/

# Run a specific test
go test -v ./internal/app/ -run TestValidateTestModeCache
```

### Integration Testing with --test Flag

The `--test` flag enables cache-only mode with validation against expected values. This requires:

1. **Cached API responses** - Run `./enphase-monitor --once` first to populate the cache
2. **Expected values file** - Create `test-data/expected_values_YYYY-MM-DD.json`

#### Early Cache Validation

The application validates cache existence before running in test mode:

```bash
# If cache doesn't exist for today:
$ ./enphase-monitor --test --once
ERROR: --test flag requires cached data, but no cache exists for 2026-01-30.

To populate the cache, run:
  ./enphase-monitor --once

Then retry with --test.
```

#### Missing Expected Values File

If cache exists but expected values file is missing, a helpful error is displayed:

```bash
$ ./enphase-monitor --test --date 2026-01-01 --once
Validation failed: no expected values file found for 2026-01-01.

To run validation, create the file:
  test-data/expected_values_2026-01-01.json

Example format:
  {
    "date": "2026-01-01",
    "systems": [
      {
        "id": "SYSTEM_ID",
        "name": "System Name",
        "expected": {
          "grid_import": 10.0,
          "grid_export": 5.0,
          "production": 20.0,
          ...
        }
      }
    ]
  }
```

#### Testing the Test Mode Itself

The test mode validation behavior is tested in:

| Test File | Test Functions | Purpose |
|-----------|----------------|---------|
| `cache_functions_test.go` | `TestHasCacheForDate` | Cache existence check |
| `setup_test.go` | `TestValidateTestModeCache` | Early validation with helpful errors |
| `validation_test.go` | `TestValidateMetrics_MissingFile_HelpfulError` | Improved error messages |

---

## Related Documentation

- **[README.md](README.md)** - User guide and project overview
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System architecture and design
- **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** - Go patterns used in codebase
