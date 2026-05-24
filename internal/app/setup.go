// setup.go contains application setup and initialization for the app package.
// Package comment and execution modes are in runner.go.
//
// SETUP FUNCTIONS
// ---------------
// These functions extract common initialization logic from main() to improve readability:
//   - CreateOAuthAdapter: Creates OAuth token adapter for aggregator
//   - SetupDisplay: Configures display with colors from config
//   - ConfigureModes: Sets up Validation Mode and cache mode flags
//   - ParseTestDate: Parses and validates test date parameter
package app

import (
	"context"
	"fmt"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/display"
	"enphase-monitor/internal/oauth"
	"enphase-monitor/internal/timezone"
)

// ParseDateInput represents the result of parsing a date input string.
type ParseDateInput struct {
	Date      time.Time
	QueryMode constants.QueryMode
}

// CreateOAuthAdapter creates an adapter function for OAuth token retrieval.
// Since config.APIConfig and aggregator.APIConfig are now type aliases to
// the same underlying types.APIConfig, no conversion is needed.
func CreateOAuthAdapter() aggregator.OAuthTokenGetter {
	return func(ctx context.Context, apiConfig *aggregator.APIConfig) (string, error) {
		// No conversion needed - both are aliases to types.APIConfig
		return oauth.GetAccessToken(ctx, apiConfig)
	}
}

// SetupDisplay creates a Display instance with colors from config.
func SetupDisplay(cfg *config.Config, reportTZ *time.Location) *display.Display {
	colors := display.GetDefaultColors()
	if cfg.Colors != nil {
		// Copy config color fields into the display color struct
		colors.Production = cfg.Colors.Production
		colors.Discharge = cfg.Colors.Discharge
		colors.Import = cfg.Colors.Import
		colors.Export = cfg.Colors.Export
		colors.NetImport = cfg.Colors.NetImport
		colors.NetExport = cfg.Colors.NetExport
		colors.NetImportBackground = cfg.Colors.NetImportBackground
		colors.NetExportBackground = cfg.Colors.NetExportBackground
		colors.Headers = cfg.Colors.Headers
		colors.Charge = cfg.Colors.Charge
		colors.TotalConsumed = cfg.Colors.TotalConsumed
		colors.SecondaryText = cfg.Colors.SecondaryText
		colors.PrimaryText = cfg.Colors.PrimaryText
		colors.Error = cfg.Colors.Error
	}
	return display.NewDisplayWithColorsAndTimezone(colors, reportTZ)
}

// ConfigureModes sets up Validation Mode, cache mode, and debug mode based on flags.
func ConfigureModes(validationMode, noCache, debug bool) {
	if validationMode {
		cache.SetValidationMode(true)
		fmt.Println("VALIDATION MODE: Using cache only, no live API calls")
	}

	if noCache {
		cache.SetCacheDisabled(true)
		fmt.Println("LIVE MODE: Bypassing cache, making live API calls")
	}

	if debug {
		cache.SetDebugMode(true)
	}
}

// ParseTestDate parses the test date string and returns the date and query mode.
// Supports YYYY-MM-DD (day), YYYY-MM (month), and YYYY (year) formats.
// Returns a zero time and QueryModeDay if the input is empty (meaning: query today in Day Mode).
func ParseTestDate(dateStr string, reportTZ *time.Location) (ParseDateInput, error) {
	if dateStr == "" {
		return ParseDateInput{
			Date:      time.Time{},
			QueryMode: constants.QueryModeDay,
		}, nil
	}

	// Use the unified parser that detects format
	result, err := timezone.ParseDateString(dateStr, reportTZ)
	if err != nil {
		return ParseDateInput{}, fmt.Errorf("invalid date format: use YYYY-MM-DD, YYYY-MM, or YYYY (e.g., 2026-01-19, 2026-01, 2026): %w", err)
	}

	return ParseDateInput{
		Date:      result.Date,
		QueryMode: result.QueryMode,
	}, nil
}

// FormatDateForQueryMode formats a date according to its query mode.
func FormatDateForQueryMode(date time.Time, queryMode constants.QueryMode) string {
	switch queryMode {
	case constants.QueryModeYear:
		return date.Format(constants.YearFormat)
	case constants.QueryModeMonth:
		return date.Format(constants.MonthFormat)
	default:
		return date.Format(constants.DateFormat)
	}
}

// GetAggregatorTypes extracts systems and API config from the main config.
// Returns a copy of the systems slice so callers cannot mutate the config.
// Since config.SystemConfig and config.APIConfig are type aliases to the same
// underlying types, no conversion is needed for the API config.
func GetAggregatorTypes(cfg *config.Config) ([]aggregator.SystemConfig, *aggregator.APIConfig) {
	if len(cfg.Systems) == 0 {
		return nil, cfg.API
	}
	systems := make([]aggregator.SystemConfig, len(cfg.Systems))
	copy(systems, cfg.Systems)
	return systems, cfg.API
}

// ValidateValidationModeCache checks if cache exists for the target date when in Validation Mode.
// Returns an error with a helpful message if no cache exists for the date.
// This prevents confusing errors when running --test without populated cache.
func ValidateValidationModeCache(targetDate time.Time, reportTZ *time.Location) error {
	// Determine the date string to check: default to specified date, override for today if none
	dateStr := targetDate.Format(constants.DateFormat)
	if targetDate.IsZero() {
		dateStr = time.Now().In(reportTZ).Format(constants.DateFormat)
	}

	// Check if cache exists for this date
	hasCache, err := cache.HasCacheForDate(dateStr)
	if err != nil {
		return fmt.Errorf("failed to check cache for %s: %w\n\n"+
			"To populate the cache, run:\n"+
			"  ./enphase-monitor\n\n"+
			"Then retry with --test", dateStr, err)
	}

	if !hasCache {
		return fmt.Errorf("--test flag requires cached data, but no cache exists for %s\n\n"+
			"To populate the cache, run:\n"+
			"  ./enphase-monitor\n\n"+
			"Then retry with --test\n\n"+
			"For past dates with expected values, use:\n"+
			"  ./enphase-monitor --date %s\n"+
			"  ./enphase-monitor --test --date %s", dateStr, dateStr, dateStr)
	}

	return nil
}
