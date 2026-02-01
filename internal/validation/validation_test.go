// Package validation - validation_test.go
//
// TEST FILE WALKTHROUGH
// =====================
// This file demonstrates several Go testing patterns. Each test function below
// includes detailed comments explaining what the code does and why.
//
// PATTERNS DEMONSTRATED
// ---------------------
// - Pattern 1: Table-Driven Tests (multiple inputs, one test function)
// - Pattern 3: Subtests with t.Run() (named, independently runnable tests)
// - Pattern 7: Error Path Testing (deliberately cause errors to test handling)
//
// HOW TO RUN THESE TESTS
// ----------------------
//
//	go test -v ./internal/validation/                    # Run all tests in package
//	go test -v ./internal/validation/ -run ExactMatch    # Run tests matching "ExactMatch"
//	go test -v ./internal/validation/ -run "WithinTolerance/small"  # Run specific subtest
//
// UNDERSTANDING TEST OUTPUT
// -------------------------
// When you run `go test -v`, you'll see:
//
//	=== RUN   TestValidateMetric_ExactMatch           <- Test function started
//	=== RUN   TestValidateMetric_ExactMatch/zero_values  <- Subtest started
//	--- PASS: TestValidateMetric_ExactMatch/zero_values  <- Subtest passed
//	--- PASS: TestValidateMetric_ExactMatch (0.00s)      <- All subtests passed
//
// See TESTING.md for detailed pattern explanations.
package validation

import (
	"math"
	"testing"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/constants"
)

// =============================================================================
// PATTERN 1: TABLE-DRIVEN TESTS
// =============================================================================
//
// Table-driven tests let you test the same logic with many different inputs.
// Instead of writing 4 separate test functions, you define test cases in a
// "table" (slice of structs) and loop through them.
//
// BENEFITS:
// - Add new test cases by adding one line (no new functions needed)
// - All cases use identical test logic (DRY - Don't Repeat Yourself)
// - Easy to see all inputs/outputs at a glance
// - Less code to maintain

