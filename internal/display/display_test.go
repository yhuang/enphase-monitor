package display

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/config"
)

// TestNewDisplayWithWriter verifies that the display can be created with a custom writer.
func TestNewDisplayWithWriter(t *testing.T) {
	var buf bytes.Buffer
	tz, _ := time.LoadLocation("US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	if d == nil {
		t.Fatal("NewDisplayWithWriter returned nil")
	}
	if d.writer != &buf {
		t.Error("Display writer was not set correctly")
	}
	if d.timezone != tz {
		t.Error("Display timezone was not set correctly")
	}
}

// TestNewDisplayWithColorsAndTimezone verifies the default constructor.
func TestNewDisplayWithColorsAndTimezone(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")

	d := NewDisplayWithColorsAndTimezone(config.ColorConfig{}, tz)

	if d == nil {
		t.Fatal("NewDisplayWithColorsAndTimezone returned nil")
	}
	// Should use os.Stdout as default writer
	if d.writer == nil {
		t.Error("Display writer should not be nil")
	}
}

// TestShowMetrics_ContainsHeader verifies that ShowMetrics outputs the header.
func TestShowMetrics_ContainsHeader(t *testing.T) {
	var buf bytes.Buffer
	tz, _ := time.LoadLocation("US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp:        time.Now(),
		ProductionToday:  10.5,
		ConsumptionToday: 8.2,
		GridImportToday:  2.0,
		GridExportToday:  4.3,
		NetImportToday:   -2.3, // Net export
	}

	d.ShowMetrics(metrics)

	output := buf.String()

	// Verify header is present
	if !strings.Contains(output, "ENPHASE MULTI-SYSTEM MONITOR") {
		t.Error("Output should contain header 'ENPHASE MULTI-SYSTEM MONITOR'")
	}
}

// TestShowMetrics_ContainsMetricValues verifies that ShowMetrics outputs metric values.
func TestShowMetrics_ContainsMetricValues(t *testing.T) {
	var buf bytes.Buffer
	tz, _ := time.LoadLocation("US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp:        time.Now(),
		ProductionToday:  10.5,
		ConsumptionToday: 8.2,
	}

	d.ShowMetrics(metrics)

	output := buf.String()

	// Verify metric values are present
	if !strings.Contains(output, "10.5") {
		t.Error("Output should contain production value '10.5'")
	}
	if !strings.Contains(output, "8.2") {
		t.Error("Output should contain consumption value '8.2'")
	}
}

// TestShowMetrics_CacheIndicator verifies that cache status is displayed.
func TestShowMetrics_CacheIndicator(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")

	tests := []struct {
		name      string
		cacheUsed bool
		expected  string
	}{
		{"cached", true, "(cached)"},
		{"live", false, "(live)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

			metrics := &aggregator.AggregatedMetrics{
				Timestamp: time.Now(),
				CacheUsed: tt.cacheUsed,
			}

			d.ShowMetrics(metrics)

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("Output should contain %q for cacheUsed=%v", tt.expected, tt.cacheUsed)
			}
		})
	}
}

// TestShowMetrics_NetFlow verifies that net flow displays correctly.
func TestShowMetrics_NetFlow(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")

	tests := []struct {
		name           string
		netImportToday float64
		expected       string
	}{
		{"net export", -5.0, "(export)"},
		{"net import", 5.0, "(import)"},
		{"zero", 0.0, "(import)"}, // Zero is treated as import
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

			metrics := &aggregator.AggregatedMetrics{
				Timestamp:      time.Now(),
				NetImportToday: tt.netImportToday,
			}

			d.ShowMetrics(metrics)

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("Output should contain %q for netImportToday=%v", tt.expected, tt.netImportToday)
			}
		})
	}
}

// TestShowMetrics_IndividualSystems verifies that individual systems are displayed when there are multiple.
func TestShowMetrics_IndividualSystems(t *testing.T) {
	var buf bytes.Buffer
	tz, _ := time.LoadLocation("US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp: time.Now(),
		Systems: []aggregator.SystemMetrics{
			{Name: "Home System", ID: "12345", ProductionToday: 5.5},
			{Name: "Office System", ID: "67890", ProductionToday: 4.5},
		},
	}

	d.ShowMetrics(metrics)

	output := buf.String()

	// Verify individual systems section appears
	if !strings.Contains(output, "INDIVIDUAL SYSTEMS REPORT") {
		t.Error("Output should contain 'INDIVIDUAL SYSTEMS REPORT' when multiple systems exist")
	}
	if !strings.Contains(output, "Home System") {
		t.Error("Output should contain 'Home System'")
	}
	if !strings.Contains(output, "Office System") {
		t.Error("Output should contain 'Office System'")
	}
}

