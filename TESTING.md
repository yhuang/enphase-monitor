# Testing Guide

## Overview

This project includes comprehensive unit tests to ensure code quality and prevent regressions during refactoring. All tests use Go's standard `testing` package and follow table-driven test patterns.

## Quick Start

### Run All Tests
```bash
make test
# or
go test -v ./...
```

### Run Tests with Coverage
```bash
make test-coverage
# Then view HTML report
go tool cover -html=coverage.out
```

### Run Specific Test
```bash
make test-one TEST=TestLoadConfig
# or
go test -v -run TestLoadConfig ./...
```

### Run Tests Without Cache
```bash
make test-verbose
# or
go test -v -count=1 ./...
```

## Test Files

### constants_test.go
Tests all constant values and helper functions:
- **TestIsRateLimitError**: Validates rate limit error detection (5 test cases)
- **TestConstants**: Validates all 30+ constant values (ANSI codes, display settings, date formats, HTTP status codes, error messages, energy conversion, validation tolerances, color conversion, timezone defaults, API URLs)

### config_test.go
Tests configuration loading and color conversion:
- **TestLoadConfig**: Configuration file loading with various scenarios (5 test cases)
  - Valid config with all fields
  - Missing systems (error case)
  - System without ID (error case)
  - Missing API config (error case)
  - Default refresh_interval fallback
- **TestConvertIfHex**: Hex color recognition and passthrough (5 test cases)
- **TestHexToANSI**: Hex to ANSI 256-color conversion (8 test cases)

### response_parser_test.go
Tests JSON parsing for Enphase API telemetry responses:
- **TestParseTelemetryResponse**: Flat array format parsing (5 test cases)
  - Production meter data
  - Consumption meter data
  - Empty intervals
  - Invalid JSON
  - Malformed structure
- **TestParseNestedTelemetryResponse**: Nested array format parsing (4 test cases)
  - Import telemetry data
  - Export telemetry data
  - Empty nested intervals
  - Invalid JSON
- **TestSumIntervalValues**: Field-based interval summation (5 test cases)
  - wh_del (production)
  - wh_imported (grid import)
  - wh_exported (grid export)
  - enwh (consumption)
  - Unknown field (returns 0)

### timezone_test.go
Tests timezone loading and date handling:
- **TestLoadTimezone**: Timezone string parsing (5 test cases)
  - Empty string (system timezone)
  - US/Pacific
  - America/New_York
  - UTC
  - Invalid timezone (graceful fallback)
- **TestGetDayBoundaries**: Day start/end calculations (2 test cases)
  - Specific past date
  - Zero time (today)
- **TestIsPastDate**: Past date detection (5 test cases)
  - Zero time (false)
  - Yesterday (true)
  - Tomorrow (false)
  - Today (false)
  - Last week (true)
- **TestParseDateInTimezone**: Date string parsing (4 test cases)
  - Valid date
  - Invalid format
  - Invalid date values
  - Empty string

### url_builder_test.go
Tests API URL construction:
- **TestBuildTelemetryURL**: Telemetry endpoint URL building (4 test cases)
  - production_meter endpoint
  - consumption_meter endpoint
  - energy_import_telemetry endpoint
  - battery endpoint
  - Validates: base URL, system ID, endpoint path, API key parameter, query structure

### validation_test.go
Tests validation tolerance calculations:
- **TestValidationTolerance**: 10% tolerance and 0.1 kWh minimum (9 test cases)
  - Within 10% tolerance
  - Exactly at 10% tolerance
  - Exceeds 10% tolerance
  - Small values with minimum tolerance (0.1 kWh)
  - Small values exceeding minimum tolerance
  - Zero expected value
  - Zero expected value exceeding tolerance
  - Both zero (valid)
  - Negative values within tolerance
- **TestPercentageDifferenceCalculation**: Percentage math validation (5 test cases)
  - 5% increase
  - 10% decrease
  - Zero expected with non-zero actual (infinite %)
  - Both zero (0%)
  - Small percentage rounds to zero

### cache_test.go
Tests cache state management and thread safety:
- **TestCacheState_ThreadSafety**: Concurrent access to cache state (uses sync.WaitGroup with multiple goroutines)
  - Tests TestMode, CacheDisabled, RateLimitWarningShown with concurrent access
  - Verifies no race conditions with mutex protection
- **TestResetState**: Verifies ResetState resets all flags
- **TestTestModeGetterSetter**: Tests TestMode flag get/set
- **TestCacheDisabledGetterSetter**: Tests CacheDisabled flag get/set
- **TestRateLimitWarningShownGetterSetter**: Tests RateLimitWarningShown flag get/set

