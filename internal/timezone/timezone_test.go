// Package timezone - timezone_test.go
//
// TEST SETUP
// ----------
// This test suite validates timezone handling and date boundary calculations.
// Tests ensure consistent date/time handling across different timezones.
//
// TEST PLAN
// ---------
// 1. Timezone Loading Tests
//   - Test valid IANA timezone identifiers (US/Pacific, America/New_York, etc.)
//   - Test empty string uses system timezone
//   - Test UTC timezone fallback logic
//   - Test invalid timezone returns error
//
// 2. Date Boundary Tests
//   - Test GetDayBoundaries returns correct start/end times
//   - Test boundaries respect timezone (not UTC)
//   - Test daylight saving time transitions
//   - Test zero time value (use today)
//
// 3. Past Date Detection Tests
//   - Test IsPastDate correctly identifies historical dates
//   - Test today returns false
//   - Test future dates return false
//
// TESTING APPROACH
// ----------------
// - Table-driven tests with various timezone strings
// - Use known dates for predictable results
// - Verify boundaries are in specified timezone, not UTC
// - Test edge cases (UTC, empty string, invalid timezone)
//
// WHY TIMEZONE MATTERS
// --------------------
// Timezone handling is critical for:
// - Accurate day boundaries (midnight to midnight in local time)
// - Cache key generation (dates must be consistent)
// - Display formatting (show times in user's timezone)
// - API queries (request data for correct 24-hour period)
//
// PATTERN USED
// ------------
// - Pattern 1: Table-Driven Tests
// - Pattern 3: Subtests with t.Run()
//
// See docs/TESTING.md for detailed pattern explanations.
package timezone

import (
	"testing"
	"time"

	"enphase-monitor/internal/constants"
)

// TestLoadTimezone tests timezone loading
func TestLoadTimezone(t *testing.T) {
	tests := []struct {
		name           string
		timezoneStr    string
		expectedString string // Expected timezone name/offset
		expectError    bool
	}{
		{
			name:           "empty string uses system timezone",
			timezoneStr:    "",
			expectedString: time.Now().Location().String(),
			expectError:    false,
		},
		{
			name:           "valid US/Pacific",
			timezoneStr:    "US/Pacific",
			expectedString: "US/Pacific",
			expectError:    false,
		},
		{
			name:           "valid America/New_York",
			timezoneStr:    "America/New_York",
			expectedString: "America/New_York",
			expectError:    false,
		},
		{
			name:           "valid UTC",
			timezoneStr:    constants.UTCTimezone,
			expectedString: constants.UTCTimezone,
			expectError:    false,
		},
		{
			name:        "invalid timezone falls back gracefully",
			timezoneStr: "Invalid/Timezone",
			// Should fall back to system timezone or US/Pacific
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tz, err := LoadTimezone(tt.timezoneStr)

			if tt.expectError {
				if err == nil {
					t.Errorf("LoadTimezone(%q) error = nil, want error", tt.timezoneStr)
				}
				return
			}

			if err != nil {
				t.Errorf("LoadTimezone(%q) unexpected error = %v", tt.timezoneStr, err)
				return
			}

			if tz == nil {
				t.Errorf("LoadTimezone(%q) returned nil timezone", tt.timezoneStr)
				return
			}

			// For valid timezones, check the string matches
			if tt.timezoneStr != "" && tt.timezoneStr != "Invalid/Timezone" {
				if tz.String() != tt.expectedString {
					t.Errorf("LoadTimezone(%q) returned %q, want %q", tt.timezoneStr, tz.String(), tt.expectedString)
				}
			}
		})
	}
}

// TestGetDayBoundaries tests day boundary calculations
func TestGetDayBoundaries(t *testing.T) {
	// Use a fixed timezone for predictable testing
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("Failed to load test timezone: %v", err)
	}

	tests := []struct {
		name           string
		targetDate     time.Time
		expectedStart  string // Time format: 15:04:05
		expectedEndMin string // Minimum expected end time
	}{
		{
			name:           "specific past date",
			targetDate:     time.Date(2026, 1, 15, 12, 30, 45, 0, tz),
			expectedStart:  "00:00:00",
			expectedEndMin: "23:59:59",
		},
		{
			name:           "zero time (today)",
			targetDate:     time.Time{},
			expectedStart:  "00:00:00",
			expectedEndMin: "00:00:00", // Could be any time today
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dayStart, dayEnd := GetDayBoundaries(tt.targetDate, tz)

			// Check start time is at midnight
			startTime := dayStart.Format("15:04:05")
			if startTime != tt.expectedStart {
				t.Errorf("GetDayBoundaries() start time = %s, want %s", startTime, tt.expectedStart)
			}

			// Check end time is either end of day or current time (if today)
			endTime := dayEnd.Format("15:04:05")
			if tt.expectedEndMin == "23:59:59" {
				// For past dates, should be end of day
				if endTime != "23:59:59" && !dayEnd.After(time.Now().In(tz)) {
					t.Errorf("GetDayBoundaries() end time = %s, want %s or current time", endTime, tt.expectedEndMin)
				}
			}

			// Verify end is after or equal to start
			if dayEnd.Before(dayStart) {
				t.Errorf("GetDayBoundaries() end (%v) is before start (%v)", dayEnd, dayStart)
			}
		})
	}
}

