// Package constants - constants_test.go
//
// TEST SETUP
// ----------
// This test suite validates constant values and helper functions.
// Tests ensure constants maintain expected values throughout refactoring.
//
// TEST PLAN
// ---------
// 1. Helper Function Tests
//    - Test IsRateLimitError() correctly identifies 429 errors
//    - Test nil error returns false
//    - Test non-rate-limit errors return false
//    - Test error message substring matching
//
// 2. Constant Value Tests (Optional)
//    - Test ANSI codes are correct
//    - Test numeric constants (timeouts, tolerances)
//    - Test string constants (error messages, field names)
//
// TESTING APPROACH
// ----------------
// - Table-driven tests for multiple error scenarios
// - Simple value equality checks for constants
// - Focus on helper functions that use constants
//
// WHY TEST CONSTANTS
// ------------------
// Testing constants ensures:
// - Values don't change accidentally during refactoring
// - Helper functions work correctly
// - Centralized values are used consistently
// - Documentation stays in sync with values
//
// COVERAGE
// --------
// This package achieves 100% coverage because:
// - All helper functions are tested
// - Constants are referenced in tests
// - Simple, pure logic with no I/O
//
// PATTERN USED
// ------------
// - Pattern 1: Table-Driven Tests
// - Pattern 3: Subtests with t.Run()
//
// See TESTING.md for detailed pattern explanations.
package constants

import (
	"testing"
)

// testError is a simple error implementation for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// TestIsRateLimitError tests the isRateLimitError helper function
func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "rate limit error message",
			err:      &testError{msg: RateLimitError},
			expected: true,
		},
		{
			name:     "rate limit in message",
			err:      &testError{msg: "API failed: " + RateLimitError},
			expected: true,
		},
		{
			name:     "other error",
			err:      &testError{msg: "connection timeout"},
			expected: false,
		},
		{
			name:     "empty error",
			err:      &testError{msg: ""},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRateLimitError(tt.err)
			if result != tt.expected {
				t.Errorf("isRateLimitError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestConstants validates all constant values are correct
func TestConstants(t *testing.T) {
	// ANSI formatting constants
	if Reset != "\033[0m" {
		t.Errorf("Reset = %q, want %q", Reset, "\033[0m")
	}
	if Bold != "\033[1m" {
		t.Errorf("Bold = %q, want %q", Bold, "\033[1m")
	}

	// Display constants
	if SeparatorWidth != 57 {
		t.Errorf("SeparatorWidth = %d, want 57", SeparatorWidth)
	}

	// Date/time constants
	if DateFormat != "2006-01-02" {
		t.Errorf("DateFormat = %q, want %q", DateFormat, "2006-01-02")
	}

	// HTTP constants
	if APIRequestTimeout.String() != "30s" {
		t.Errorf("APIRequestTimeout = %v, want 30s", APIRequestTimeout)
	}
	if HTTPStatusOK != 200 {
		t.Errorf("HTTPStatusOK = %d, want 200", HTTPStatusOK)
	}
	if HTTPStatusUnauthorized != 401 {
		t.Errorf("HTTPStatusUnauthorized = %d, want 401", HTTPStatusUnauthorized)
	}
	if HTTPStatusTooManyRequests != 429 {
		t.Errorf("HTTPStatusTooManyRequests = %d, want 429", HTTPStatusTooManyRequests)
	}

	// Error messages
	if RateLimitError != "rate limit exceeded (429)" {
		t.Errorf("RateLimitError = %q, want %q", RateLimitError, "rate limit exceeded (429)")
	}

	// Energy conversion
	if WhToKWh != 1000.0 {
		t.Errorf("WhToKWh = %f, want 1000.0", WhToKWh)
	}

	// Validation constants
	if ValidationTolerancePercent != 0.10 {
		t.Errorf("ValidationTolerancePercent = %f, want 0.10", ValidationTolerancePercent)
	}
	if ValidationMinToleranceKWh != 0.1 {
		t.Errorf("ValidationMinToleranceKWh = %f, want 0.1", ValidationMinToleranceKWh)
	}

	// Color conversion constants
	if ANSIColorCubeBase != 16 {
		t.Errorf("ANSIColorCubeBase = %d, want 16", ANSIColorCubeBase)
	}
	if ANSIColorCubeLevels != 6 {
		t.Errorf("ANSIColorCubeLevels = %d, want 6", ANSIColorCubeLevels)
	}

	// Timezone constants
	if UTCTimezone != "UTC" {
		t.Errorf("UTCTimezone = %q, want %q", UTCTimezone, "UTC")
	}
	if FallbackTimezone != "US/Pacific" {
		t.Errorf("FallbackTimezone = %q, want %q", FallbackTimezone, "US/Pacific")
	}

	// API URL constants
	if EnphaseAPIBaseURL != "https://api.enphaseenergy.com" {
		t.Errorf("EnphaseAPIBaseURL = %q, want %q", EnphaseAPIBaseURL, "https://api.enphaseenergy.com")
	}
}
