package display

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
)

// mustLoadLocation loads a timezone for tests; fails the test if the timezone is invalid.
func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// TestNewDisplayWithWriter verifies that the display can be created with a custom writer.
func TestNewDisplayWithWriter(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	if d == nil {
		t.Fatal("NewDisplayWithWriter returned nil")
	}
	if d.writer != &buf {
		t.Errorf("NewDisplayWithWriter(..., &buf): writer = %p, want %p", d.writer, &buf)
	}
	if d.timezone != tz {
		t.Errorf("NewDisplayWithWriter(..., tz): timezone = %v, want %v", d.timezone, tz)
	}
}

// TestNewDisplayWithColorsAndTimezone verifies the default constructor.
func TestNewDisplayWithColorsAndTimezone(t *testing.T) {
	tz := mustLoadLocation(t, "US/Pacific")

	d := NewDisplayWithColorsAndTimezone(config.ColorConfig{}, tz)

	if d == nil {
		t.Fatal("NewDisplayWithColorsAndTimezone returned nil")
	}
	// Should use os.Stdout as default writer
	if d.writer == nil {
		t.Error("NewDisplayWithColorsAndTimezone(...): writer = nil, want non-nil")
	}
}

// TestShowMetrics_ContainsHeader verifies that ShowMetrics outputs the header.
func TestShowMetrics_ContainsHeader(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp:        time.Now(),
		ProductionToday:  10.5,
		ConsumptionToday: 8.2,
		GridImportToday:  2.0,
		GridExportToday:  4.3,
		NetFlowToday:     -2.3, // Net export
	}

	d.ShowMetrics(metrics)

	output := buf.String()

	// Verify header is present
	if !strings.Contains(output, "ENPHASE MULTI-SYSTEM MONITOR") {
		t.Error("Output should contain header 'ENPHASE MULTI-SYSTEM MONITOR'")
	}
}

// TestShowMetrics_WeatherShownWhenPresent verifies the weather block is rendered
// when the metrics carry it.
func TestShowMetrics_WeatherShownWhenPresent(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp: time.Now(),
		Weather: &aggregator.DailyWeather{
			TempHigh:       78.4,
			TempLow:        54.1,
			TempUnit:       "°F",
			Condition:      "Partly cloudy",
			CloudCoverPct:  41,
			SolarRadiation: 6.8,
		},
	}
	d.ShowMetrics(metrics)

	output := buf.String()
	for _, want := range []string{"Temperature", "78°F", "54°F", "Partly cloudy", "41% cloud", "Irradiance", "6.8 kWh/m²"} {
		if !strings.Contains(output, want) {
			t.Errorf("output should contain %q\n%s", want, output)
		}
	}
}

// TestShowMetrics_WeatherCurrentLayout verifies the live-today layout shows the
// current condition/temp plus the day's range.
func TestShowMetrics_WeatherCurrentLayout(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp: time.Now(),
		Weather: &aggregator.DailyWeather{
			TempHigh: 77, TempLow: 59, TempUnit: "°F", SolarRadiation: 6.9,
			HasCurrent: true, CurrentTemp: 63, CurrentCondition: "Clear", CurrentCloudCoverPct: 2,
		},
	}
	d.ShowMetrics(metrics)

	output := buf.String()
	for _, want := range []string{"63°F now", "59°F — 77°F", "Clear", "2% cloud", "kWh/m² forecasted"} {
		if !strings.Contains(output, want) {
			t.Errorf("current layout should contain %q\n%s", want, output)
		}
	}
}

// TestShowMetrics_WeatherAbsentWhenNil verifies no weather block is rendered
// when Weather is nil (e.g. month/year/true-up reports).
func TestShowMetrics_WeatherAbsentWhenNil(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{Timestamp: time.Now()}
	d.ShowMetrics(metrics)

	for _, unwanted := range []string{"Temperature", "Conditions", "Irradiance"} {
		if strings.Contains(buf.String(), unwanted) {
			t.Errorf("output should not contain %q when Weather is nil\n%s", unwanted, buf.String())
		}
	}
}

// TestShowMetrics_ContainsMetricValues verifies that ShowMetrics outputs metric values.
func TestShowMetrics_ContainsMetricValues(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")

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

// TestShowMetrics_CombinedSectionLabels verifies that the combined energy report
// includes all expected labels, including grid import and export.
func TestShowMetrics_CombinedSectionLabels(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp:        time.Now(),
		ProductionToday:  10.5,
		ConsumptionToday: 8.2,
		GridImportToday:  2.0,
		GridExportToday:  4.3,
		NetFlowToday:     -2.3,
	}

	d.ShowMetrics(metrics)

	output := buf.String()

	for _, want := range []string{
		"Production",
		"Consumption",
		"Grid Import",
		"Grid Export",
		"Net Flow",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("combined section missing %q", want)
		}
	}
}