### display_test.go
Tests display output formatting and testability:
- **TestNewDisplayWithWriter**: Verifies display can be created with custom writer
- **TestNewDisplayWithColorsAndTimezone**: Verifies default constructor
- **TestShowMetrics_ContainsHeader**: Verifies header is present in output
- **TestShowMetrics_ContainsMetricValues**: Verifies metric values appear in output
- **TestShowMetrics_CacheIndicator**: Verifies cache status display (cached/live)
- **TestShowMetrics_NetFlow**: Verifies net flow direction labels (import/export)
- **TestShowMetrics_IndividualSystems**: Verifies multiple systems display
- **TestShowMetrics_SingleSystem**: Verifies single system doesn't show individual section
- **TestShowError**: Verifies error message formatting
- **TestShowInfo**: Verifies info message formatting
- **TestGetDateRange**: Tests date range calculation for past dates
- **TestGetDateRange_Today**: Tests date range for current day
- **TestGetDateRange_ZeroQueryDate**: Tests date range with zero value (today)

### aggregator_test.go
Tests data aggregation with mock cloud clients:
- **TestNewDataAggregator**: Verifies default constructor
- **TestNewDataAggregatorWithFactory**: Verifies factory constructor for testing
- **TestGetAggregatedMetrics_SingleSystem**: Tests single system aggregation
- **TestGetAggregatedMetrics_MultipleSystems**: Tests multi-system aggregation and totals
- **TestGetAggregatedMetrics_MissingAPIConfig**: Verifies error for nil API config
- **TestGetAggregatedMetrics_MissingAPIKey**: Verifies error for missing API key
- **TestGetAggregatedMetrics_TokenError**: Verifies error handling for token failures
- **TestGetAggregatedMetrics_ContextCancellation**: Verifies context cancellation handling

## Test Patterns

### Table-Driven Tests
All tests follow Go's table-driven pattern:

```go
func TestExample(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"case 1", "input1", "output1"},
        {"case 2", "input2", "output2"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := FunctionUnderTest(tt.input)
            if result != tt.expected {
                t.Errorf("got %v, want %v", result, tt.expected)
            }
        })
    }
}
```

### Subtests
Each test uses `t.Run()` for named subtests, providing:
- Clear test failure messages
- Ability to run specific subtests
- Better test organization

## Coverage

Current test coverage: **9.5%** of statements

Coverage by component:
- **Config**: Color conversion, validation logic
- **Response Parsing**: JSON unmarshaling, interval summation
- **Timezone**: Date parsing, boundary calculations
- **URL Building**: Telemetry endpoint construction
- **Validation**: Tolerance calculations
- **Constants**: Helper functions

### Viewing Coverage

#### Terminal View
```bash
go tool cover -func=coverage.out
```

#### HTML View
```bash
go tool cover -html=coverage.out
```

#### Coverage by Package
```bash
go test -cover ./...
```

## Integration Tests

The project also includes integration tests via `run-tests.sh`:
- Uses cached API responses (test mode)
- Validates against expected values
- Tests real data flow through the application
- See README.md "Testing" section for details

To run integration tests:
```bash
./run-tests.sh
```

## Best Practices

### Running Tests After Changes
Always run tests after making changes:
```bash
make test
```

### Adding New Tests
1. Create `*_test.go` file in the same directory as code under test
2. Follow table-driven test pattern
3. Use descriptive test case names
4. Test both success and error cases
5. Run tests to verify: `go test -v ./...`

### Test Coverage Goals
- Aim for >70% coverage on business logic
- Focus on critical paths (config, parsing, calculations)
- Don't test trivial getters/setters
- Test error handling paths

### Test Organization
- Group related test cases in subtests
- Use clear, descriptive names
- Keep tests independent (no shared state)
- Clean up resources in tests

## Troubleshooting

### Tests Fail to Compile
```bash
# Clear test cache
go clean -testcache
# Rebuild
go build ./...
```

### Flaky Tests
Use `-count=1` to disable test caching:
```bash
go test -v -count=1 ./...
```

### Specific Test Fails
Run only that test:
```bash
go test -v -run TestName ./...
```

## CI/CD Integration

Tests can be integrated into CI/CD pipelines:

### GitHub Actions Example
```yaml
- name: Run tests
  run: go test -v ./...

- name: Check coverage
  run: |
    go test -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out
```

### Pre-commit Hook
```bash
#!/bin/bash
# .git/hooks/pre-commit
make test || exit 1
```

## Recent Additions

The following tests were recently added to improve coverage:
- **Aggregator tests**: Uses mock CloudClient via CloudClientFactory dependency injection
- **Display tests**: Uses io.Writer injection to capture and verify output
- **Cache tests**: Tests thread-safe state management with sync.Mutex

## Future Enhancements

Potential areas for additional testing:
- API client tests (with mock HTTP responses)
- OAuth token management tests
- Cache file I/O tests (read/write operations)
- Error propagation tests across package boundaries
- End-to-end integration tests

## Related Documentation

- **[README.md](README.md)** - User guide and project overview
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System architecture and design
- **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** - Go patterns used in codebase