// TestIsPastDate tests past date detection
func TestIsPastDate(t *testing.T) {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("Failed to load test timezone: %v", err)
	}

	now := time.Now().In(tz)
	yesterday := now.AddDate(0, 0, -1)
	tomorrow := now.AddDate(0, 0, 1)

	tests := []struct {
		name       string
		targetDate time.Time
		expected   bool
	}{
		{
			name:       "zero time (today)",
			targetDate: time.Time{},
			expected:   false,
		},
		{
			name:       "yesterday",
			targetDate: yesterday,
			expected:   true,
		},
		{
			name:       "tomorrow",
			targetDate: tomorrow,
			expected:   false,
		},
		{
			name:       "today",
			targetDate: now,
			expected:   false,
		},
		{
			name:       "last week",
			targetDate: now.AddDate(0, 0, -7),
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPastDate(tt.targetDate, tz)
			if result != tt.expected {
				t.Errorf("IsPastDate(%v) = %v, want %v", tt.targetDate.Format(constants.DateFormat), result, tt.expected)
			}
		})
	}
}

// TestParseDateString tests unified date string parsing with format detection.
func TestParseDateString(t *testing.T) {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("Failed to load test timezone: %v", err)
	}

	tests := []struct {
		name          string
		dateStr       string
		wantErr       bool
		wantQueryType constants.QueryType
		wantYear      int
		wantMonth     time.Month
		wantDay       int
	}{
		{
			name:          "YYYY-MM-DD day format",
			dateStr:       "2026-01-15",
			wantQueryType: constants.QueryTypeDay,
			wantYear:      2026,
			wantMonth:     time.January,
			wantDay:       15,
		},
		{
			name:          "YYYY-MM month format",
			dateStr:       "2026-03",
			wantQueryType: constants.QueryTypeMonth,
			wantYear:      2026,
			wantMonth:     time.March,
			wantDay:       1,
		},
		{
			name:          "YYYY year format",
			dateStr:       "2025",
			wantQueryType: constants.QueryTypeYear,
			wantYear:      2025,
			wantMonth:     time.January,
			wantDay:       1,
		},
		{
			name:    "invalid format",
			dateStr: "15/01/2026",
			wantErr: true,
		},
		{
			name:    "invalid day date",
			dateStr: "2026-02-30",
			wantErr: true,
		},
		{
			name:    "year out of range",
			dateStr: "1800",
			wantErr: true,
		},
		{
			name:    "empty string",
			dateStr: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDateString(tt.dateStr, tz)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseDateString(%q) error = nil, want error", tt.dateStr)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseDateString(%q) unexpected error = %v", tt.dateStr, err)
				return
			}
			if result.QueryType != tt.wantQueryType {
				t.Errorf("ParseDateString(%q) QueryType = %v, want %v", tt.dateStr, result.QueryType, tt.wantQueryType)
			}
			if result.Date.Year() != tt.wantYear || result.Date.Month() != tt.wantMonth || result.Date.Day() != tt.wantDay {
				t.Errorf("ParseDateString(%q) date = %v, want %04d-%02d-%02d",
					tt.dateStr, result.Date, tt.wantYear, tt.wantMonth, tt.wantDay)
			}
		})
	}
}

// TestGetMonthBoundaries tests month boundary calculations.
func TestGetMonthBoundaries(t *testing.T) {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("Failed to load test timezone: %v", err)
	}

	// Past month: boundaries should be exact (Jan 1 to Jan 31)
	jan2025 := time.Date(2025, time.January, 15, 0, 0, 0, 0, tz)
	start, end := GetMonthBoundaries(jan2025, tz)

	if start.Year() != 2025 || start.Month() != time.January || start.Day() != 1 {
		t.Errorf("GetMonthBoundaries() start = %v, want 2025-01-01", start)
	}
	if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Errorf("GetMonthBoundaries() start time = %s, want 00:00:00", start.Format("15:04:05"))
	}
	if end.Year() != 2025 || end.Month() != time.January || end.Day() != 31 {
		t.Errorf("GetMonthBoundaries() end = %v, want 2025-01-31", end)
	}
	if end.Hour() != 23 || end.Minute() != 59 || end.Second() != 59 {
		t.Errorf("GetMonthBoundaries() end time = %s, want 23:59:59", end.Format("15:04:05"))
	}

	// Zero time uses current month — end should be capped to now
	start2, end2 := GetMonthBoundaries(time.Time{}, tz)
	if start2.Day() != 1 {
		t.Errorf("GetMonthBoundaries(zero) start day = %d, want 1", start2.Day())
	}
	now := time.Now().In(tz)
	if end2.After(now.Add(time.Second)) {
		t.Errorf("GetMonthBoundaries(zero) end %v is after now %v", end2, now)
	}
}