// TestValidateMetric_ExactMatch tests validation with exact matches.
//
// WALKTHROUGH:
// This test verifies that validateMetric returns true when expected == actual.
// It tests several edge cases: zero, positive, negative, and large values.
func TestValidateMetric_ExactMatch(t *testing.T) {
	// STEP 1: Define the test table
	// Each struct in this slice represents one test case.
	// The struct fields are:
	//   - name: human-readable description (used in test output)
	//   - expected: first input to validateMetric
	//   - actual: second input to validateMetric
	//   - want: what we expect validateMetric to return
	tests := []struct {
		name     string
		expected float64
		actual   float64
		want     bool
	}{
		// Each line is a complete test case - easy to add more!
		{"zero values", 0.0, 0.0, true},
		{"positive match", 10.5, 10.5, true},
		{"negative match", -5.2, -5.2, true},
		{"large value match", 1234.5, 1234.5, true},
	}

	// STEP 2: Loop through each test case
	// The `tt` variable holds the current test case (convention: "tt" for "table test")
	for _, tt := range tests {
		// STEP 3: Create a subtest for each case using t.Run()
		// This gives each case its own name in the output, and allows running
		// individual cases with: go test -run "ExactMatch/zero_values"
		t.Run(tt.name, func(t *testing.T) {
			// STEP 4: Call the function we're testing
			// validateMetric is defined in validation.go (same package, so we can call it)
			got := validateMetric("Test Metric", tt.expected, tt.actual)

			// STEP 5: Compare the result with what we expected
			// If they don't match, report an error with helpful context
			if got != tt.want {
				t.Errorf("validateMetric() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// TABLE-DRIVEN TEST WITH RICHER ERROR MESSAGES
// =============================================================================
//
// This test shows how to add extra fields to your test struct for better
// debugging when tests fail.

// TestValidateMetric_WithinTolerance tests validation within tolerance bounds.
//
// WALKTHROUGH:
// This test verifies the tolerance logic: values within ±10% (or ±0.1 kWh minimum)
// should pass validation. The test includes a "description" field that explains
// WHY each case should pass, making failures easier to debug.
func TestValidateMetric_WithinTolerance(t *testing.T) {
	// Notice the extra "description" field - this helps when debugging failures
	tests := []struct {
		name        string
		expected    float64
		actual      float64
		want        bool
		description string // Extra context for debugging
	}{
		{
			name:        "within 10% tolerance - positive",
			expected:    100.0,
			actual:      109.0, // 9% difference
			want:        true,
			description: "9% is within 10% tolerance",
		},
		{
			name:        "within 10% tolerance - negative",
			expected:    100.0,
			actual:      91.0, // 9% difference
			want:        true,
			description: "9% is within 10% tolerance",
		},
		{
			name:        "at 10% tolerance boundary",
			expected:    100.0,
			actual:      110.0, // exactly 10%
			want:        true,
			description: "10% is at the tolerance boundary",
		},
		{
			name:        "small value uses minimum tolerance",
			expected:    0.5,
			actual:      0.6, // 20% but within 0.1 kWh minimum
			want:        true,
			description: "0.1 kWh minimum tolerance applies",
		},
		{
			name:        "small value at minimum tolerance boundary",
			expected:    0.5,
			actual:      0.6, // exactly 0.1 kWh difference
			want:        true,
			description: "At minimum tolerance boundary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateMetric("Test Metric", tt.expected, tt.actual)
			if got != tt.want {
				// RICH ERROR MESSAGE: When test fails, show all relevant info
				// This makes debugging much easier - you can see exactly what went wrong
				diff := math.Abs(tt.actual - tt.expected)
				tolerance := math.Max(math.Abs(tt.expected)*constants.ValidationTolerancePercent, constants.ValidationMinToleranceKWh)
				t.Errorf("validateMetric() = %v, want %v\nExpected: %.2f, Actual: %.2f, Diff: %.2f, Tolerance: %.2f\n%s",
					got, tt.want, tt.expected, tt.actual, diff, tolerance, tt.description)
			}
		})
	}
}

// TestValidateMetric_OutsideTolerance tests validation outside tolerance bounds.
//
// WALKTHROUGH:
// This test verifies that values OUTSIDE the tolerance correctly return false.
// It's the "negative case" counterpart to TestValidateMetric_WithinTolerance.
func TestValidateMetric_OutsideTolerance(t *testing.T) {
	tests := []struct {
		name        string
		expected    float64
		actual      float64
		want        bool
		description string
	}{
		{
			name:        "outside 10% tolerance - high",
			expected:    100.0,
			actual:      111.0, // 11% difference
			want:        false, // Should FAIL validation
			description: "11% exceeds 10% tolerance",
		},
		{
			name:        "outside 10% tolerance - low",
			expected:    100.0,
			actual:      89.0, // 11% difference
			want:        false,
			description: "11% exceeds 10% tolerance",
		},
		{
			name:        "small value outside minimum tolerance",
			expected:    0.5,
			actual:      0.65, // 0.15 kWh difference (> 0.1 minimum)
			want:        false,
			description: "Exceeds minimum tolerance of 0.1 kWh",
		},
		{
			name:        "large difference",
			expected:    100.0,
			actual:      200.0, // 100% difference
			want:        false,
			description: "100% difference far exceeds tolerance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateMetric("Test Metric", tt.expected, tt.actual)
			if got != tt.want {
				diff := math.Abs(tt.actual - tt.expected)
				tolerance := math.Max(math.Abs(tt.expected)*constants.ValidationTolerancePercent, constants.ValidationMinToleranceKWh)
				t.Errorf("validateMetric() = %v, want %v\nExpected: %.2f, Actual: %.2f, Diff: %.2f, Tolerance: %.2f\n%s",
					got, tt.want, tt.expected, tt.actual, diff, tolerance, tt.description)
			}
		})
	}
}

// =============================================================================
// EDGE CASE TESTING
// =============================================================================
//
// Edge cases are inputs at the boundaries of what a function handles:
// - Zero values (division by zero concerns)
// - Negative values (sign handling)
// - Boundary conditions (exactly at tolerance limit)

// TestValidateMetric_ZeroValues tests validation with zero expected values.
//
// WALKTHROUGH:
// Zero is a special case because:
// 1. 10% of zero is zero (not useful as a tolerance)
// 2. We need the minimum tolerance (0.1 kWh) to kick in
func TestValidateMetric_ZeroValues(t *testing.T) {
	tests := []struct {
		name     string
		expected float64
		actual   float64
		want     bool
	}{
		{
			name:     "both zero",
			expected: 0.0,
			actual:   0.0,
			want:     true, // 0 == 0, obviously passes
		},
		{
			name:     "zero expected, small actual (within min tolerance)",
			expected: 0.0,
			actual:   0.1,  // at minimum tolerance boundary
			want:     true, // 0.1 kWh diff <= 0.1 kWh tolerance
		},
		{
			name:     "zero expected, actual outside min tolerance",
			expected: 0.0,
			actual:   0.2,   // exceeds 0.1 kWh minimum tolerance
			want:     false, // 0.2 kWh diff > 0.1 kWh tolerance
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateMetric("Test Metric", tt.expected, tt.actual)
			if got != tt.want {
				t.Errorf("validateMetric() = %v, want %v (expected: %.2f, actual: %.2f)",
					got, tt.want, tt.expected, tt.actual)
			}
		})
	}
}

// TestValidateMetric_NegativeValues tests validation with negative values.
//
// WALKTHROUGH:
// Negative values test sign handling - the tolerance should use absolute values.
func TestValidateMetric_NegativeValues(t *testing.T) {
	tests := []struct {
		name     string
		expected float64
		actual   float64
		want     bool
	}{
		{
			name:     "negative within tolerance",
			expected: -10.0,
			actual:   -10.9, // 9% difference (absolute)
			want:     true,
		},
		{
			name:     "negative outside tolerance",
			expected: -10.0,
			actual:   -11.5, // 15% difference
			want:     false,
		},
		{
			name:     "negative to positive crossing zero (outside tolerance)",
			expected: -0.5,
			actual:   0.5, // 1.0 kWh difference, exceeds 0.1 kWh minimum tolerance
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateMetric("Test Metric", tt.expected, tt.actual)
			if got != tt.want {
				t.Errorf("validateMetric() = %v, want %v (expected: %.2f, actual: %.2f)",
					got, tt.want, tt.expected, tt.actual)
			}
		})
	}
}

// =============================================================================
// PATTERN 7: ERROR PATH TESTING
// =============================================================================
//
// Error path tests deliberately cause errors to verify the code handles them
// correctly. This ensures:
// - The function returns an error (not nil)
// - The error message is helpful
// - No panics or crashes occur
//
// WHY TEST ERRORS?
// Most bugs in production come from unhandled error cases. Testing error paths
// ensures your code fails gracefully.

// TestValidateMetrics_MissingFile tests handling of missing expected values file.
//
// WALKTHROUGH:
// This test passes an impossible date ("9999-99-99") that will never have
// an expected values file. We verify that ValidateMetrics returns an error
// instead of crashing or returning nil.
func TestValidateMetrics_MissingFile(t *testing.T) {
	// STEP 1: Create valid input data
	// The metrics struct is valid - only the date is impossible
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:               "test-system",
				Name:             "Test System",
				ProductionToday:  10.0,
				ConsumptionToday: 8.0,
				GridImportToday:  5.0,
				GridExportToday:  2.0,
				NetImportedToday: 3.0,
			},
		},
	}

	// STEP 2: Call function with input that SHOULD cause an error
	err := ValidateMetrics(metrics, "9999-99-99") // Non-existent date

	// STEP 3: Verify we got an error (not nil)
	if err == nil {
		t.Error("Expected error for missing file, got nil")
		// Note: We use t.Error (not t.Fatal) because there's no more code after this
	}
}

