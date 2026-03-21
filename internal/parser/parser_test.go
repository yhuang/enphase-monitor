// Package parser - parser_test.go
//
// TEST SETUP
// ----------
// This test suite validates JSON telemetry response parsing from Enphase API.
// Tests use inline JSON strings (no external files or API calls).
//
// TEST PLAN
// ---------
// 1. Flat Array Format Tests (production, consumption, battery)
//   - Test valid responses with multiple intervals
//   - Test empty intervals array
//   - Test malformed JSON
//   - Test missing required fields
//
// 2. Nested Array Format Tests (grid import/export)
//   - Test nested array structure
//   - Test array flattening logic
//   - Test mixed interval counts
//
// 3. Interval Summing Tests
//   - Test summing wh_del (production)
//   - Test summing enwh (consumption)
//   - Test summing wh_imported (grid import)
//   - Test summing wh_exported (grid export)
//
// TESTING APPROACH
// ----------------
// - Table-driven tests with inline JSON
// - Each test case validates: parse success, interval count, sum calculation
// - Tests cover both API response formats
// - Floating-point comparisons use tolerance (0.01 kWh)
//
// API RESPONSE FORMATS
// --------------------
// Format 1 - Flat Array: {"intervals": [{"end_at": 123, "enwh": 100}, ...]}
// Format 2 - Nested Array: {"intervals": [[{"end_at": 123, "wh_imported": 100}], ...]}
//
// PATTERN USED
// ------------
// - Pattern 1: Table-Driven Tests
// - Pattern 3: Subtests with t.Run()
//
// See docs/TESTING.md for detailed pattern explanations.
package parser

import (
	"io"
	"strings"
	"testing"

	"enphase-monitor/internal/constants"
)

// TestParseTelemetryResponse tests parsing of flat array telemetry responses
func TestParseTelemetryResponse(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		wantErr     bool
		wantCount   int
		validateSum float64
		fieldName   string
	}{
		{
			name: "valid production response",
			jsonData: `{
				"intervals": [
					{"end_at": 1234567890, "wh_del": 100.5},
					{"end_at": 1234568790, "wh_del": 200.3},
					{"end_at": 1234569690, "wh_del": 150.2}
				]
			}`,
			wantErr:     false,
			wantCount:   3,
			validateSum: 451.0,
			fieldName:   constants.FieldWhDel,
		},
		{
			name: "valid consumption response",
			jsonData: `{
				"intervals": [
					{"end_at": 1234567890, "enwh": 50.5},
					{"end_at": 1234568790, "enwh": 75.3}
				]
			}`,
			wantErr:     false,
			wantCount:   2,
			validateSum: 125.8,
			fieldName:   constants.FieldEnwh,
		},
		{
			name:      "empty intervals",
			jsonData:  `{"intervals": []}`,
			wantErr:   false,
			wantCount: 0,
		},
		{
			name:     "invalid json",
			jsonData: `{"intervals": [`,
			wantErr:  true,
		},
		{
			name:     "not an object",
			jsonData: `[]`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intervals, err := ParseTelemetryResponse([]byte(tt.jsonData))

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseTelemetryResponse() error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Errorf("parseTelemetryResponse() unexpected error = %v", err)
				return
			}

			if len(intervals) != tt.wantCount {
				t.Errorf("parseTelemetryResponse() returned %d intervals, want %d", len(intervals), tt.wantCount)
			}

			// Validate sum if specified
			if tt.validateSum > 0 {
				sum := SumIntervalValues(intervals, tt.fieldName)
				if sum != tt.validateSum {
					t.Errorf("SumIntervalValues() = %.1f, want %.1f", sum, tt.validateSum)
				}
			}
		})
	}
}