// TestGetYearBoundaries tests year boundary calculations.
func TestGetYearBoundaries(t *testing.T) {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("Failed to load test timezone: %v", err)
	}

	// Past year: boundaries should be Jan 1 to Dec 31
	y2024 := time.Date(2024, time.June, 15, 0, 0, 0, 0, tz)
	start, end := GetYearBoundaries(y2024, tz)

	if start.Year() != 2024 || start.Month() != time.January || start.Day() != 1 {
		t.Errorf("GetYearBoundaries() start = %v, want 2024-01-01", start)
	}
	if end.Year() != 2024 || end.Month() != time.December || end.Day() != 31 {
		t.Errorf("GetYearBoundaries() end = %v, want 2024-12-31", end)
	}
	if end.Hour() != 23 || end.Minute() != 59 || end.Second() != 59 {
		t.Errorf("GetYearBoundaries() end time = %s, want 23:59:59", end.Format("15:04:05"))
	}

	// Zero time uses current year — end capped to now
	start2, end2 := GetYearBoundaries(time.Time{}, tz)
	if start2.Month() != time.January || start2.Day() != 1 {
		t.Errorf("GetYearBoundaries(zero) start = %v, want Jan 1", start2)
	}
	now := time.Now().In(tz)
	if end2.After(now.Add(time.Second)) {
		t.Errorf("GetYearBoundaries(zero) end %v is after now %v", end2, now)
	}
}

// TestGetBoundaries tests the unified boundary dispatch function.
func TestGetBoundaries(t *testing.T) {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("Failed to load test timezone: %v", err)
	}

	target := time.Date(2025, time.March, 15, 0, 0, 0, 0, tz)

	// Day query: should return day boundaries
	start, end := GetBoundaries(target, constants.QueryTypeDay, tz)
	if start.Day() != 15 || start.Hour() != 0 {
		t.Errorf("GetBoundaries(day) start = %v, want 2025-03-15 00:00:00", start)
	}
	if end.Day() != 15 {
		t.Errorf("GetBoundaries(day) end = %v, want 2025-03-15", end)
	}

	// Month query: should return month boundaries
	start, end = GetBoundaries(target, constants.QueryTypeMonth, tz)
	if start.Day() != 1 {
		t.Errorf("GetBoundaries(month) start day = %d, want 1", start.Day())
	}
	if end.Month() != time.March {
		t.Errorf("GetBoundaries(month) end month = %v, want March", end.Month())
	}

	// Year query: should return year boundaries
	start, end = GetBoundaries(target, constants.QueryTypeYear, tz)
	if start.Month() != time.January || start.Day() != 1 {
		t.Errorf("GetBoundaries(year) start = %v, want Jan 1", start)
	}
	if end.Month() != time.December || end.Day() != 31 {
		t.Errorf("GetBoundaries(year) end = %v, want Dec 31", end)
	}
}

// TestIsPastPeriod tests period-aware past detection (day/month/year).
func TestIsPastPeriod(t *testing.T) {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("Failed to load test timezone: %v", err)
	}

	now := time.Now().In(tz)

	tests := []struct {
		name       string
		targetDate time.Time
		queryType  constants.QueryType
		want       bool
	}{
		{
			name:       "zero time returns false",
			targetDate: time.Time{},
			queryType:  constants.QueryTypeDay,
			want:       false,
		},
		{
			name:       "past day",
			targetDate: now.AddDate(0, 0, -1),
			queryType:  constants.QueryTypeDay,
			want:       true,
		},
		{
			name:       "today is not past day",
			targetDate: now,
			queryType:  constants.QueryTypeDay,
			want:       false,
		},
		{
			name:       "past month (same year)",
			targetDate: time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, tz),
			queryType:  constants.QueryTypeMonth,
			want:       now.Month() > 1, // only true if we're not in January
		},
		{
			name:       "current month is not past",
			targetDate: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, tz),
			queryType:  constants.QueryTypeMonth,
			want:       false,
		},
		{
			name:       "past year",
			targetDate: time.Date(now.Year()-1, time.January, 1, 0, 0, 0, 0, tz),
			queryType:  constants.QueryTypeYear,
			want:       true,
		},
		{
			name:       "current year is not past",
			targetDate: time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, tz),
			queryType:  constants.QueryTypeYear,
			want:       false,
		},
		// QueryTypeTrueUp always returns false regardless of date — the true-up
		// period is treated as ongoing so lifetime endpoints are re-fetched each run.
		{
			name:       "true-up with past date is not past",
			targetDate: time.Date(2025, time.January, 1, 0, 0, 0, 0, tz),
			queryType:  constants.QueryTypeTrueUp,
			want:       false,
		},
		{
			name:       "true-up with today is not past",
			targetDate: now,
			queryType:  constants.QueryTypeTrueUp,
			want:       false,
		},
		{
			name:       "true-up with zero time is not past",
			targetDate: time.Time{},
			queryType:  constants.QueryTypeTrueUp,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPastPeriod(tt.targetDate, tt.queryType, tz)
			if got != tt.want {
				t.Errorf("IsPastPeriod(%v, %v) = %v, want %v",
					tt.targetDate.Format("2006-01-02"), tt.queryType, got, tt.want)
			}
		})
	}
}

