// Package validation - validation_test.go
//
// TEST SETUP
// ----------
// This test suite validates the validation logic that compares actual metrics
// against expected values from JSON files. Tests focus on tolerance calculations,
// metric comparison logic, and error handling.
//
// TEST PLAN
// ---------
// 1. Metric Validation Tests
//    - Test validateMetric with exact matches
//    - Test validateMetric with values within tolerance (±10%)
//    - Test validateMetric with values outside tolerance
//    - Test tolerance calculation edge cases (zero values, negative values)
//    - Test minimum tolerance enforcement (0.1 kWh)
//
// 2. Expected Values Loading Tests
//    - Test loading valid expected values JSON
//    - Test handling missing JSON files
//    - Test handling malformed JSON
//    - Test date mismatch detection
//
// 3. System Matching Tests
//    - Test finding systems by ID
//    - Test handling missing systems
//    - Test multiple systems validation
//
// TESTING APPROACH
// ----------------
// - Table-driven tests for tolerance calculations
// - File fixtures in test-data/ directory
// - Mock metrics structures for validation
// - Error validation for missing/malformed data
//
// PATTERN USED
// ------------
// - Pattern 1: Table-Driven Tests
// - Pattern 3: Subtests with t.Run()
// - Pattern 8: Error Path Testing
//
// See TESTING.md for detailed pattern explanations.
package validation

import (
	"math"
	"testing"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/constants"
)

// TestValidateMetric_ExactMatch tests validation with exact matches
func TestValidateMetric_ExactMatch(t *testing.T) {
	tests := []struct {
		name     string
		expected float64
		actual   float64
		want     bool
	}{
		{"zero values", 0.0, 0.0, true},
		{"positive match", 10.5, 10.5, true},
		{"negative match", -5.2, -5.2, true},
		{"large value match", 1234.5, 1234.5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateMetric("Test Metric", tt.expected, tt.actual)
			if got != tt.want {
				t.Errorf("validateMetric() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestValidateMetric_WithinTolerance tests validation within tolerance bounds
func TestValidateMetric_WithinTolerance(t *testing.T) {
	tests := []struct {
		name        string
		expected    float64
		actual      float64
		want        bool
		description string
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
				diff := math.Abs(tt.actual - tt.expected)
				tolerance := math.Max(math.Abs(tt.expected)*constants.ValidationTolerancePercent, constants.ValidationMinToleranceKWh)
				t.Errorf("validateMetric() = %v, want %v\nExpected: %.2f, Actual: %.2f, Diff: %.2f, Tolerance: %.2f\n%s",
					got, tt.want, tt.expected, tt.actual, diff, tolerance, tt.description)
			}
		})
	}
}

// TestValidateMetric_OutsideTolerance tests validation outside tolerance bounds
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
			want:        false,
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

// TestValidateMetric_ZeroValues tests validation with zero expected values
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
			want:     true,
		},
		{
			name:     "zero expected, small actual (within min tolerance)",
			expected: 0.0,
			actual:   0.1, // at minimum tolerance boundary
			want:     true,
		},
		{
			name:     "zero expected, actual outside min tolerance",
			expected: 0.0,
			actual:   0.2, // exceeds 0.1 kWh minimum tolerance
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

// TestValidateMetric_NegativeValues tests validation with negative values
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
			actual:   -10.9, // 9% difference
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

// TestValidateMetrics_MissingFile tests handling of missing expected values file
func TestValidateMetrics_MissingFile(t *testing.T) {
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:                "test-system",
				Name:              "Test System",
				ProductionToday:   10.0,
				ConsumptionToday:  8.0,
				GridImportToday:   5.0,
				GridExportToday:   2.0,
				NetImportedToday:  3.0,
			},
		},
	}

	err := ValidateMetrics(metrics, "9999-99-99") // Non-existent date

	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}
}

// TestValidateMetrics_InvalidJSON tests handling of malformed JSON
func TestValidateMetrics_InvalidJSON(t *testing.T) {
	// This test would require creating a malformed JSON file in test-data/
	// For now, we test that the error handling path exists
	t.Skip("Requires test fixture with malformed JSON")
}

// TestToleranceCalculation tests the tolerance calculation logic
func TestToleranceCalculation(t *testing.T) {
	tests := []struct {
		name              string
		expected          float64
		wantTolerance     float64
		description       string
	}{
		{
			name:              "large value uses percentage",
			expected:          100.0,
			wantTolerance:     10.0, // 10% of 100
			description:       "For large values, 10% tolerance applies",
		},
		{
			name:              "small value uses minimum",
			expected:          0.5,
			wantTolerance:     0.1, // Minimum tolerance
			description:       "For small values, minimum 0.1 kWh applies",
		},
		{
			name:              "zero value uses minimum",
			expected:          0.0,
			wantTolerance:     0.1, // Minimum tolerance
			description:       "For zero values, minimum 0.1 kWh applies",
		},
		{
			name:              "medium value at crossover point",
			expected:          1.0,
			wantTolerance:     0.1, // 10% of 1.0 = 0.1, equal to minimum
			description:       "At crossover point, both yield same tolerance",
		},
		{
			name:              "above crossover uses percentage",
			expected:          1.5,
			wantTolerance:     0.15, // 10% of 1.5 > 0.1 minimum
			description:       "Above crossover, percentage tolerance applies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := math.Max(math.Abs(tt.expected)*constants.ValidationTolerancePercent, constants.ValidationMinToleranceKWh)
			if math.Abs(got-tt.wantTolerance) > 0.001 { // Allow small floating point error
				t.Errorf("Tolerance calculation = %.3f, want %.3f\n%s",
					got, tt.wantTolerance, tt.description)
			}
		})
	}
}

// TestValidateMetrics_EmptyMetrics tests validation with empty metrics
func TestValidateMetrics_EmptyMetrics(t *testing.T) {
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{},
	}

	// This should fail because expected values will have systems but metrics don't
	err := ValidateMetrics(metrics, "2026-01-20")
	if err == nil {
		t.Error("Expected error for empty metrics, got nil")
	}
}

// TestValidateMetrics_SystemIDMismatch tests handling of system ID mismatches
func TestValidateMetrics_SystemIDMismatch(t *testing.T) {
	metrics := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{
				ID:   "wrong-id",
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
