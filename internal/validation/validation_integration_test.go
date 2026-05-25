// Package validation - validation_integration_test.go
//
// Integration tests for validation logic using real expected-values JSON files
// from the test-data directory.
package validation

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"enphase-monitor/internal/aggregator"
)

// testProjectRoot returns the absolute path to the repository root by resolving
// two directories up from this source file. This avoids os.Chdir (process-global
// state) while still allowing tests to locate the test-data directory.
func testProjectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// this file lives at internal/validation/; project root is two dirs up
	return filepath.Join(filepath.Dir(filename), "../..")
}

// TestValidateMetrics_FullFlow_Success tests successful validation with matching metrics
func TestValidateMetrics_FullFlow_Success(t *testing.T) {
	root := testProjectRoot(t)
	if _, err := os.Stat(filepath.Join(root, "test-data/expected_values_2026-01-20.json")); os.IsNotExist(err) {
		t.Skip("Expected values file not found - skipping integration test")
	}

	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:                     "5525881",
				Name:                   "Right Subpanel",
				GridImportToday:        23.4,
				GridExportToday:        3.9,
				ProductionToday:        14.6,
				BatteryDischargedToday: 6.8,
				BatteryChargedToday:    8.6,
				NetFlowToday:           19.6,
				ConsumptionToday:       32.3,
			},
			{
				ID:                     "5392556",
				Name:                   "Left Subpanel",
				GridImportToday:        7.5,
				GridExportToday:        7.6,
				ProductionToday:        19.3,
				BatteryDischargedToday: 5.5,
				BatteryChargedToday:    8.1,
				NetFlowToday:           -0.2,
				ConsumptionToday:       16.5,
			},
		},
	}

	var buf bytes.Buffer
	if err := validateMetricsFromRoot(&buf, metrics, "2026-01-20", root); err != nil {
		t.Errorf("Validation failed with matching metrics: %v", err)
	}
	if !strings.Contains(buf.String(), "ALL VALIDATIONS PASSED") {
		t.Error("Expected output to contain 'ALL VALIDATIONS PASSED'")
	}
}

// TestValidateMetrics_FullFlow_WithinTolerance tests validation with metrics within tolerance
func TestValidateMetrics_FullFlow_WithinTolerance(t *testing.T) {
	root := testProjectRoot(t)
	if _, err := os.Stat(filepath.Join(root, "test-data/expected_values_2026-01-20.json")); os.IsNotExist(err) {
		t.Skip("Expected values file not found - skipping integration test")
	}

	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:                     "5525881",
				Name:                   "Right Subpanel",
				GridImportToday:        23.2, // Within tolerance of 23.4
				GridExportToday:        3.8,  // Within tolerance of 3.9
				ProductionToday:        14.5, // Within tolerance of 14.6
				BatteryDischargedToday: 6.7,  // Within tolerance of 6.8
				BatteryChargedToday:    8.5,  // Within tolerance of 8.6
				NetFlowToday:           19.5, // Within tolerance of 19.6
				ConsumptionToday:       32.2, // Within tolerance of 32.3
			},
			{
				ID:                     "5392556",
				Name:                   "Left Subpanel",
				GridImportToday:        7.4,   // Within tolerance of 7.5
				GridExportToday:        7.5,   // Within tolerance of 7.6
				ProductionToday:        19.2,  // Within tolerance of 19.3
				BatteryDischargedToday: 5.4,   // Within tolerance of 5.5
				BatteryChargedToday:    8.0,   // Within tolerance of 8.1
				NetFlowToday:           -0.15, // Within tolerance of -0.2
				ConsumptionToday:       16.4,  // Within tolerance of 16.5
			},
		},
	}

	var buf bytes.Buffer
	if err := validateMetricsFromRoot(&buf, metrics, "2026-01-20", root); err != nil {
		t.Errorf("Validation failed with metrics within tolerance: %v", err)
	}
	if !strings.Contains(buf.String(), "ALL VALIDATIONS PASSED") {
		t.Error("Expected output to contain 'ALL VALIDATIONS PASSED'")
	}
}

// TestValidateMetrics_FullFlow_OutsideTolerance tests validation with metrics outside tolerance
func TestValidateMetrics_FullFlow_OutsideTolerance(t *testing.T) {
	root := testProjectRoot(t)
	if _, err := os.Stat(filepath.Join(root, "test-data/expected_values_2026-01-20.json")); os.IsNotExist(err) {
		t.Skip("Expected values file not found - skipping integration test")
	}

	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:                     "5525881",
				Name:                   "Right Subpanel",
				GridImportToday:        30.0, // Far from 23.4 (28% difference)
				GridExportToday:        3.9,
				ProductionToday:        14.6,
				BatteryDischargedToday: 6.8,
				BatteryChargedToday:    8.6,
				NetFlowToday:           19.6,
				ConsumptionToday:       32.3,
			},
			{
				ID:                     "5392556",
				Name:                   "Left Subpanel",
				GridImportToday:        7.5,
				GridExportToday:        7.6,
				ProductionToday:        19.3,
				BatteryDischargedToday: 5.5,
				BatteryChargedToday:    8.1,
				NetFlowToday:           -0.2,
				ConsumptionToday:       16.5,
			},
		},
	}

	var buf bytes.Buffer
	err := validateMetricsFromRoot(&buf, metrics, "2026-01-20", root)
	if err == nil {
		t.Error("Expected validation to fail with metrics outside tolerance, but it passed")
	}
	output := buf.String()
	if !strings.Contains(output, "SOME VALIDATIONS FAILED") {
		t.Error("Expected output to contain 'SOME VALIDATIONS FAILED'")
	}
	if !strings.Contains(output, "❌") && !strings.Contains(output, "Grid Import") {
		t.Error("Expected output to show Grid Import failure")
	}
}

