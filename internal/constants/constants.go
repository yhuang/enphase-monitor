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
//   - HTTP client configuration (APIRequestTimeout, API Budget)
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

// QueryMode represents the Query Mode for a date query (day, month, year, or true-up).
type QueryMode int

const (
	// QueryModeDay represents a query for a specific day (YYYY-MM-DD)
	QueryModeDay QueryMode = iota
	// QueryModeMonth represents a query for a specific month (YYYY-MM)
	QueryModeMonth
	// QueryModeYear represents a query for a specific year (YYYY)
	QueryModeYear
	// QueryModeTrueUp represents a single-batch query spanning a full True-Up Period.
	// The start date is the first day of the utility True-Up Start Date's month; data
	// covers all complete days through yesterday using Lifetime Data endpoints.
	QueryModeTrueUp
)

// String returns a human-readable name for the query mode.
func (qt QueryMode) String() string {
	switch qt {
	case QueryModeMonth:
		return "month"
	case QueryModeYear:
		return "year"
	case QueryModeTrueUp:
		return "true-up"
	default:
		return "day"
	}
}

// HTTP client configuration
const (
	// APIRequestTimeout is the timeout for API HTTP requests
	APIRequestTimeout = 30 * time.Second
	// APIBudgetPerMinute is the API Budget window size (10 requests/minute)
	APIBudgetPerMinute = 10
	// APIBudgetWindowSeconds is the sliding-window duration for the API Budget counter
	APIBudgetWindowSeconds = 60
	// APIMaxDateRangeDays is the maximum date range per API request (7 days)
	// The Enphase API returns 422 errors for ranges exceeding this limit
	APIMaxDateRangeDays = 7
)

// Error Message Constants
// These constants ensure consistent error identification across the codebase.
const (
	// RateLimitError is the identifier for rate limit (429) errors.
	// Callers compare against this string via the IsRateLimitError helper below.
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
	// KWhToMWh is the conversion factor from kilowatt-hours to megawatt-hours
	KWhToMWh = 1000.0
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
	// ANSIColorCubeRedMultiplier is the stride for the red channel (levels^2 = 36)
	ANSIColorCubeRedMultiplier = ANSIColorCubeLevels * ANSIColorCubeLevels
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