// TestShowMetrics_CacheIndicator verifies that cache status is displayed.
func TestShowMetrics_CacheIndicator(t *testing.T) {
	tz := mustLoadLocation(t, "US/Pacific")

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
	tz := mustLoadLocation(t, "US/Pacific")

	tests := []struct {
		name         string
		netFlowToday float64
		expected     string
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
				Timestamp:    time.Now(),
				NetFlowToday: tt.netFlowToday,
			}

			d.ShowMetrics(metrics)

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("Output should contain %q for netFlowToday=%v", tt.expected, tt.netFlowToday)
			}
		})
	}
}

// TestShowMetrics_IndividualSystems verifies that individual systems are displayed when there are multiple.
func TestShowMetrics_IndividualSystems(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")

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
	tz := mustLoadLocation(t, "US/Pacific")

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
	tz := mustLoadLocation(t, "US/Pacific")

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
	tz := mustLoadLocation(t, "US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	d.ShowInfo("Starting continuous monitoring")

	output := buf.String()

	if !strings.Contains(output, "Starting continuous monitoring") {
		t.Error("Output should contain the info message")
	}
}

// TestShowMetrics_Battery_DayQuery verifies all battery metrics are shown for day queries.
func TestShowMetrics_Battery_DayQuery(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp: time.Now(),
		QueryMode: constants.QueryModeDay,
		Systems: []aggregator.SystemMetrics{
			{Name: "Home System", ID: "12345", BatterySOC: 75, BatteryChargedToday: 5.0, BatteryDischargedToday: 3.0},
			{Name: "Office System", ID: "67890", BatterySOC: 50, BatteryChargedToday: 2.0, BatteryDischargedToday: 1.0},
		},
	}

	d.ShowMetrics(metrics)

	output := buf.String()

	// All three battery fields should appear for day queries
	if !strings.Contains(output, "Battery Charge") {
		t.Error("Output should contain 'Battery Charge' for Day Mode query")
	}
	if !strings.Contains(output, "Battery Discharge") {
		t.Error("Output should contain 'Battery Discharge' for Day Mode query")
	}
	if !strings.Contains(output, "Battery State of Charge") {
		t.Error("Output should contain 'Battery State of Charge' for day query")
	}
}

// TestShowMetrics_Battery_MonthQuery verifies battery metrics are hidden for month queries.
func TestShowMetrics_Battery_MonthQuery(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp: time.Now(),
		QueryMode: constants.QueryModeMonth,
		Systems: []aggregator.SystemMetrics{
			{Name: "Home System", ID: "12345", BatterySOC: 75},
			{Name: "Office System", ID: "67890", BatterySOC: 50},
		},
	}

	d.ShowMetrics(metrics)

	output := buf.String()

	// No battery fields should appear for month queries
	if strings.Contains(output, "Battery Charge") {
		t.Error("Output should NOT contain 'Battery Charge' for Month Mode query")
	}
	if strings.Contains(output, "Battery Discharge") {
		t.Error("Output should NOT contain 'Battery Discharge' for Month Mode query")
	}
	if strings.Contains(output, "Battery State of Charge") {
		t.Error("Output should NOT contain 'Battery State of Charge' for month query")
	}
}

// TestShowMetrics_Battery_YearQuery verifies battery metrics are hidden for year queries.
func TestShowMetrics_Battery_YearQuery(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")

	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp: time.Now(),
		QueryMode: constants.QueryModeYear,
		Systems: []aggregator.SystemMetrics{
			{Name: "Home System", ID: "12345", BatterySOC: 75},
			{Name: "Office System", ID: "67890", BatterySOC: 50},
		},
	}

	d.ShowMetrics(metrics)

	output := buf.String()

	// No battery fields should appear for year queries
	if strings.Contains(output, "Battery Charge") {
		t.Error("Output should NOT contain 'Battery Charge' for Year Mode query")
	}
	if strings.Contains(output, "Battery Discharge") {
		t.Error("Output should NOT contain 'Battery Discharge' for Year Mode query")
	}
	if strings.Contains(output, "Battery State of Charge") {
		t.Error("Output should NOT contain 'Battery State of Charge' for year query")
	}
}

// makeTrueUpReport is a test fixture that builds a TrueUpReport with two systems.
func makeTrueUpReport(tz *time.Location) *aggregator.TrueUpReport {
	startDate := time.Date(2025, time.January, 15, 0, 0, 0, 0, tz)
	endDate := time.Date(2026, time.April, 24, 23, 59, 59, 0, tz)
	return &aggregator.TrueUpReport{
		StartDate:   startDate,
		EndDate:     endDate,
		Timestamp:   time.Date(2026, time.April, 25, 10, 30, 0, 0, tz),
		CacheUsed:   false,
		GridImport:  1234.5,
		GridExport:  2345.6,
		Production:  3456.7,
		Consumption: 2345.6,
		NetFlow:     -1111.1, // net export
		Systems: []aggregator.TrueUpSystemReport{
			{Name: "Right Subpanel", ID: "5525881", GridImport: 600, GridExport: 1200, Production: 1700, Consumption: 1100, NetFlow: -600},
			{Name: "Left Subpanel", ID: "5392556", GridImport: 634.5, GridExport: 1145.6, Production: 1756.7, Consumption: 1245.6, NetFlow: -511.1},
		},
	}
}

