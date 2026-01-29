// Package main - timezone.go
//
// PURPOSE
// -------
// Utilities for timezone handling and date boundary calculations.
// Ensures all date ranges use the configured timezone (not UTC).
//
// For timezone configuration details, see:
//   - config.go: LoadTimezone() usage and fallback logic
//   - config.yaml.example: timezone configuration examples
//   - README.md: "Optional Settings" section
//
// WHY TIMEZONE MATTERS
// --------------------
// Date boundaries (00:00:00 to 23:59:59) must be calculated in the reporting
// timezone to match how Enphase calculates daily totals. Using UTC would
// result in incorrect date ranges and mismatched totals.

package main

import (
	"time"
)

// LoadTimezone loads a timezone location from a timezone string (e.g., "America/Los_Angeles").
// If the timezone string is empty, uses the system's local timezone.
// If the timezone string is invalid, falls back to system timezone, then US/Pacific as last resort.
func LoadTimezone(timezoneStr string) (*time.Location, error) {
	if timezoneStr == "" {
		// Use system timezone if not specified
		return time.Now().Location(), nil
	}

	tz, err := time.LoadLocation(timezoneStr)
	if err != nil {
		// If invalid, fall back to system timezone
		systemTZ := time.Now().Location()
		// If system timezone is UTC, use FallbackTimezone as last resort
		if systemTZ.String() == UTCTimezone {
			if fallbackTZ, err := time.LoadLocation(FallbackTimezone); err == nil {
				return fallbackTZ, nil
			}
		}
		return systemTZ, nil
	}
	return tz, nil
}

// getDayBoundaries returns the start and end times for a given date in the specified timezone.
// The end time is capped to the current time if the date is today to prevent 422 errors.
func getDayBoundaries(targetDate time.Time, tz *time.Location) (dayStart, dayEnd time.Time) {
	var date time.Time
	if !targetDate.IsZero() {
		date = targetDate.In(tz)
	} else {
		date = time.Now().In(tz)
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

// isPastDate checks if the given date is before today in the specified timezone.
func isPastDate(targetDate time.Time, tz *time.Location) bool {
	if targetDate.IsZero() {
		return false
	}

	now := time.Now().In(tz)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)
	targetDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, tz)

	return targetDay.Before(today)
}

// ParseDateInTimezone parses a date string in YYYY-MM-DD format in the specified timezone
func ParseDateInTimezone(dateStr string, tz *time.Location) (time.Time, error) {
	parsed, err := time.ParseInLocation(DateFormat, dateStr, tz)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}