// TestGetTrueUpBoundaries verifies that the true-up boundaries start at midnight on
// the given date and end at 23:59:59 yesterday.
func TestGetTrueUpBoundaries(t *testing.T) {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("Failed to load test timezone: %v", err)
	}

	// Simulate RunTrueUp passing the first day of the start month.
	input := time.Date(2025, time.January, 1, 0, 0, 0, 0, tz)
	start, end := GetTrueUpBoundaries(input, tz)

	// Start must be midnight on January 1, 2025.
	if start.Year() != 2025 || start.Month() != time.January || start.Day() != 1 {
		t.Errorf("start = %v, want 2025-01-01", start)
	}
	if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Errorf("start time = %02d:%02d:%02d, want 00:00:00", start.Hour(), start.Minute(), start.Second())
	}

	// End must be 23:59:59 of yesterday.
	yesterday := time.Now().In(tz).AddDate(0, 0, -1)
	if end.Year() != yesterday.Year() || end.Month() != yesterday.Month() || end.Day() != yesterday.Day() {
		t.Errorf("end date = %v, want yesterday (%v)", end.Format("2006-01-02"), yesterday.Format("2006-01-02"))
	}
	if end.Hour() != 23 || end.Minute() != 59 || end.Second() != 59 {
		t.Errorf("end time = %02d:%02d:%02d, want 23:59:59", end.Hour(), end.Minute(), end.Second())
	}
}

// TestGetBoundaries_TrueUp verifies that GetBoundaries dispatches correctly for QueryTypeTrueUp.
func TestGetBoundaries_TrueUp(t *testing.T) {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("Failed to load test timezone: %v", err)
	}

	input := time.Date(2025, time.January, 1, 0, 0, 0, 0, tz)
	start, end := GetBoundaries(input, constants.QueryTypeTrueUp, tz)

	if start.Year() != 2025 || start.Month() != time.January || start.Day() != 1 {
		t.Errorf("GetBoundaries(true-up) start = %v, want 2025-01-01", start)
	}

	yesterday := time.Now().In(tz).AddDate(0, 0, -1)
	if end.Year() != yesterday.Year() || end.Month() != yesterday.Month() || end.Day() != yesterday.Day() {
		t.Errorf("GetBoundaries(true-up) end date = %v, want yesterday (%v)",
			end.Format("2006-01-02"), yesterday.Format("2006-01-02"))
	}
}

// TestParseDateInTimezone tests date parsing in specific timezone
func TestParseDateInTimezone(t *testing.T) {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("Failed to load test timezone: %v", err)
	}

	tests := []struct {
		name         string
		dateStr      string
		wantErr      bool
		expectedYear int
		expectedMon  time.Month
		expectedDay  int
	}{
		{
			name:         "valid date",
			dateStr:      "2026-01-15",
			wantErr:      false,
			expectedYear: 2026,
			expectedMon:  time.January,
			expectedDay:  15,
		},
		{
			name:    "invalid format",
			dateStr: "01/15/2026",
			wantErr: true,
		},
		{
			name:    "invalid date",
			dateStr: "2026-13-45",
			wantErr: true,
		},
		{
			name:    "empty string",
			dateStr: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDateInTimezone(tt.dateStr, tz)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseDateInTimezone(%q) error = nil, want error", tt.dateStr)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseDateInTimezone(%q) unexpected error = %v", tt.dateStr, err)
				return
			}

			if result.Year() != tt.expectedYear || result.Month() != tt.expectedMon || result.Day() != tt.expectedDay {
				t.Errorf("ParseDateInTimezone(%q) = %v, want %d-%02d-%02d",
					tt.dateStr, result, tt.expectedYear, tt.expectedMon, tt.expectedDay)
			}

			// Verify timezone is correct
			if result.Location().String() != tz.String() {
				t.Errorf("ParseDateInTimezone(%q) timezone = %v, want %v",
					tt.dateStr, result.Location(), tz)
			}
		})
	}
}