// TestShowMetrics_SingleSystem verifies that individual systems section is not shown for single system.
func TestShowMetrics_SingleSystem(t *testing.T) {
	var buf bytes.Buffer
	tz, _ := time.LoadLocation("US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp: time.Now(),
		Systems: []aggregator.SystemMetrics{
			{Name: "Home System", ID: "12345", ProductionToday: 5.5},
		},
	}

	d.ShowMetrics(metrics)

	output := buf.String()

	// Verify individual systems section does NOT appear for single system
	if strings.Contains(output, "INDIVIDUAL SYSTEMS REPORT") {
		t.Error("Output should NOT contain 'INDIVIDUAL SYSTEMS REPORT' for single system")
	}
}

// TestShowError verifies that ShowError outputs error messages.
func TestShowError(t *testing.T) {
	var buf bytes.Buffer
	tz, _ := time.LoadLocation("US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	testErr := &testError{msg: "test error message"}
	d.ShowError(testErr)

	output := buf.String()

	if !strings.Contains(output, "ERROR") {
		t.Error("Output should contain 'ERROR'")
	}
	if !strings.Contains(output, "test error message") {
		t.Error("Output should contain the error message")
	}
}

// TestShowInfo verifies that ShowInfo outputs informational messages.
func TestShowInfo(t *testing.T) {
	var buf bytes.Buffer
	tz, _ := time.LoadLocation("US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	d.ShowInfo("Starting continuous monitoring")

	output := buf.String()

	if !strings.Contains(output, "Starting continuous monitoring") {
		t.Error("Output should contain the info message")
	}
}

// TestGetDateRange verifies the date range calculation helper.
func TestGetDateRange(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &bytes.Buffer{})

	// Test case: query date in the past
	now := time.Date(2026, 1, 30, 15, 30, 0, 0, tz)
	queryDate := time.Date(2026, 1, 20, 10, 0, 0, 0, tz)

	start, end := d.getDateRange(queryDate, now)

	// Start should be midnight of query date
	expectedStart := time.Date(2026, 1, 20, 0, 0, 0, 0, tz)
	if !start.Equal(expectedStart) {
		t.Errorf("Start should be %v, got %v", expectedStart, start)
	}

	// End should be 23:59:59 of query date (past date)
	expectedEnd := time.Date(2026, 1, 20, 23, 59, 59, 0, tz)
	if !end.Equal(expectedEnd) {
		t.Errorf("End should be %v, got %v", expectedEnd, end)
	}
}

// TestGetDateRange_Today verifies date range for today.
func TestGetDateRange_Today(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &bytes.Buffer{})

	// Test case: query date is today
	now := time.Date(2026, 1, 30, 15, 30, 0, 0, tz)
	queryDate := time.Date(2026, 1, 30, 10, 0, 0, 0, tz) // Same day

	start, end := d.getDateRange(queryDate, now)

	// Start should be midnight of today
	expectedStart := time.Date(2026, 1, 30, 0, 0, 0, 0, tz)
	if !start.Equal(expectedStart) {
		t.Errorf("Start should be %v, got %v", expectedStart, start)
	}

	// End should be current time (now)
	if !end.Equal(now) {
		t.Errorf("End should be %v (now), got %v", now, end)
	}
}

// TestGetDateRange_ZeroQueryDate verifies date range when query date is zero (today).
func TestGetDateRange_ZeroQueryDate(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &bytes.Buffer{})

	// Test case: zero query date (means today)
	now := time.Date(2026, 1, 30, 15, 30, 0, 0, tz)
	queryDate := time.Time{} // Zero value

	start, end := d.getDateRange(queryDate, now)

	// Start should be midnight of today
	expectedStart := time.Date(2026, 1, 30, 0, 0, 0, 0, tz)
	if !start.Equal(expectedStart) {
		t.Errorf("Start should be %v, got %v", expectedStart, start)
	}

	// End should be current time (now)
	if !end.Equal(now) {
		t.Errorf("End should be %v (now), got %v", now, end)
	}
}

// testError is a simple error implementation for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