// =============================================================================
// TESTING ERROR MESSAGE QUALITY
// =============================================================================
//
// It's not enough to just return an error - the error message should be
// HELPFUL. It should tell the user:
// - What went wrong
// - What they can do to fix it
// - Any relevant context (file paths, dates, etc.)

// TestValidateMetrics_MissingFile_HelpfulError tests that missing file error is helpful.
//
// WALKTHROUGH:
// This test verifies that when the expected values file is missing, the error
// message contains all the information a user needs to fix the problem.
func TestValidateMetrics_MissingFile_HelpfulError(t *testing.T) {
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{ID: "test-system", Name: "Test System"},
		},
	}

	testDate := "2050-06-15" // Date that definitely won't exist
	err := ValidateMetrics(metrics, testDate)

	// STEP 1: First verify we got an error at all
	if err == nil {
		t.Fatal("Expected error for missing file, got nil")
		// t.Fatal STOPS the test immediately - no point checking error message if there's no error
	}

	// STEP 2: Get the error message as a string for checking
	errMsg := err.Error()

	// STEP 3: Use subtests to check MULTIPLE aspects of the error message
	// Each aspect is its own subtest, so you can see exactly which checks failed

	t.Run("contains target date", func(t *testing.T) {
		// The error should mention which date we were looking for
		if !containsString(errMsg, testDate) {
			t.Errorf("Error message should contain date %q", testDate)
		}
	})

	t.Run("contains file path", func(t *testing.T) {
		// The error should tell the user WHERE the file should be
		if !containsString(errMsg, "test-data/expected_values_") {
			t.Error("Error message should contain expected file path")
		}
	})

	t.Run("contains JSON format example", func(t *testing.T) {
		// The error should show the user what format the file needs
		if !containsString(errMsg, "\"date\":") {
			t.Error("Error message should contain JSON format example")
		}
		if !containsString(errMsg, "\"systems\":") {
			t.Error("Error message should contain systems field in example")
		}
		if !containsString(errMsg, "\"expected\":") {
			t.Error("Error message should contain expected field in example")
		}
	})

	t.Run("contains skip validation hint", func(t *testing.T) {
		// The error should tell the user how to skip validation if they don't want it
		if !containsString(errMsg, "skip validation") || !containsString(errMsg, "omit the --test flag") {
			t.Error("Error message should contain hint about skipping validation")
		}
	})
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================
//
// Helper functions keep test code DRY (Don't Repeat Yourself).
// This helper is used by multiple tests above.

