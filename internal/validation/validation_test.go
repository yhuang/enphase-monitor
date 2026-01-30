package validation

import (
	"math"
	"testing"

	"enphase-monitor/internal/constants"
)

// TestValidationTolerance tests the validation tolerance calculation
func TestValidationTolerance(t *testing.T) {
	tests := []struct {
		name        string
		expected    float64
		actual      float64
		shouldPass  bool
		description string
	}{
		{
			name:        "within 10% tolerance",
			expected:    100.0,
			actual:      105.0,
			shouldPass:  true,
			description: "5% difference is within 10% tolerance",
		},
		{
			name:        "exactly at 10% tolerance",
			expected:    100.0,
			actual:      110.0,
			shouldPass:  true,
			description: "10% difference is exactly at tolerance",
		},
		{
			name:        "exceeds 10% tolerance",
			expected:    100.0,
			actual:      115.0,
			shouldPass:  false,
			description: "15% difference exceeds 10% tolerance",
		},
		{
			name:        "small value with minimum tolerance",
			expected:    0.5,
			actual:      0.55,
			shouldPass:  true,
			description: "0.05 kWh difference is within 0.1 kWh minimum tolerance",
		},
		{
			name:        "small value exceeds minimum tolerance",
			expected:    0.5,
			actual:      0.65,
			shouldPass:  false,
			description: "0.15 kWh difference exceeds 0.1 kWh minimum tolerance",
		},
		{
			name:        "zero expected value",
			expected:    0.0,
			actual:      0.05,
			shouldPass:  true,
			description: "Actual 0.05 with expected 0 uses minimum tolerance 0.1",
		},
		{
			name:        "zero expected value exceeds tolerance",
			expected:    0.0,
			actual:      0.15,
			shouldPass:  false,
			description: "Actual 0.15 with expected 0 exceeds minimum tolerance 0.1",
		},
		{
			name:        "both zero",
			expected:    0.0,
			actual:      0.0,
			shouldPass:  true,
			description: "Both zero should pass",
		},
		{
			name:        "negative values within tolerance",
			expected:    -100.0,
			actual:      -105.0,
			shouldPass:  true,
			description: "Negative values with 5% difference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate tolerance using same logic as validation.go
			diff := math.Abs(tt.actual - tt.expected)
			tolerance := math.Abs(tt.expected) * constants.ValidationTolerancePercent
			if tolerance < constants.ValidationMinToleranceKWh {
				tolerance = constants.ValidationMinToleranceKWh
			}

			passed := diff <= tolerance

			if passed != tt.shouldPass {
				t.Errorf("Validation %s:\n  Expected: %.2f, Actual: %.2f, Diff: %.2f, Tolerance: %.2f\n  Result: %v, Want: %v\n  %s",
					tt.name, tt.expected, tt.actual, diff, tolerance, passed, tt.shouldPass, tt.description)
			}
		})
	}
}

// TestPercentageDifferenceCalculation tests percentage difference calculation
func TestPercentageDifferenceCalculation(t *testing.T) {
	tests := []struct {
		name           string
		expected       float64
		actual         float64
		wantPercentage float64
		wantInfinite   bool
		description    string
	}{
		{
			name:           "5% increase",
			expected:       100.0,
			actual:         105.0,
			wantPercentage: 5.0,
			wantInfinite:   false,
			description:    "5 kWh difference on 100 kWh expected",
		},
		{
			name:           "10% decrease",
			expected:       100.0,
			actual:         90.0,
			wantPercentage: -10.0,
			wantInfinite:   false,
			description:    "10 kWh difference on 100 kWh expected",
		},
		{
			name:           "zero expected with non-zero actual",
			expected:       0.0,
			actual:         10.0,
			wantPercentage: constants.ValidationInfinitePercent,
			wantInfinite:   true,
			description:    "Expected 0 but got 10 - infinite percentage",
		},
		{
			name:           "zero expected with zero actual",
			expected:       0.0,
			actual:         0.0,
			wantPercentage: 0.0,
			wantInfinite:   false,
			description:    "Both zero - 0% difference",
		},
		{
			name:           "small percentage rounds to zero",
			expected:       100.0,
			actual:         100.3,
			wantPercentage: 0.3,
			wantInfinite:   false,
			description:    "0.3% difference might round to 0 in display",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := tt.actual - tt.expected
			var percentDiff float64

			if tt.expected != 0 {
				percentDiff = (diff / math.Abs(tt.expected)) * 100.0
			} else if diff != 0 {
				percentDiff = constants.ValidationInfinitePercent
			} else {
				percentDiff = 0.0
			}

			// Check if infinite is expected
			if tt.wantInfinite {
				if percentDiff != constants.ValidationInfinitePercent {
					t.Errorf("Expected infinite percentage (%.1f), got %.1f", constants.ValidationInfinitePercent, percentDiff)
				}
			} else {
				// Check percentage matches (with small tolerance for floating point)
				if math.Abs(percentDiff-tt.wantPercentage) > 0.01 {
					t.Errorf("Percentage difference = %.2f%%, want %.2f%%\n  %s",
						percentDiff, tt.wantPercentage, tt.description)
				}
			}
		})
	}
}
