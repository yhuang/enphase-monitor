package validation

import (
	"bytes"
	"io"
	"math"
	"strings"
	"testing"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/constants"
)

// TestValidateMetric_ExactMatch tests validation with exact matches.
func TestValidateMetric_ExactMatch(t *testing.T) {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateMetric(io.Discard, "Test Metric", tt.expected, tt.actual)

			if got != tt.want {
				t.Errorf("validateMetric() = %v, want %v", got, tt.want)
			}
		})
	}
}

//
// This test shows how to add extra fields to your test struct for better
// debugging when tests fail.

// TestValidateMetric_WithinTolerance tests validation within tolerance bounds.
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
			got := validateMetric(io.Discard, "Test Metric", tt.expected, tt.actual)
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
			got := validateMetric(io.Discard, "Test Metric", tt.expected, tt.actual)
			if got != tt.want {
				diff := math.Abs(tt.actual - tt.expected)
				tolerance := math.Max(math.Abs(tt.expected)*constants.ValidationTolerancePercent, constants.ValidationMinToleranceKWh)
				t.Errorf("validateMetric() = %v, want %v\nExpected: %.2f, Actual: %.2f, Diff: %.2f, Tolerance: %.2f\n%s",
					got, tt.want, tt.expected, tt.actual, diff, tolerance, tt.description)
			}
		})
	}
}

//
// Edge cases are inputs at the boundaries of what a function handles:
// - Zero values (division by zero concerns)
// - Negative values (sign handling)
// - Boundary conditions (exactly at tolerance limit)

// TestValidateMetric_ZeroValues tests validation with zero expected values.
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
			got := validateMetric(io.Discard, "Test Metric", tt.expected, tt.actual)
			if got != tt.want {
				t.Errorf("validateMetric() = %v, want %v (expected: %.2f, actual: %.2f)",
					got, tt.want, tt.expected, tt.actual)
			}
		})
	}
}

// TestValidateMetric_NegativeValues tests validation with negative values.
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
			got := validateMetric(io.Discard, "Test Metric", tt.expected, tt.actual)
			if got != tt.want {
				t.Errorf("validateMetric() = %v, want %v (expected: %.2f, actual: %.2f)",
					got, tt.want, tt.expected, tt.actual)
			}
		})
	}
}

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
func TestValidateMetrics_MissingFile(t *testing.T) {
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:               "test-system",
				Name:             "Test System",
				ProductionToday:  10.0,
				ConsumptionToday: 8.0,
				GridImportToday:  5.0,
				GridExportToday:  2.0,
				NetFlowToday:     3.0,
			},
		},
	}

	var buf bytes.Buffer
	err := ValidateMetrics(&buf, metrics, "9999-99-99") // Non-existent date

	if err == nil {
		t.Error("Expected error for missing file, got nil")
		// Note: We use t.Error (not t.Fatal) because there's no more code after this
	}
}

//
// It's not enough to just return an error - the error message should be
// HELPFUL. It should tell the user:
// - What went wrong
// - What they can do to fix it
// - Any relevant context (file paths, dates, etc.)

// TestValidateMetrics_MissingFile_HelpfulError tests that missing file error is helpful.
func TestValidateMetrics_MissingFile_HelpfulError(t *testing.T) {
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{ID: "test-system", Name: "Test System"},
		},
	}

	testDate := "2050-06-15" // Date that definitely won't exist
	var buf bytes.Buffer
	err := ValidateMetrics(&buf, metrics, testDate)

	if err == nil {
		t.Fatal("Expected error for missing file, got nil")
		// t.Fatal STOPS the test immediately - no point checking error message if there's no error
	}

	errMsg := err.Error()

	t.Run("contains target date", func(t *testing.T) {
		// The error should mention which date we were looking for
		if !strings.Contains(errMsg, testDate) {
			t.Errorf("Error message should contain date %q", testDate)
		}
	})

	t.Run("contains file path", func(t *testing.T) {
		// The error should tell the user WHERE the file should be
		if !strings.Contains(errMsg, "test-data/expected_values_") {
			t.Error("Error message should contain expected file path")
		}
	})

	t.Run("contains JSON format example", func(t *testing.T) {
		// The error should show the user what format the file needs
		if !strings.Contains(errMsg, "\"date\":") {
			t.Error("Error message should contain JSON format example")
		}
		if !strings.Contains(errMsg, "\"systems\":") {
			t.Error("Error message should contain systems field in example")
		}
		if !strings.Contains(errMsg, "\"expected\":") {
			t.Error("Error message should contain expected field in example")
		}
	})

	t.Run("contains skip validation hint", func(t *testing.T) {
		// The error should tell the user how to skip validation if they don't want it
		if !strings.Contains(errMsg, "without validation") || !strings.Contains(errMsg, "omit the --test flag") {
			t.Error("Error message should contain hint about skipping validation")
		}
	})
}

//
// Sometimes you want to write a test but can't run it yet (missing fixtures,
// external dependencies, etc.). Use t.Skip() to mark it as skipped.

// TestValidateMetrics_InvalidJSON tests handling of malformed JSON.
func TestValidateMetrics_InvalidJSON(t *testing.T) {
	// t.Skip() marks this test as skipped (not failed)
	// The message explains WHY it's skipped
	t.Skip("Requires test fixture with malformed JSON")

	// When someone creates the fixture, they can remove t.Skip()
	// and implement the test logic here
}

//
// Sometimes you want to test a calculation directly, not through the function
// that uses it. This makes tests more focused and failures easier to diagnose.

// TestToleranceCalculation tests the tolerance calculation logic.
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
			total, passed, anyFailed := runMetricTests(io.Discard, tt.cases)
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

//
// These tests call the full ValidateMetrics function with realistic inputs.
// They test how multiple components work together.

// TestValidateMetrics_EmptyMetrics tests validation with empty metrics.
func TestValidateMetrics_EmptyMetrics(t *testing.T) {
	// Create metrics with an EMPTY systems slice
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{}, // No systems!
	}

	// This should fail because expected values will have systems but metrics don't
	var buf bytes.Buffer
	err := ValidateMetrics(&buf, metrics, "2026-01-20")
	if err == nil {
		t.Error("Expected error for empty metrics, got nil")
	}
}

// TestValidateMetrics_SystemIDMismatch tests handling of system ID mismatches.
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
	var buf bytes.Buffer
	err := ValidateMetrics(&buf, metrics, "2026-01-20")
	if err == nil {
		t.Error("Expected error for system ID mismatch, got nil")
	}
}