// containsString is a helper for string containment check.
// We could use strings.Contains, but this avoids adding an import.
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// =============================================================================
// SKIPPING TESTS
// =============================================================================
//
// Sometimes you want to write a test but can't run it yet (missing fixtures,
// external dependencies, etc.). Use t.Skip() to mark it as skipped.

// TestValidateMetrics_InvalidJSON tests handling of malformed JSON.
//
// WALKTHROUGH:
// This test is SKIPPED because it would require creating a malformed JSON file
// in the test-data/ directory. The test is here to document that this case
// SHOULD be tested, and to make it easy to implement later.
func TestValidateMetrics_InvalidJSON(t *testing.T) {
	// t.Skip() marks this test as skipped (not failed)
	// The message explains WHY it's skipped
	t.Skip("Requires test fixture with malformed JSON")

	// When someone creates the fixture, they can remove t.Skip()
	// and implement the test logic here
}

// =============================================================================
// TESTING INTERNAL CALCULATIONS
// =============================================================================
//
// Sometimes you want to test a calculation directly, not through the function
// that uses it. This makes tests more focused and failures easier to diagnose.

// TestToleranceCalculation tests the tolerance calculation logic.
//
// WALKTHROUGH:
// This test verifies the tolerance formula directly:
//
//	tolerance = max(expected * 10%, 0.1 kWh)
//
// By testing the formula separately, we can verify the math is correct
// independent of the validateMetric function.
func TestToleranceCalculation(t *testing.T) {
	tests := []struct {
		name          string
		expected      float64
		wantTolerance float64
		description   string
	}{
		{
			name:          "large value uses percentage",
			expected:      100.0,
			wantTolerance: 10.0, // 10% of 100
			description:   "For large values, 10% tolerance applies",
		},
		{
			name:          "small value uses minimum",
			expected:      0.5,
			wantTolerance: 0.1, // Minimum tolerance
			description:   "For small values, minimum 0.1 kWh applies",
		},
		{
			name:          "zero value uses minimum",
			expected:      0.0,
			wantTolerance: 0.1, // Minimum tolerance
			description:   "For zero values, minimum 0.1 kWh applies",
		},
		{
			name:          "medium value at crossover point",
			expected:      1.0,
			wantTolerance: 0.1, // 10% of 1.0 = 0.1, equal to minimum
			description:   "At crossover point, both yield same tolerance",
		},
		{
			name:          "above crossover uses percentage",
			expected:      1.5,
			wantTolerance: 0.15, // 10% of 1.5 > 0.1 minimum
			description:   "Above crossover, percentage tolerance applies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// DIRECT CALCULATION: We're testing the formula, not the function
			got := math.Max(math.Abs(tt.expected)*constants.ValidationTolerancePercent, constants.ValidationMinToleranceKWh)

			// Allow small floating point error (0.001)
			if math.Abs(got-tt.wantTolerance) > 0.001 {
				t.Errorf("Tolerance calculation = %.3f, want %.3f\n%s",
					got, tt.wantTolerance, tt.description)
			}
		})
	}
}

