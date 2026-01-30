// Package validation - validation.go
//
// PURPOSE
// -------
// This package provides validation logic for comparing actual metrics against expected values.
// Used in test mode (--test flag) to verify system metrics match expected values from JSON files.
//
// VALIDATION APPROACH
// -------------------
// Loads expected values from JSON files in test-data/ directory, compares with actual metrics,
// and applies tolerance-based comparison (±10% or minimum 0.1 kWh).
//
// TOLERANCE LOGIC
// ---------------
// For each metric, the validation allows a tolerance of:
//   - ±10% of the expected value
//   - Minimum tolerance: ±0.1 kWh (for small values)
//
// This accounts for minor variations in API responses, floating-point precision, and timing differences.
package validation

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/constants"
)

// ExpectedValues represents the structure of expected values JSON files
type ExpectedValues struct {
	Date    string          `json:"date"`
	Systems []ExpectedSystem `json:"systems"`
}

// ExpectedSystem represents expected metrics for a single system
type ExpectedSystem struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Expected ExpectedMetrics `json:"expected"`
}

// ExpectedMetrics holds the expected values for system metrics
type ExpectedMetrics struct {
	GridImport         float64 `json:"grid_import"`
	GridExport         float64 `json:"grid_export"`
	Production         float64 `json:"production"`
	BatteryDischarged  float64 `json:"battery_discharged"`
	BatteryCharged     float64 `json:"battery_charged"`
	NetImported        float64 `json:"net_imported"`
	Consumption        float64 `json:"consumption"`
}

// ValidateMetrics validates actual metrics against expected values for the given date
func ValidateMetrics(metrics *aggregator.AggregatedMetrics, dateStr string) error {
	// Load expected values
	expectedPath := filepath.Join("test-data", fmt.Sprintf("expected_values_%s.json", dateStr))
	expectedData, err := os.ReadFile(expectedPath)
	if err != nil {
		return fmt.Errorf("failed to read expected values file: %w", err)
	}

	var expected ExpectedValues
	if err := json.Unmarshal(expectedData, &expected); err != nil {
		return fmt.Errorf("failed to parse expected values: %w", err)
	}

	// Validate date matches
	if expected.Date != dateStr {
		return fmt.Errorf("date mismatch: expected %s, got %s", expected.Date, dateStr)
	}

	// Display validation header
	fmt.Printf("\n%s\n", constants.Bold+"=== VALIDATION RESULTS ==="+constants.Reset)
	fmt.Printf("Comparing against expected values for %s\n\n", dateStr)

	allPassed := true
	totalTests := 0
	passedTests := 0

	// Validate each system
	for _, expectedSys := range expected.Systems {
		// Find matching system in actual metrics
		var actualSys *aggregator.SystemMetrics
		for i := range metrics.Systems {
			if metrics.Systems[i].ID == expectedSys.ID {
				actualSys = &metrics.Systems[i]
				break
			}
		}

		if actualSys == nil {
			fmt.Printf("❌ System %s (%s) not found in actual metrics\n", expectedSys.Name, expectedSys.ID)
			allPassed = false
			continue
		}

		fmt.Printf("%s[%s] %s (ID: %s)%s\n", constants.Bold, actualSys.Name, expectedSys.Name, expectedSys.ID, constants.Reset)

		// Validate individual metrics
		tests := []struct {
			name     string
			expected float64
			actual   float64
		}{
			{"Grid Import", expectedSys.Expected.GridImport, actualSys.GridImportToday},
			{"Grid Export", expectedSys.Expected.GridExport, actualSys.GridExportToday},
			{"Production", expectedSys.Expected.Production, actualSys.ProductionToday},
			{"Battery Discharged", expectedSys.Expected.BatteryDischarged, actualSys.BatteryDischargedToday},
			{"Battery Charged", expectedSys.Expected.BatteryCharged, actualSys.BatteryChargedToday},
			{"Net Imported", expectedSys.Expected.NetImported, actualSys.NetImportedToday},
			{"Consumption", expectedSys.Expected.Consumption, actualSys.ConsumptionToday},
		}

		for _, test := range tests {
			totalTests++
			passed := validateMetric(test.name, test.expected, test.actual)
			if passed {
				passedTests++
			} else {
				allPassed = false
			}
		}
		fmt.Println()
	}

	// Print summary
	fmt.Printf("%s\n", constants.Bold+"=== VALIDATION SUMMARY ==="+constants.Reset)
	fmt.Printf("Total tests: %d\n", totalTests)
	fmt.Printf("Passed: %d\n", passedTests)
	fmt.Printf("Failed: %d\n", totalTests-passedTests)

	if allPassed {
		fmt.Printf("\n%s✅ ALL VALIDATIONS PASSED%s\n", constants.Bold, constants.Reset)
		return nil
	}

	fmt.Printf("\n%s❌ SOME VALIDATIONS FAILED%s\n", constants.Bold, constants.Reset)
	return fmt.Errorf("%d/%d validation tests failed", totalTests-passedTests, totalTests)
}

// validateMetric validates a single metric value against expected with tolerance
func validateMetric(name string, expected, actual float64) bool {
	// Calculate tolerance: 10% of expected value, with minimum 0.1 kWh
	tolerance := math.Max(math.Abs(expected)*constants.ValidationTolerancePercent, constants.ValidationMinToleranceKWh)
	diff := actual - expected
	absDiff := math.Abs(diff)
	
	// Calculate percentage difference
	var percentDiff float64
	if expected == 0 {
		percentDiff = constants.ValidationInfinitePercent // Use special value for division by zero
	} else {
		percentDiff = (absDiff / math.Abs(expected)) * 100
	}

	passed := absDiff <= tolerance

	// Format output
	status := "✅"
	if !passed {
		status = "❌"
	}

	// Show percentage only if it's meaningful (> 0.5%)
	percentStr := ""
	if percentDiff >= constants.ValidationPercentThreshold && percentDiff != constants.ValidationInfinitePercent {
		percentStr = fmt.Sprintf(" (%.1f%%)", percentDiff)
	}

	fmt.Printf("  %s %-20s Expected: %6.1f kWh  Actual: %6.1f kWh  Diff: %+6.1f kWh%s\n",
		status, name+":", expected, actual, diff, percentStr)

	return passed
}
