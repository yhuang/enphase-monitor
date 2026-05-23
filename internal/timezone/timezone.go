// Package timezone provides utilities for timezone handling and date boundary calculations.
//
// PURPOSE
// -------
// Utilities for timezone handling and date boundary calculations.
// Ensures all date ranges use the configured timezone (not UTC).
//
// For timezone configuration details, see:
//   - config package: LoadTimezone() usage and fallback logic
//   - config.yaml.example: timezone configuration examples
//   - README.md: "Optional Settings" section
//
// WHY TIMEZONE MATTERS
// --------------------
// Date boundaries (00:00:00 to 23:59:59) must be calculated in the reporting
// timezone to match how Enphase calculates daily totals. Using UTC would
// result in incorrect date ranges and mismatched totals.

package timezone

import (
	"fmt"
	"time"

	"enphase-monitor/internal/constants"
)

// ParseResult contains the parsed date and its query mode.
type ParseResult struct {
	Date      time.Time
	QueryMode constants.QueryMode
}

// LoadTimezone loads a timezone location from a timezone string (e.g., "America/Los_Angeles").
// If the timezone string is empty, uses the system's local timezone.
// If the timezone string is invalid, falls back to system timezone, then US/Pacific as last resort.
func LoadTimezone(timezoneStr string) (*time.Location, error) {
	if timezoneStr == "" {
		// Use system timezone if not specified
		return time.Now().Location(), nil
	}

	tz, err := time.LoadLocation(timezoneStr)
	if err == nil {
		return tz, nil
	}
	// If invalid, fall back to system timezone (go-style-core: max 2 levels)
	systemTZ := time.Now().Location()
	if systemTZ.String() != constants.UTCTimezone {
		return systemTZ, nil
	}
	fallbackTZ, err := time.LoadLocation(constants.FallbackTimezone)
	if err != nil {
		return systemTZ, nil
	}
	return fallbackTZ, nil
}

// GetDayBoundaries returns the start and end times for a given date in the specified timezone.
// The end time is capped to the current time if the date is today to prevent 422 errors.
func GetDayBoundaries(targetDate time.Time, tz *time.Location) (dayStart, dayEnd time.Time) {
	date := time.Now().In(tz)
	if !targetDate.IsZero() {
		date = targetDate.In(tz)
	}

	// Calculate day boundaries in the specified timezone
	dayStart = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, tz)
	dayEnd = time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, tz)

	// Always ensure end_at is not in the future (prevents 422 errors)
	// This handles clock skew and timezone differences
	now := time.Now().In(tz)
	if dayEnd.After(now) {
		dayEnd = now
	}

	return dayStart, dayEnd
}

// IsPastDate checks if the given date is before today in the specified timezone.
func IsPastDate(targetDate time.Time, tz *time.Location) bool {
	if targetDate.IsZero() {
		return false
	}

	now := time.Now().In(tz)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)
	targetDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, tz)

	return targetDay.Before(today)
}

