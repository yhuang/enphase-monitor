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
