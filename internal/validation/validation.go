// Package validation provides validation logic for comparing actual metrics against expected values.
//
// PURPOSE
// -------
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
//
// TESTABILITY
// -----------
// Functions accept an io.Writer parameter for output, following the Go idiom for testable I/O.
// Production code passes os.Stdout; tests pass bytes.Buffer to capture and verify output.
// This keeps test output clean and enables explicit assertions on validation results.
package validation

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/constants"
)

// ExpectedValues represents the structure of expected values JSON files.
type ExpectedValues struct {
	Date    string           `json:"date"`
	Systems []ExpectedSystem `json:"systems"`
}

// ExpectedSystem represents expected metrics for a single system.
type ExpectedSystem struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Expected ExpectedMetrics `json:"expected"`
}

// ExpectedMetrics holds the expected values for system metrics.
type ExpectedMetrics struct {
	GridImport        float64 `json:"grid_import"`
	GridExport        float64 `json:"grid_export"`
	Production        float64 `json:"production"`
	BatteryDischarged float64 `json:"battery_discharged"`
	BatteryCharged    float64 `json:"battery_charged"`
	NetFlow           float64 `json:"net_flow"`
	Consumption       float64 `json:"consumption"`
}

// ValidateMetrics validates actual metrics against expected values for the given date.
// Output is written to w, allowing tests to capture and verify the output.
func ValidateMetrics(w io.Writer, metrics *aggregator.AggregatedMetrics, dateStr string) error {
	// Load expected values
	expectedPath := filepath.Join("test-data", fmt.Sprintf("expected_values_%s.json", dateStr))
	expectedData, err := os.ReadFile(expectedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no expected values file found for %s\n\n"+
				"To run validation, create the file:\n"+
				"  %s\n\n"+
				"Example format:\n"+
				"  {\n"+
				"    \"date\": \"%s\",\n"+
				"    \"systems\": [\n"+
				"      {\n"+
				"        \"id\": \"SYSTEM_ID\",\n"+
				"        \"name\": \"System Name\",\n"+
				"        \"expected\": {\n"+
				"          \"grid_import\": 10.0,\n"+
				"          \"grid_export\": 5.0,\n"+
				"          \"production\": 20.0,\n"+
				"          \"battery_discharged\": 2.0,\n"+
				"          \"battery_charged\": 3.0,\n"+
				"          \"net_flow\": 5.0,\n"+
				"          \"consumption\": 15.0\n"+
				"        }\n"+
				"      }\n"+
				"    ]\n"+
				"  }\n\n"+
				"To skip validation and just use cache-only mode, omit the --test flag:\n"+
				"  ./enphase-monitor --date %s", dateStr, expectedPath, dateStr, dateStr)
		}
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
	fmt.Fprintf(w, "\n%s\n", constants.Bold+"=== VALIDATION RESULTS ==="+constants.Reset)
	fmt.Fprintf(w, "Comparing against expected values for %s\n\n", dateStr)

	allPassed := true
	totalTests := 0
	passedTests := 0

	// Validate each system (go-style-core: max 2 levels via helpers)
	for _, expectedSys := range expected.Systems {
		actualSys := findSystemByID(metrics.Systems, expectedSys.ID)
		if actualSys == nil {
			fmt.Fprintf(w, "❌ System %s (%s) not found in actual metrics\n", expectedSys.Name, expectedSys.ID)
			allPassed = false
			continue
		}

		fmt.Fprintf(w, "%s[%s] %s (ID: %s)%s\n", constants.Bold, actualSys.Name, expectedSys.Name, expectedSys.ID, constants.Reset)

		tests := []metricTestCase{
			{"Grid Import", expectedSys.Expected.GridImport, actualSys.GridImportToday},
			{"Grid Export", expectedSys.Expected.GridExport, actualSys.GridExportToday},
			{"Production", expectedSys.Expected.Production, actualSys.ProductionToday},
			{"Battery Discharged", expectedSys.Expected.BatteryDischarged, actualSys.BatteryDischargedToday},
			{"Battery Charged", expectedSys.Expected.BatteryCharged, actualSys.BatteryChargedToday},
			{"Net Flow", expectedSys.Expected.NetFlow, actualSys.NetFlowToday},
			{"Consumption", expectedSys.Expected.Consumption, actualSys.ConsumptionToday},
		}
		total, passed, anyFailed := runMetricTests(w, tests)
		totalTests += total
		passedTests += passed
		if anyFailed {
			allPassed = false
		}
		fmt.Fprintln(w)
	}

	// Print summary
	fmt.Fprintf(w, "%s\n", constants.Bold+"=== VALIDATION SUMMARY ==="+constants.Reset)
	fmt.Fprintf(w, "Total tests: %d\n", totalTests)
	fmt.Fprintf(w, "Passed: %d\n", passedTests)
	fmt.Fprintf(w, "Failed: %d\n", totalTests-passedTests)

	if allPassed {
		fmt.Fprintf(w, "\n%s✅ ALL VALIDATIONS PASSED%s\n", constants.Bold, constants.Reset)
		return nil
	}

	fmt.Fprintf(w, "\n%s❌ SOME VALIDATIONS FAILED%s\n", constants.Bold, constants.Reset)
	return fmt.Errorf("%d/%d validation tests failed", totalTests-passedTests, totalTests)
}

// findSystemByID returns the system in systems with the given ID, or nil (go-style-core: keeps nesting ≤2).
func findSystemByID(systems []aggregator.SystemMetrics, id string) *aggregator.SystemMetrics {
	for i := range systems {
		if systems[i].ID == id {
			return &systems[i]
		}
	}
	return nil
}

// metricTestCase is a single metric comparison for validation (used by runMetricTests).
type metricTestCase struct {
	name     string
	expected float64
	actual   float64
}

// runMetricTests runs the given metric tests and returns total count, passed count, and whether any failed.
func runMetricTests(w io.Writer, tests []metricTestCase) (total, passed int, anyFailed bool) {
	for _, test := range tests {
		total++
		p := validateMetric(w, test.name, test.expected, test.actual)
		if p {
			passed++
		}
		if !p {
			anyFailed = true
		}
	}
	return total, passed, anyFailed
}

// validateMetric validates a single metric value against expected with tolerance.
func validateMetric(w io.Writer, name string, expected, actual float64) bool {
	// Calculate tolerance: 10% of expected value, with minimum 0.1 kWh
	tolerance := math.Max(math.Abs(expected)*constants.ValidationTolerancePercent, constants.ValidationMinToleranceKWh)
	diff := actual - expected
	absDiff := math.Abs(diff)

	// Calculate percentage difference (default for division by zero)
	percentDiff := constants.ValidationInfinitePercent
	if expected != 0 {
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

	fmt.Fprintf(w, "  %s %-20s Expected: %6.1f kWh  Actual: %6.1f kWh  Diff: %+6.1f kWh%s\n",
		status, name+":", expected, actual, diff, percentStr)

	return passed
}
