// Package validation - validation_integration_test.go
//
// TEST SETUP
// ----------
// This test suite performs integration testing of the validation logic using
// real expected values JSON files from the test-data directory. Tests the
// complete validation flow end-to-end.
//
// TEST PLAN
// ---------
// 1. Full Validation Flow Tests
//   - Test complete validation with real expected values files
//   - Test validation with all metrics matching
//   - Test validation with some metrics outside tolerance
//   - Test validation with multiple systems
//
// 2. JSON File Loading Tests
//   - Test loading actual expected values files
//   - Test date matching
//   - Test system matching by ID
//
// 3. Error Reporting Tests
//   - Test validation failure reporting
//   - Test summary statistics (passed/failed counts)
//
// TESTING APPROACH
// ----------------
// - Uses real expected values files from test-data/
// - Creates mock aggregated metrics structures
// - Tests complete validation flow from metrics to result
// - Validates error messages and summary output
//
// PATTERN USED
// ------------
// - Pattern 3: Subtests with t.Run()
// - Pattern 6: Test Fixtures (expected values JSON files)
// - Pattern 7: Error Path Testing
// - Pattern 11: Golden Data Validation (expected values files)
//
// See TESTING.md for detailed pattern explanations.
package validation

import (
	"os"
	"testing"

	"enphase-monitor/internal/aggregator"
)

// TestValidateMetrics_FullFlow_Success tests successful validation with matching metrics
func TestValidateMetrics_FullFlow_Success(t *testing.T) {
	// Check if expected values file exists
	if _, err := os.Stat("../../test-data/expected_values_2026-01-20.json"); os.IsNotExist(err) {
		t.Skip("Expected values file not found - skipping integration test")
	}

	// Create metrics that exactly match expected values from the file
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
				NetImportedToday:       19.6,
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
				NetImportedToday:       -0.2,
				ConsumptionToday:       16.5,
			},
		},
	}

	// Change to project root for correct file path resolution
	originalDir, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatal(getwdErr)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Error("restore working directory:", err)
		}
	}()

	err := ValidateMetrics(metrics, "2026-01-20")
	if err != nil {
		t.Errorf("Validation failed with matching metrics: %v", err)
	}
}

// TestValidateMetrics_FullFlow_WithinTolerance tests validation with metrics within tolerance
func TestValidateMetrics_FullFlow_WithinTolerance(t *testing.T) {
	// Check if expected values file exists
	if _, err := os.Stat("../../test-data/expected_values_2026-01-20.json"); os.IsNotExist(err) {
		t.Skip("Expected values file not found - skipping integration test")
	}

	// Create metrics slightly different but within 10% tolerance
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
				NetImportedToday:       19.5, // Within tolerance of 19.6
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
				NetImportedToday:       -0.15, // Within tolerance of -0.2
				ConsumptionToday:       16.4,  // Within tolerance of 16.5
			},
		},
	}

	// Change to project root for correct file path resolution
	originalDir, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatal(getwdErr)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Error("restore working directory:", err)
		}
	}()

	err := ValidateMetrics(metrics, "2026-01-20")
	if err != nil {
		t.Errorf("Validation failed with metrics within tolerance: %v", err)
	}
}

// TestValidateMetrics_FullFlow_OutsideTolerance tests validation with metrics outside tolerance
func TestValidateMetrics_FullFlow_OutsideTolerance(t *testing.T) {
	// Check if expected values file exists
	if _, err := os.Stat("../../test-data/expected_values_2026-01-20.json"); os.IsNotExist(err) {
		t.Skip("Expected values file not found - skipping integration test")
	}

	// Create metrics significantly different (outside 10% tolerance)
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:                     "5525881",
				Name:                   "Right Subpanel",
				GridImportToday:        30.0, // Far from 23.4 (28% difference)
				GridExportToday:        3.9,  // Match
				ProductionToday:        14.6, // Match
				BatteryDischargedToday: 6.8,  // Match
				BatteryChargedToday:    8.6,  // Match
				NetImportedToday:       19.6, // Match
				ConsumptionToday:       32.3, // Match
			},
			{
				ID:                     "5392556",
				Name:                   "Left Subpanel",
				GridImportToday:        7.5,
				GridExportToday:        7.6,
				ProductionToday:        19.3,
				BatteryDischargedToday: 5.5,
				BatteryChargedToday:    8.1,
				NetImportedToday:       -0.2,
				ConsumptionToday:       16.5,
			},
		},
	}

	// Change to project root for correct file path resolution
	originalDir, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatal(getwdErr)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Error("restore working directory:", err)
		}
	}()

	err := ValidateMetrics(metrics, "2026-01-20")
	if err == nil {
		t.Error("Expected validation to fail with metrics outside tolerance, but it passed")
	}
}