// TestValidateMetrics_MissingSystem tests validation when a system is missing
func TestValidateMetrics_MissingSystem(t *testing.T) {
	root := testProjectRoot(t)
	if _, err := os.Stat(filepath.Join(root, "test-data/expected_values_2026-01-20.json")); os.IsNotExist(err) {
		t.Skip("Expected values file not found - skipping integration test")
	}

	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:                     "5525881",
				Name:                   "Right Subpanel",
				GridImportToday:        23.4,
				GridExportToday:        3.9,
				ProductionToday:        14.6,
				BatteryDischargedToday: 6.8,
				BatteryChargedToday:    8.6,
				NetFlowToday:           19.6,
				ConsumptionToday:       32.3,
			},
			// Missing second system
		},
	}

	var buf bytes.Buffer
	err := validateMetricsFromRoot(&buf, metrics, "2026-01-20", root)
	if err == nil {
		t.Error("Expected validation to fail with missing system, but it passed")
	}
	if !strings.Contains(buf.String(), "not found in actual metrics") {
		t.Error("Expected output to indicate missing system")
	}
}

// TestValidateMetrics_DateMismatch tests handling of a date with no expected-values file
func TestValidateMetrics_DateMismatch(t *testing.T) {
	root := testProjectRoot(t)
	if _, err := os.Stat(filepath.Join(root, "test-data/expected_values_2026-01-20.json")); os.IsNotExist(err) {
		t.Skip("Expected values file not found - skipping integration test")
	}

	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:   "5525881",
				Name: "Right Subpanel",
			},
		},
	}

	var buf bytes.Buffer
	err := validateMetricsFromRoot(&buf, metrics, "2026-01-99", root)
	if err == nil {
		t.Error("Expected error for non-existent date file, got nil")
	}
}

// TestValidateMetrics_MultipleExpectedValuesFiles tests with different dates
func TestValidateMetrics_MultipleExpectedValuesFiles(t *testing.T) {
	root := testProjectRoot(t)

	tests := []struct {
		name string
		date string
	}{
		{"2026-01-14", "2026-01-14"},
		{"2026-01-15", "2026-01-15"},
		{"2026-01-16", "2026-01-16"},
		{"2026-01-17", "2026-01-17"},
		{"2026-01-18", "2026-01-18"},
		{"2026-01-19", "2026-01-19"},
		{"2026-01-20", "2026-01-20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename := filepath.Join(root, "test-data", "expected_values_"+tt.date+".json")
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				t.Skipf("Expected values file %s not found - skipping", filename)
			}

			metrics := &aggregator.AggregatedMetrics{
				Systems: []aggregator.SystemMetrics{},
			}

			var buf bytes.Buffer
			err := validateMetricsFromRoot(&buf, metrics, tt.date, root)
			if err == nil {
				t.Error("Expected error for empty metrics, got nil")
			}
		})
	}
}

// TestValidateMetrics_RealWorldScenario tests a realistic validation scenario
func TestValidateMetrics_RealWorldScenario(t *testing.T) {
	root := testProjectRoot(t)
	if _, err := os.Stat(filepath.Join(root, "test-data/expected_values_2026-01-20.json")); os.IsNotExist(err) {
		t.Skip("Expected values file not found - skipping integration test")
	}

	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:                     "5525881",
				Name:                   "Right Subpanel",
				GridImportToday:        23.38,
				GridExportToday:        3.92,
				ProductionToday:        14.58,
				BatteryDischargedToday: 6.79,
				BatteryChargedToday:    8.62,
				NetFlowToday:           19.46,
				ConsumptionToday:       32.28,
			},
			{
				ID:                     "5392556",
				Name:                   "Left Subpanel",
				GridImportToday:        7.48,
				GridExportToday:        7.62,
				ProductionToday:        19.28,
				BatteryDischargedToday: 5.52,
				BatteryChargedToday:    8.08,
				NetFlowToday:           -0.14,
				ConsumptionToday:       16.48,
			},
		},
	}

	var buf bytes.Buffer
	if err := validateMetricsFromRoot(&buf, metrics, "2026-01-20", root); err != nil {
		t.Errorf("Validation failed with realistic variations: %v", err)
	}
	if !strings.Contains(buf.String(), "ALL VALIDATIONS PASSED") {
		t.Error("Expected output to contain 'ALL VALIDATIONS PASSED'")
	}
}