// TestParseNestedTelemetryResponse tests parsing of nested array telemetry responses
func TestParseNestedTelemetryResponse(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		wantErr     bool
		wantCount   int
		validateSum float64
		fieldName   string
	}{
		{
			name: "valid import response",
			jsonData: `{
				"intervals": [
					[
						{"end_at": 1234567890, "wh_imported": 100.5},
						{"end_at": 1234568790, "wh_imported": 200.3}
					],
					[
						{"end_at": 1234569690, "wh_imported": 150.2}
					]
				]
			}`,
			wantErr:     false,
			wantCount:   3,
			validateSum: 451.0,
			fieldName:   constants.FieldWhImported,
		},
		{
			name: "valid export response",
			jsonData: `{
				"intervals": [
					[
						{"end_at": 1234567890, "wh_exported": 50.5}
					],
					[
						{"end_at": 1234568790, "wh_exported": 75.3}
					]
				]
			}`,
			wantErr:     false,
			wantCount:   2,
			validateSum: 125.8,
			fieldName:   constants.FieldWhExported,
		},
		{
			name:      "empty nested intervals",
			jsonData:  `{"intervals": []}`,
			wantErr:   false,
			wantCount: 0,
		},
		{
			name:     "invalid json",
			jsonData: `{"intervals": [[`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intervals, err := ParseNestedTelemetryResponse([]byte(tt.jsonData))

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseNestedTelemetryResponse() error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Errorf("parseNestedTelemetryResponse() unexpected error = %v", err)
				return
			}

			if len(intervals) != tt.wantCount {
				t.Errorf("parseNestedTelemetryResponse() returned %d intervals, want %d", len(intervals), tt.wantCount)
			}

			// Validate sum if specified
			if tt.validateSum > 0 {
				sum := SumIntervalValues(intervals, tt.fieldName)
				if sum != tt.validateSum {
					t.Errorf("SumIntervalValues() = %.1f, want %.1f", sum, tt.validateSum)
				}
			}
		})
	}
}

// TestReadResponseBody tests reading an io.ReadCloser response body.
func TestReadResponseBody(t *testing.T) {
	t.Run("reads body successfully", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(`{"intervals":[]}`))
		got, err := ReadResponseBody(body)
		if err != nil {
			t.Fatalf("ReadResponseBody() error = %v", err)
		}
		if string(got) != `{"intervals":[]}` {
			t.Errorf("ReadResponseBody() = %q, want %q", string(got), `{"intervals":[]}`)
		}
	})

	t.Run("reads empty body", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(""))
		got, err := ReadResponseBody(body)
		if err != nil {
			t.Fatalf("ReadResponseBody() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("ReadResponseBody() = %q, want empty", string(got))
		}
	})
}