// =============================================================================
// TESTING REFACTOR HELPERS (findSystemByID, runMetricTests)
// =============================================================================
//
// These helpers were introduced during go-style-core refactoring to keep
// ValidateMetrics nesting at most 2 levels. Tests ensure they behave correctly.

// TestFindSystemByID tests the findSystemByID helper.
func TestFindSystemByID(t *testing.T) {
	systems := []aggregator.SystemMetrics{
		{ID: "id-a", Name: "System A"},
		{ID: "id-b", Name: "System B"},
		{ID: "id-c", Name: "System C"},
	}

	tests := []struct {
		name    string
		systems []aggregator.SystemMetrics
		id      string
		wantNil bool
		wantID  string
	}{
		{"empty slice", nil, "id-a", true, ""},
		{"no match", systems, "id-x", true, ""},
		{"match first", systems, "id-a", false, "id-a"},
		{"match middle", systems, "id-b", false, "id-b"},
		{"match last", systems, "id-c", false, "id-c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findSystemByID(tt.systems, tt.id)
			if tt.wantNil {
				if got != nil {
					t.Errorf("findSystemByID() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("findSystemByID() = nil, want non-nil")
			}
			if got.ID != tt.wantID {
				t.Errorf("findSystemByID().ID = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

// TestRunMetricTests tests the runMetricTests helper.
// runMetricTests calls validateMetric which prints to stdout; we only assert return values.
func TestRunMetricTests(t *testing.T) {
	tests := []struct {
		name       string
		cases      []metricTestCase
		wantTotal  int
		wantPassed int
		wantFailed bool
	}{
		{"empty", nil, 0, 0, false},
		{"one pass", []metricTestCase{{"A", 10.0, 10.0}}, 1, 1, false},
		{"one fail", []metricTestCase{{"A", 10.0, 20.0}}, 1, 0, true},
		{"mixed", []metricTestCase{
			{"A", 10.0, 10.0},
			{"B", 5.0, 10.0},
			{"C", 0.0, 0.0},
		}, 3, 2, true},
		{"all pass", []metricTestCase{
			{"A", 10.0, 10.0},
			{"B", 1.0, 1.0},
		}, 2, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, passed, anyFailed := runMetricTests(tt.cases)
			if total != tt.wantTotal {
				t.Errorf("runMetricTests() total = %d, want %d", total, tt.wantTotal)
			}
			if passed != tt.wantPassed {
				t.Errorf("runMetricTests() passed = %d, want %d", passed, tt.wantPassed)
			}
			if anyFailed != tt.wantFailed {
				t.Errorf("runMetricTests() anyFailed = %v, want %v", anyFailed, tt.wantFailed)
			}
		})
	}
}

// =============================================================================
// INTEGRATION-STYLE TESTS
// =============================================================================
//
// These tests call the full ValidateMetrics function with realistic inputs.
// They test how multiple components work together.

// TestValidateMetrics_EmptyMetrics tests validation with empty metrics.
//
// WALKTHROUGH:
// This test verifies that ValidateMetrics handles the edge case where no
// systems are provided. The expected values file (if it exists) will have
// systems, but our metrics don't - this mismatch should cause an error.
func TestValidateMetrics_EmptyMetrics(t *testing.T) {
	// Create metrics with an EMPTY systems slice
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{}, // No systems!
	}

	// This should fail because expected values will have systems but metrics don't
	err := ValidateMetrics(metrics, "2026-01-20")
	if err == nil {
		t.Error("Expected error for empty metrics, got nil")
	}
}

// TestValidateMetrics_SystemIDMismatch tests handling of system ID mismatches.
//
// WALKTHROUGH:
// This test verifies that ValidateMetrics catches when the system IDs in
// the metrics don't match the system IDs in the expected values file.
func TestValidateMetrics_SystemIDMismatch(t *testing.T) {
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:   "wrong-id", // This ID won't match the expected values file
				Name: "Wrong System",
			},
		},
	}

	// This should fail because system IDs won't match
	err := ValidateMetrics(metrics, "2026-01-20")
	if err == nil {
		t.Error("Expected error for system ID mismatch, got nil")
	}
}