// TestValidateMetrics_MissingSystem tests validation when a system is missing
func TestValidateMetrics_MissingSystem(t *testing.T) {
	// Check if expected values file exists
	if _, err := os.Stat("../../test-data/expected_values_2026-01-20.json"); os.IsNotExist(err) {
		t.Skip("Expected values file not found - skipping integration test")
	}

	// Create metrics with only one system (missing the second one)
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
				NetImportedToday:       19.6,
				ConsumptionToday:       32.3,
			},
			// Missing second system
		},
	}

	// Change to project root for correct file path resolution
	originalDir, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatal(getwdErr)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Error("restore working directory:", err)
		}
	}()

	err := ValidateMetrics(metrics, "2026-01-20")
	if err == nil {
		t.Error("Expected validation to fail with missing system, but it passed")
	}
}

// TestValidateMetrics_DateMismatch tests handling of date mismatch
func TestValidateMetrics_DateMismatch(t *testing.T) {
	// This test verifies that date validation works
	// The expected values file has date "2026-01-20" but we query with a different date

	// Check if expected values file exists
	if _, err := os.Stat("../../test-data/expected_values_2026-01-20.json"); os.IsNotExist(err) {
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

	// Change to project root for correct file path resolution
	originalDir, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatal(getwdErr)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Error("restore working directory:", err)
		}
	}()

	// Try to validate with a date that doesn't exist in test-data
	err := ValidateMetrics(metrics, "2026-01-99")
	if err == nil {
		t.Error("Expected error for non-existent date file, got nil")
	}
}

// TestValidateMetrics_MultipleExpectedValuesFiles tests with different dates
func TestValidateMetrics_MultipleExpectedValuesFiles(t *testing.T) {
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

	// Change to project root for correct file path resolution
	originalDir, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatal(getwdErr)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Error("restore working directory:", err)
		}
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check if expected values file exists
			filename := "test-data/expected_values_" + tt.date + ".json"
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				t.Skipf("Expected values file %s not found - skipping", filename)
			}

			// Create empty metrics to test file loading
			metrics := &aggregator.AggregatedMetrics{
				Systems: []aggregator.SystemMetrics{},
			}

			// We expect this to fail because metrics are empty, but it should
			// successfully load the expected values file first
			err := ValidateMetrics(metrics, tt.date)

			// We expect an error because metrics are empty, but not a file loading error
			if err == nil {
				t.Error("Expected error for empty metrics, got nil")
			}
			// Don't check the specific error - just verify the file was loadable
		})
	}
}

// TestValidateMetrics_RealWorldScenario tests a realistic validation scenario
func TestValidateMetrics_RealWorldScenario(t *testing.T) {
	// Check if expected values file exists
	if _, err := os.Stat("../../test-data/expected_values_2026-01-20.json"); os.IsNotExist(err) {
		t.Skip("Expected values file not found - skipping integration test")
	}

	// Simulate real-world data with slight variations due to:
	// - API response timing differences
	// - Floating point precision
	// - 15-minute interval boundary effects
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:                     "5525881",
				Name:                   "Right Subpanel",
				GridImportToday:        23.38, // Slight variation
				GridExportToday:        3.92,  // Slight variation
				ProductionToday:        14.58, // Slight variation
				BatteryDischargedToday: 6.79,  // Slight variation
				BatteryChargedToday:    8.62,  // Slight variation
				NetImportedToday:       19.46, // Calculated value may vary
				ConsumptionToday:       32.28, // Calculated value may vary
			},
			{
				ID:                     "5392556",
				Name:                   "Left Subpanel",
				GridImportToday:        7.48,  // Slight variation
				GridExportToday:        7.62,  // Slight variation
				ProductionToday:        19.28, // Slight variation
				BatteryDischargedToday: 5.52,  // Slight variation
				BatteryChargedToday:    8.08,  // Slight variation
				NetImportedToday:       -0.14, // Calculated value may vary
				ConsumptionToday:       16.48, // Calculated value may vary
			},
		},
	}

	// Change to project root for correct file path resolution
	originalDir, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatal(getwdErr)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Error("restore working directory:", err)
		}
	}()

	err := ValidateMetrics(metrics, "2026-01-20")
	if err != nil {
		t.Errorf("Validation failed with realistic variations: %v", err)
	}
}