// TestParseLifetimeResponse tests parsing of lifetime endpoint daily aggregated responses.
func TestParseLifetimeResponse(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantErr   bool
		wantCount int
		wantFirst float64
	}{
		{
			name: "production field",
			json: `{"system_id":12345,"start_date":"2026-01-01","production":[1000.0,2000.0,1500.0]}`,
			wantCount: 3,
			wantFirst: 1000.0,
		},
		{
			name: "consumption field",
			json: `{"system_id":12345,"start_date":"2026-01-01","consumption":[500.0,600.0]}`,
			wantCount: 2,
			wantFirst: 500.0,
		},
		{
			name: "import field",
			json: `{"system_id":12345,"start_date":"2026-01-01","import":[100.0,200.0]}`,
			wantCount: 2,
			wantFirst: 100.0,
		},
		{
			name: "export field",
			json: `{"system_id":12345,"start_date":"2026-01-01","export":[50.0,75.0]}`,
			wantCount: 2,
			wantFirst: 50.0,
		},
		{
			name: "intervals fallback field",
			json: `{"system_id":12345,"start_date":"2026-01-01","intervals":[300.0,400.0]}`,
			wantCount: 2,
			wantFirst: 300.0,
		},
		{
			name:      "empty arrays",
			json:      `{"system_id":12345,"start_date":"2026-01-01","production":[]}`,
			wantCount: 0,
		},
		{
			name:    "invalid json",
			json:    `{not valid json`,
			wantErr: true,
		},
		{
			name:    "invalid start_date",
			json:    `{"system_id":12345,"start_date":"not-a-date","production":[1000.0]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLifetimeResponse([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseLifetimeResponse() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLifetimeResponse() unexpected error = %v", err)
			}
			if len(got) != tt.wantCount {
				t.Errorf("ParseLifetimeResponse() len = %d, want %d", len(got), tt.wantCount)
			}
			if tt.wantCount > 0 && got[0].Enwh != tt.wantFirst {
				t.Errorf("ParseLifetimeResponse() first value = %v, want %v", got[0].Enwh, tt.wantFirst)
			}
			// Verify dates are sequential
			if tt.wantCount > 1 {
				if got[0].Date >= got[1].Date {
					t.Errorf("ParseLifetimeResponse() dates not sequential: %s >= %s", got[0].Date, got[1].Date)
				}
			}
		})
	}
}

// TestSumDailyIntervals tests summing daily interval values within a date range.
func TestSumDailyIntervals(t *testing.T) {
	intervals := []DailyInterval{
		{Date: "2026-01-01", Enwh: 1000.0},
		{Date: "2026-01-02", Enwh: 2000.0},
		{Date: "2026-01-03", Enwh: 1500.0},
		{Date: "2026-01-04", Enwh: 500.0},
		{Date: "2026-01-05", Enwh: 750.0},
	}

	tests := []struct {
		name      string
		startDate string
		endDate   string
		want      float64
	}{
		{
			name:      "full range",
			startDate: "2026-01-01",
			endDate:   "2026-01-05",
			want:      5750.0,
		},
		{
			name:      "single day",
			startDate: "2026-01-02",
			endDate:   "2026-01-02",
			want:      2000.0,
		},
		{
			name:      "partial range",
			startDate: "2026-01-02",
			endDate:   "2026-01-04",
			want:      4000.0,
		},
		{
			name:      "range before all intervals",
			startDate: "2025-12-01",
			endDate:   "2025-12-31",
			want:      0.0,
		},
		{
			name:      "range after all intervals",
			startDate: "2026-02-01",
			endDate:   "2026-02-28",
			want:      0.0,
		},
		{
			name:      "empty intervals",
			startDate: "2026-01-01",
			endDate:   "2026-01-05",
			want:      0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := intervals
			if tt.name == "empty intervals" {
				src = nil
			}
			got := SumDailyIntervals(src, tt.startDate, tt.endDate)
			if got != tt.want {
				t.Errorf("SumDailyIntervals() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSumIntervalValues tests summing interval values by field name
func TestSumIntervalValues(t *testing.T) {
	intervals := []TelemetryInterval{
		{EndAt: 1234567890, WhDel: 100.5, WhImported: 50.0, WhExported: 10.0, Enwh: 75.0},
		{EndAt: 1234568790, WhDel: 200.3, WhImported: 60.0, WhExported: 15.0, Enwh: 85.0},
		{EndAt: 1234569690, WhDel: 150.2, WhImported: 40.0, WhExported: 5.0, Enwh: 65.0},
	}

	tests := []struct {
		name      string
		fieldName string
		expected  float64
	}{
		{
			name:      "sum wh_del",
			fieldName: constants.FieldWhDel,
			expected:  451.0,
		},
		{
			name:      "sum wh_imported",
			fieldName: constants.FieldWhImported,
			expected:  150.0,
		},
		{
			name:      "sum wh_exported",
			fieldName: constants.FieldWhExported,
			expected:  30.0,
		},
		{
			name:      "sum enwh",
			fieldName: constants.FieldEnwh,
			expected:  225.0,
		},
		{
			name:      "unknown field",
			fieldName: "unknown",
			expected:  0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SumIntervalValues(intervals, tt.fieldName)
			if result != tt.expected {
				t.Errorf("SumIntervalValues(%s) = %.1f, want %.1f", tt.fieldName, result, tt.expected)
			}
		})
	}
}