// ParseDateInTimezone parses a date string in YYYY-MM-DD format in the specified timezone.
func ParseDateInTimezone(dateStr string, tz *time.Location) (time.Time, error) {
	parsed, err := time.ParseInLocation(constants.DateFormat, dateStr, tz)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

// ParseDateString parses a date string and determines its format (day/month/year).
// Supported formats:
//   - YYYY-MM-DD (day)
//   - YYYY-MM (month)
//   - YYYY (year)
//
// Returns the parsed date and query mode, or an error if format is invalid.
func ParseDateString(dateStr string, tz *time.Location) (ParseResult, error) {
	// Try YYYY-MM-DD first (most specific)
	if parsed, err := time.ParseInLocation(constants.DateFormat, dateStr, tz); err == nil {
		// Validate the date is real (e.g., not 2026-02-30)
		if parsed.Format(constants.DateFormat) != dateStr {
			return ParseResult{}, fmt.Errorf("invalid date: %s does not exist", dateStr)
		}
		return ParseResult{Date: parsed, QueryMode: constants.QueryModeDay}, nil
	}

	// Try YYYY-MM (month)
	if parsed, err := time.ParseInLocation(constants.MonthFormat, dateStr, tz); err == nil {
		if parsed.Format(constants.MonthFormat) != dateStr {
			return ParseResult{}, fmt.Errorf("invalid month: %s", dateStr)
		}
		return ParseResult{Date: parsed, QueryMode: constants.QueryModeMonth}, nil
	}

	// Try YYYY (year)
	if parsed, err := time.ParseInLocation(constants.YearFormat, dateStr, tz); err == nil {
		year := parsed.Year()
		if year < 1900 || year > 2100 {
			return ParseResult{}, fmt.Errorf("invalid year: %s (must be between 1900-2100)", dateStr)
		}
		return ParseResult{Date: parsed, QueryMode: constants.QueryModeYear}, nil
	}

	return ParseResult{}, fmt.Errorf("invalid date format: use YYYY-MM-DD, YYYY-MM, or YYYY")
}

// GetMonthBoundaries returns the start and end times for a given month.
// Start is 00:00:00 on the 1st day, end is 23:59:59 on the last day.
// End time is capped to now if the month includes today.
func GetMonthBoundaries(targetDate time.Time, tz *time.Location) (start, end time.Time) {
	date := time.Now().In(tz)
	if !targetDate.IsZero() {
		date = targetDate.In(tz)
	}

	// Start of month: 1st day at 00:00:00
	start = time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, tz)

	// End of month: last day at 23:59:59
	// Go to next month, then back one day
	nextMonth := start.AddDate(0, 1, 0)
	lastDay := nextMonth.Add(-24 * time.Hour)
	end = time.Date(lastDay.Year(), lastDay.Month(), lastDay.Day(), 23, 59, 59, 0, tz)

	// Cap to now if end is in the future
	now := time.Now().In(tz)
	if end.After(now) {
		end = now
	}

	return start, end
}

// GetYearBoundaries returns the start and end times for a given year.
// Start is 00:00:00 on Jan 1, end is 23:59:59 on Dec 31.
// End time is capped to now if the year includes today.
func GetYearBoundaries(targetDate time.Time, tz *time.Location) (start, end time.Time) {
	date := time.Now().In(tz)
	if !targetDate.IsZero() {
		date = targetDate.In(tz)
	}

	// Start of year: Jan 1 at 00:00:00
	start = time.Date(date.Year(), time.January, 1, 0, 0, 0, 0, tz)

	// End of year: Dec 31 at 23:59:59
	end = time.Date(date.Year(), time.December, 31, 23, 59, 59, 0, tz)

	// Cap to now if end is in the future
	now := time.Now().In(tz)
	if end.After(now) {
		end = now
	}

	return start, end
}

// GetTrueUpBoundaries returns the start and end times for a true-up year query.
// Start is midnight on trueUpStartDate (callers normalize this to the first of the month).
// End is 23:59:59 of yesterday (the most recent complete day).
func GetTrueUpBoundaries(trueUpStartDate time.Time, tz *time.Location) (start, end time.Time) {
	d := trueUpStartDate.In(tz)
	start = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, tz)
	yesterday := time.Now().In(tz).AddDate(0, 0, -1)
	end = time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 0, tz)
	return start, end
}

// GetBoundaries returns the start and end times based on query mode.
// This is a unified boundary function that delegates to the appropriate handler.
func GetBoundaries(targetDate time.Time, queryMode constants.QueryMode, tz *time.Location) (start, end time.Time) {
	switch queryMode {
	case constants.QueryModeMonth:
		return GetMonthBoundaries(targetDate, tz)
	case constants.QueryModeYear:
		return GetYearBoundaries(targetDate, tz)
	case constants.QueryModeTrueUp:
		return GetTrueUpBoundaries(targetDate, tz)
	default:
		return GetDayBoundaries(targetDate, tz)
	}
}

// IsPastPeriod checks if the given date's period (day/month/year) is in the past.
func IsPastPeriod(targetDate time.Time, queryMode constants.QueryMode, tz *time.Location) bool {
	if targetDate.IsZero() {
		return false
	}

	now := time.Now().In(tz)
	target := targetDate.In(tz)

	switch queryMode {
	case constants.QueryModeYear:
		return target.Year() < now.Year()
	case constants.QueryModeMonth:
		if target.Year() < now.Year() {
			return true
		}
		return target.Year() == now.Year() && target.Month() < now.Month()
	case constants.QueryModeTrueUp:
		// The true-up period is always treated as ongoing so Lifetime Data endpoints
		// are re-fetched with fresh data each run (through yesterday).
		return false
	default:
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)
		targetDay := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, tz)
		return targetDay.Before(today)
	}
}
