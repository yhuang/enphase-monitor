// Package constants centralizes all application constants.
//
// PURPOSE
// -------
// This package centralizes all application constants to avoid magic numbers and strings.
// By consolidating constants here, we improve maintainability and make the codebase
// more readable and easier to modify.
//
// CONSTANT CATEGORIES
// -------------------
// This file organizes constants into logical groups:
//   - ANSI font style constants (Reset, Bold)
//   - Display formatting (SeparatorWidth)
//   - Date and time formats (DateFormat, TimestampFormat)
//   - HTTP client configuration (APIRequestTimeout, status codes, rate limits)
//   - Error messages (RateLimitError, API config errors)
//   - Energy conversion (WhToKWh)
//   - Telemetry field names (API JSON field constants)
//   - Battery formatting (BatterySOCPercentSuffix)
//   - Validation tolerances and thresholds
//   - Response parsing (ResponseBodyPreviewLength)
//   - Color conversion (ANSI color cube constants, hex parsing)
//   - Timezone (UTCTimezone, FallbackTimezone)
//   - API URLs (Enphase API base URLs and endpoints)
//
// USAGE GUIDELINES
// ----------------
// When adding new constants:
//  1. Group them logically with related constants
//  2. Add descriptive comments explaining purpose and usage
//  3. Use descriptive names (e.g., ValidationTolerancePercent, not just Tolerance)
//  4. Consider if the constant should be configurable (move to config.yaml if so)
package constants

import (
	"strings"
	"time"
)

// ANSI font style constants
const (
	// Reset resets all text formatting and colors
	Reset = "\033[0m"
	// Bold makes text bold
	Bold = "\033[1m"
)

// Display formatting constants
const (
	// SeparatorWidth is the width of separator lines in the display output
	SeparatorWidth = 57
)

// Date and time format constants
const (
	// DateFormat is the standard date format used throughout the application (YYYY-MM-DD)
	DateFormat = "2006-01-02"
	// MonthFormat is the format for month-level queries (YYYY-MM)
	MonthFormat = "2006-01"
	// YearFormat is the format for year-level queries (YYYY)
	YearFormat = "2006"
	// AltDateFormat is an alternative date format (YYYY/MM/DD)
	AltDateFormat = "2006/01/02"
	// TimestampFormat includes time and timezone
	TimestampFormat = "2006-01-02 15:04:05 MST"
	// JSONExtension is the file extension for JSON cache files
	JSONExtension = ".json"
)

// QueryType represents the granularity of a date query (day, month, or year).
type QueryType int

const (
	// QueryTypeDay represents a query for a specific day (YYYY-MM-DD)
	QueryTypeDay QueryType = iota
	// QueryTypeMonth represents a query for a specific month (YYYY-MM)
	QueryTypeMonth
	// QueryTypeYear represents a query for a specific year (YYYY)
	QueryTypeYear
)

// String returns a human-readable name for the query type.
func (qt QueryType) String() string {
	switch qt {
	case QueryTypeMonth:
		return "month"
	case QueryTypeYear:
		return "year"
	default:
		return "day"
	}
}

// HTTP client configuration
const (
	// APIRequestTimeout is the timeout for API HTTP requests
	APIRequestTimeout = 30 * time.Second
	// APIRateLimitPerMinute is the free tier rate limit (10 requests/minute)
	APIRateLimitPerMinute = 10
	// APIRateLimitWaitSeconds is the recommended wait time after hitting rate limit
	APIRateLimitWaitSeconds = 60
	// APIMaxDateRangeDays is the maximum date range per API request (7 days)
	// The Enphase API returns 422 errors for ranges exceeding this limit
	APIMaxDateRangeDays = 7
	// HTTPStatusOK is the standard HTTP success status code
	HTTPStatusOK = 200
	// HTTPStatusUnauthorized is the unauthorized HTTP status code
	HTTPStatusUnauthorized = 401
	// HTTPStatusTooManyRequests is the rate limit HTTP status code
	HTTPStatusTooManyRequests = 429
	// HTTPStatusUnprocessableEntity is returned when request parameters are invalid
	HTTPStatusUnprocessableEntity = 422
)

// Error Message Constants
// These constants ensure consistent error identification across the codebase.
const (
	// RateLimitError is the identifier for rate limit (429) errors.
	// Used in aggregator.go, main.go, and cloud_client.go for consistent error detection.
	RateLimitError = "rate limit exceeded (429)"
	// ErrAPIConfigRequired is returned when API configuration is missing
	ErrAPIConfigRequired = "api configuration required"
	// ErrTokenRefreshFailed is returned when OAuth token refresh fails
	ErrTokenRefreshFailed = "failed to get OAuth access token"
	// ErrInvalidSystemID is returned when system ID is missing
	ErrInvalidSystemID = "system must have id"
)

// Energy conversion constants
const (
	// WhToKWh is the conversion factor from watt-hours to kilowatt-hours
	WhToKWh = 1000.0
)

// Telemetry field name constants
// These are the JSON field names used in API responses
const (
	FieldWhImported = "wh_imported"
	FieldWhExported = "wh_exported"
	FieldWhDel      = "wh_del"
	FieldEnwh       = "enwh"
)

// Battery constants
const (
	// BatterySOCPercentSuffix is the suffix used in SOC strings (e.g., "97%")
	BatterySOCPercentSuffix = "%"
)

// Validation constants
const (
	// ValidationTolerancePercent is the acceptable deviation (10%)
	ValidationTolerancePercent = 0.10
	// ValidationMinToleranceKWh is the minimum tolerance for small values
	ValidationMinToleranceKWh = 0.1
	// ValidationPercentThreshold is the threshold below which percentage is shown as 0%
	ValidationPercentThreshold = 0.5
	// ValidationInfinitePercent is used for undefined percentage (when expected is 0)
	ValidationInfinitePercent = 999.9
)

// Response parsing constants
const (
	// ResponseBodyPreviewLength is the max length for body preview in error messages
	ResponseBodyPreviewLength = 200
)

// Color conversion constants
const (
	// ANSIColorCubeBase is the base offset for ANSI 256-color cube
	ANSIColorCubeBase = 16
	// ANSIColorCubeLevels is the number of levels per RGB channel (6x6x6 cube)
	ANSIColorCubeLevels = 6
	// HexBase is the base for hexadecimal parsing
	HexBase = 16
	// RGBMaxValue is the maximum RGB value (0-255)
	RGBMaxValue = 255
)

// Timezone constants
const (
	// UTCTimezone is the UTC timezone identifier
	UTCTimezone = "UTC"
	// FallbackTimezone is the default fallback timezone when system timezone is UTC
	FallbackTimezone = "US/Pacific"
)

// API URL constants
const (
	// EnphaseAPIBaseURL is the base URL for Enphase API v4
	EnphaseAPIBaseURL = "https://api.enphaseenergy.com"
	// EnphaseOAuthAuthorizeURL is the OAuth authorization endpoint
	EnphaseOAuthAuthorizeURL = EnphaseAPIBaseURL + "/oauth/authorize"
	// EnphaseOAuthTokenURL is the OAuth token endpoint
	EnphaseOAuthTokenURL = EnphaseAPIBaseURL + "/oauth/token"
	// EnphaseAPIv4SystemsURL is the base URL for systems API endpoints
	EnphaseAPIv4SystemsURL = EnphaseAPIBaseURL + "/api/v4/systems"
)

// IsRateLimitError checks if an error is a rate limit (429) error.
// Centralizes the repeated strings.Contains check pattern.
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), RateLimitError)
}