// TestShowTrueUpReport_ContainsHeader verifies the standard header elements are present.
func TestShowTrueUpReport_ContainsHeader(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	d.ShowTrueUpReport(makeTrueUpReport(tz))
	output := buf.String()

	checks := []string{
		"ENPHASE MULTI-SYSTEM MONITOR",
		"True-Up Start:",
		"Query Range:",
		"Last Updated:",
	}
	for _, want := range checks {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

// TestShowTrueUpReport_StartDateInHeader verifies the user-provided start date appears.
func TestShowTrueUpReport_StartDateInHeader(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	d.ShowTrueUpReport(makeTrueUpReport(tz))
	output := buf.String()

	// The start date Jan 15, 2025 must appear in the header
	if !strings.Contains(output, "Jan 15, 2025") {
		t.Error("output should contain the true-up start date 'Jan 15, 2025'")
	}
	// Data range should start from Jan 1 (first of the start month)
	if !strings.Contains(output, "Jan 1, 2025") {
		t.Error("output should contain data range start 'Jan 1, 2025' (first of start month)")
	}
}

// TestShowTrueUpReport_CombinedSection verifies the TRUE-UP ENERGY REPORT section.
func TestShowTrueUpReport_CombinedSection(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	d.ShowTrueUpReport(makeTrueUpReport(tz))
	output := buf.String()

	checks := []string{
		"TRUE-UP ENERGY REPORT",
		"Production",
		"Consumption",
		"Grid Import",
		"Grid Export",
		"Net Flow",
	}
	for _, want := range checks {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

// TestShowTrueUpReport_NetFlowDirection verifies the net flow direction label.
func TestShowTrueUpReport_NetFlowDirection(t *testing.T) {
	tz := mustLoadLocation(t, "US/Pacific")

	tests := []struct {
		name    string
		netFlow float64
		wantDir string
		noDir   string
	}{
		{"net export", -500.0, "(export)", "(import)"},
		{"net import", 500.0, "(import)", "(export)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

			report := makeTrueUpReport(tz)
			report.NetFlow = tt.netFlow

			d.ShowTrueUpReport(report)
			output := buf.String()

			if !strings.Contains(output, tt.wantDir) {
				t.Errorf("output should contain %q for NetFlow=%.1f", tt.wantDir, tt.netFlow)
			}
		})
	}
}

// TestShowTrueUpReport_IndividualSystemsShownForMultiple verifies per-system section
// appears when there are multiple systems.
func TestShowTrueUpReport_IndividualSystemsShownForMultiple(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	d.ShowTrueUpReport(makeTrueUpReport(tz))
	output := buf.String()

	if !strings.Contains(output, "INDIVIDUAL SYSTEMS REPORT") {
		t.Error("output should contain 'INDIVIDUAL SYSTEMS REPORT' for multiple systems")
	}
	if !strings.Contains(output, "Right Subpanel") {
		t.Error("output should contain system name 'Right Subpanel'")
	}
	if !strings.Contains(output, "Left Subpanel") {
		t.Error("output should contain system name 'Left Subpanel'")
	}
}

// TestShowTrueUpReport_IndividualSystemsHiddenForSingle verifies per-system section
// is suppressed when there is only one system.
func TestShowTrueUpReport_IndividualSystemsHiddenForSingle(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	report := makeTrueUpReport(tz)
	report.Systems = report.Systems[:1] // trim to one system

	d.ShowTrueUpReport(report)
	output := buf.String()

	if strings.Contains(output, "INDIVIDUAL SYSTEMS REPORT") {
		t.Error("output should NOT contain 'INDIVIDUAL SYSTEMS REPORT' for a single system")
	}
}

// TestShowTrueUpReport_PerSystemMetrics verifies the per-system section shows the
// correct five metrics and omits battery metrics.
func TestShowTrueUpReport_PerSystemMetrics(t *testing.T) {
	var buf bytes.Buffer
	tz := mustLoadLocation(t, "US/Pacific")
	d := NewDisplayWithWriter(config.ColorConfig{}, tz, &buf)

	d.ShowTrueUpReport(makeTrueUpReport(tz))
	output := buf.String()

	wantLabels := []string{
		"Net Flow",
		"Production",
		"Consumption",
		"Grid Import",
		"Grid Export",
	}
	for _, label := range wantLabels {
		if !strings.Contains(output, label) {
			t.Errorf("output missing per-system label %q", label)
		}
	}

	// Battery metrics must be absent
	batteryLabels := []string{"Battery", "Charged", "Discharged", "Battery State of Charge"}
	for _, label := range batteryLabels {
		if strings.Contains(output, label) {
			t.Errorf("output should NOT contain battery label %q in true-up report", label)
		}
	}
}

// testError is a simple error implementation for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
