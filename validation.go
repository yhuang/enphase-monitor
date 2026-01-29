package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Validation tolerance constants
const (
	// tolerancePercent is the acceptable deviation from expected value (10%)
	tolerancePercent = 0.10
	// minToleranceKWh is the minimum tolerance in kWh for small values
	minToleranceKWh = 0.1
)

// ExpectedValues represents the expected values for a test date
type ExpectedValues struct {
	Date    string `json:"date"`
	Systems []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Expected struct {
			GridImport        float64 `json:"grid_import"`
			GridExport        float64 `json:"grid_export"`
			Production        float64 `json:"production"`
			BatteryDischarged float64 `json:"battery_discharged"`
			BatteryCharged    float64 `json:"battery_charged"`
			NetImported       float64 `json:"net_imported"`
			Consumption       float64 `json:"consumption"`
		} `json:"expected"`
	} `json:"systems"`
}

// ValidateMetrics compares actual metrics against expected values
func ValidateMetrics(metrics *AggregatedMetrics, testDate string) error {
	// Load expected values
	expectedFile := filepath.Join("test-data", fmt.Sprintf("expected_values_%s.json", testDate))
	data, err := os.ReadFile(expectedFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("expected values file not found: %s (use --date to specify the test date)", expectedFile)
		}
		return fmt.Errorf("failed to load expected values: %w", err)
	}

	var expected ExpectedValues
	if err := json.Unmarshal(data, &expected); err != nil {
		return fmt.Errorf("failed to parse expected values: %w", err)
	}

	fmt.Println("\n=== VALIDATION RESULTS ===")
	fmt.Println()

	// Note: All validation values are always displayed, regardless of pass/fail status
	allPassed := true
	for _, expectedSys := range expected.Systems {
		// Find matching system in actual metrics
		var actualSys *SystemMetrics
		for i := range metrics.Systems {
			if metrics.Systems[i].ID == expectedSys.ID {
				actualSys = &metrics.Systems[i]
				break
			}
		}

		if actualSys == nil {
			fmt.Printf("❌ System %s (%s): NOT FOUND in actual metrics\n", expectedSys.Name, expectedSys.ID)
			allPassed = false
			continue
		}

		fmt.Printf("System: %s (%s)\n", expectedSys.Name, expectedSys.ID)
		// Headers: Metric (left-aligned), then all others right-aligned to match values
		// Format must match data format exactly: %-25s %10s    %10s    %7s %12s     %s
		fmt.Printf("  %-25s %10s    %10s    %7s %12s     %s\n", "Metric", "Expected", "Actual", "Diff", "(Pct)", "Status")
		fmt.Println("  ------------------------------------------------------------------------------------")

		// Validate each metric
		// Tolerance: ±10% of expected value is acceptable
		// For values near 0, use a minimum tolerance of 0.1 kWh
		// Note: All metric values are always printed, regardless of pass/fail status
		validateMetric := func(name string, expected, actual float64) bool {
			diff := actual - expected
			status := "✅"
			// Calculate tolerance with minimum for small values
			tolerance := math.Abs(expected) * tolerancePercent
			if tolerance < minToleranceKWh {
				tolerance = minToleranceKWh
			}
			if math.Abs(diff) > tolerance {
				status = "❌"
				allPassed = false
			}
			// Calculate percentage difference
			var percentDiff float64
			if expected != 0 {
				percentDiff = (diff / math.Abs(expected)) * 100.0
			} else if diff != 0 {
				// If expected is 0 but actual is not, percentage is infinite/undefined
				// Use a large number to indicate significant difference
				percentDiff = 999.9
			} else {
				percentDiff = 0.0
			}
			// Format with right-aligned columns:
			// Metric name: 25 chars left-aligned
			// Expected: 10 chars right-aligned
			// Actual: 10 chars right-aligned
			// Diff: 7 chars right-aligned with sign
			// Percentage: format as string with fixed width (12 chars) for right alignment, whole numbers only
			// If percentage rounds to zero, do not show the sign
			var percentStr string
			if math.Abs(percentDiff) < 0.5 {
				percentStr = fmt.Sprintf("(0%%)")
			} else {
				percentStr = fmt.Sprintf("(%+.0f%%)", percentDiff)
			}
			// Status: 1 emoji with 5 spaces padding
			// Always print the metric values, even when tests pass
			fmt.Printf("  %-25s %10.1f    %10.1f    %+7.1f %12s     %s\n", name, expected, actual, diff, percentStr, status)
			return status == "✅"
		}

		validateMetric("Grid Import", expectedSys.Expected.GridImport, actualSys.GridImportToday)
		validateMetric("Grid Export", expectedSys.Expected.GridExport, actualSys.GridExportToday)
		validateMetric("Production", expectedSys.Expected.Production, actualSys.ProductionToday)
		validateMetric("Battery Discharged", expectedSys.Expected.BatteryDischarged, actualSys.BatteryDischargedToday)
		validateMetric("Battery Charged", expectedSys.Expected.BatteryCharged, actualSys.BatteryChargedToday)
		// Check if net imported is negative (net exported)
		if actualSys.NetImportedToday < 0 {
			// Net exported - show as "Net Energy Flow (export)" and compare absolute values
			// Expected might be negative (net export) or positive (should be net import but actual is export)
			expectedValue := expectedSys.Expected.NetImported
			if expectedValue < 0 {
				// Expected is also negative (net export), compare absolute values
				validateMetric("Net Energy Flow (export)", math.Abs(expectedValue), math.Abs(actualSys.NetImportedToday))
			} else {
				// Expected is positive but actual is negative - compare absolute values
				// This indicates a mismatch (expected import but got export)
				validateMetric("Net Energy Flow (export)", expectedValue, math.Abs(actualSys.NetImportedToday))
			}
		} else {
			validateMetric("Net Energy Flow (import)", expectedSys.Expected.NetImported, actualSys.NetImportedToday)
		}
		validateMetric("Consumption", expectedSys.Expected.Consumption, actualSys.ConsumptionToday)
		fmt.Println()
	}

	if allPassed {
		fmt.Println("✅ ALL VALIDATIONS PASSED")
		return nil
	}
	fmt.Println("❌ SOME VALIDATIONS FAILED")
	return fmt.Errorf("validation failed")
}

