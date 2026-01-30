package validation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
)

// TestValidateMetricsWithFile tests the ValidateMetrics function with actual file I/O
func TestValidateMetricsWithFile(t *testing.T) {
	// Create temp directory for test data
	tmpDir := t.TempDir()

	// Create expected values file
	expectedValues := ExpectedValues{
		Date: "2026-01-20",
		Systems: []struct {
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
		}{
			{
				ID:   "12345",
				Name: "Test System",
				Expected: struct {
					GridImport        float64 `json:"grid_import"`
					GridExport        float64 `json:"grid_export"`
					Production        float64 `json:"production"`
					BatteryDischarged float64 `json:"battery_discharged"`
					BatteryCharged    float64 `json:"battery_charged"`
					NetImported       float64 `json:"net_imported"`
					Consumption       float64 `json:"consumption"`
				}{
					GridImport:        10.0,
					GridExport:        5.0,
					Production:        20.0,
					BatteryDischarged: 3.0,
					BatteryCharged:    4.0,
					NetImported:       5.0,
					Consumption:       25.0,
				},
			},
		},
	}

	// Write expected values to temp file in test-data subdirectory
	testDataDir := filepath.Join(tmpDir, "test-data")
	if err := os.MkdirAll(testDataDir, 0755); err != nil {
		t.Fatalf("Failed to create test-data directory: %v", err)
	}

	expectedData, err := json.Marshal(expectedValues)
	if err != nil {
		t.Fatalf("Failed to marshal expected values: %v", err)
	}

	expectedFile := filepath.Join(testDataDir, "expected_values_2026-01-20.json")
	if err := os.WriteFile(expectedFile, expectedData, 0644); err != nil {
		t.Fatalf("Failed to write expected values file: %v", err)
	}

	// Change to temp directory temporarily
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(oldDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	tests := []struct {
		name        string
		metrics     *aggregator.AggregatedMetrics
		testDate    string
		expectError bool
		description string
	}{
		{
			name: "metrics match expected values",
			metrics: &aggregator.AggregatedMetrics{
				Timestamp: time.Now(),
				Systems: []aggregator.SystemMetrics{
					{
						ID:                     "12345",
						Name:                   "Test System",
						GridImportToday:        10.0,
						GridExportToday:        5.0,
						ProductionToday:        20.0,
						BatteryDischargedToday: 3.0,
						BatteryChargedToday:    4.0,
						NetImportedToday:       5.0,
						ConsumptionToday:       25.0,
					},
				},
			},
			testDate:    "2026-01-20",
			expectError: false,
			description: "All metrics match within tolerance",
		},
		{
			name: "metrics exceed tolerance",
			metrics: &aggregator.AggregatedMetrics{
				Timestamp: time.Now(),
				Systems: []aggregator.SystemMetrics{
					{
						ID:                     "12345",
						Name:                   "Test System",
						GridImportToday:        15.0, // 50% off
						GridExportToday:        5.0,
						ProductionToday:        20.0,
						BatteryDischargedToday: 3.0,
						BatteryChargedToday:    4.0,
						NetImportedToday:       5.0,
						ConsumptionToday:       25.0,
					},
				},
			},
			testDate:    "2026-01-20",
			expectError: true,
			description: "Grid import exceeds 10% tolerance",
		},
		{
			name: "system not found in expected values",
			metrics: &aggregator.AggregatedMetrics{
				Timestamp: time.Now(),
				Systems: []aggregator.SystemMetrics{
					{
						ID:                     "99999",
						Name:                   "Unknown System",
						GridImportToday:        10.0,
						GridExportToday:        5.0,
						ProductionToday:        20.0,
						BatteryDischargedToday: 3.0,
						BatteryChargedToday:    4.0,
						NetImportedToday:       5.0,
						ConsumptionToday:       25.0,
					},
				},
			},
			testDate:    "2026-01-20",
			expectError: true,
			description: "System ID not found in expected values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMetrics(tt.metrics, tt.testDate)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestValidateMetricsMissingFile tests error handling when expected values file is missing
func TestValidateMetricsMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(tmpDir)

	metrics := &aggregator.AggregatedMetrics{
		Timestamp: time.Now(),
		Systems: []aggregator.SystemMetrics{
			{
				ID:   "12345",
				Name: "Test System",
			},
		},
	}

	err := ValidateMetrics(metrics, "2026-01-20")
	if err == nil {
		t.Error("Expected error for missing file but got none")
	}
	if err != nil && !os.IsNotExist(err) && err.Error() == "" {
		// Error message should mention the missing file
		t.Errorf("Error should mention missing file: %v", err)
	}
}

// TestValidateMetricsInvalidJSON tests error handling for invalid JSON
func TestValidateMetricsInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(tmpDir)

	// Create invalid JSON file
	invalidJSON := []byte("not valid json{")
	expectedFile := filepath.Join("test-data", "expected_values_2026-01-20.json")
	os.MkdirAll(filepath.Dir(expectedFile), 0755)
	if err := os.WriteFile(expectedFile, invalidJSON, 0644); err != nil {
		t.Fatalf("Failed to write invalid JSON file: %v", err)
	}

	metrics := &aggregator.AggregatedMetrics{
		Timestamp: time.Now(),
		Systems: []aggregator.SystemMetrics{
			{
				ID:   "12345",
				Name: "Test System",
			},
		},
	}

	err := ValidateMetrics(metrics, "2026-01-20")
	if err == nil {
		t.Error("Expected error for invalid JSON but got none")
	}
}
