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
