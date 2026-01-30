package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
)

// TestGetEnergyImportForDate tests grid import data fetching
func TestGetEnergyImportForDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Return nested array format (energy_import_telemetry)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"intervals": [
				[
					{"end_at": 1737676800, "wh_imported": 1000.0},
					{"end_at": 1737677700, "wh_imported": 1500.0}
				]
			]
		}`))
	}))
	defer server.Close()

	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	imported, err := client.GetEnergyImportForDate(ctx, time.Time{})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// 1000 + 1500 = 2500 Wh = 2.5 kWh
	expected := 2.5
	if imported != expected {
		t.Errorf("Expected %v kWh, got %v kWh", expected, imported)
	}
}

// TestGetEnergyExportForDate tests grid export data fetching
func TestGetEnergyExportForDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Return nested array format (energy_export_telemetry)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"intervals": [
				[
					{"end_at": 1737676800, "wh_exported": 500.0},
					{"end_at": 1737677700, "wh_exported": 750.0}
				]
			]
		}`))
	}))
	defer server.Close()

	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	exported, err := client.GetEnergyExportForDate(ctx, time.Time{})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// 500 + 750 = 1250 Wh = 1.25 kWh
	expected := 1.25
	if exported != expected {
		t.Errorf("Expected %v kWh, got %v kWh", expected, exported)
	}
}

// TestGetConsumptionForDate tests consumption data fetching
func TestGetConsumptionForDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Return single array format (consumption_meter)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"intervals": [
				{"end_at": 1737676800, "enwh": 2000.0},
				{"end_at": 1737677700, "enwh": 2500.0}
			]
		}`))
	}))
	defer server.Close()

	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	consumption, err := client.GetConsumptionForDate(ctx, time.Time{})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// 2000 + 2500 = 4500 Wh = 4.5 kWh
	expected := 4.5
	if consumption != expected {
		t.Errorf("Expected %v kWh, got %v kWh", expected, consumption)
	}
}

// TestGetBatteryDataForDate tests battery data fetching with SOC
func TestGetBatteryDataForDate(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")
	
	// Use yesterday to avoid current time capping issue
	// (GetDayBoundaries caps dayEnd to current time for "today")
	now := time.Now().In(tz)
	yesterday := now.AddDate(0, 0, -1)
	dayStart := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, tz)
	
	// Create timestamps within yesterday's boundaries (guaranteed to be within 00:00-23:59:59)
	ts1 := dayStart.Add(8 * time.Hour).Unix()  // 8 AM yesterday
	ts2 := dayStart.Add(12 * time.Hour).Unix() // 12 PM yesterday
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Return battery telemetry with SOC using timestamps within yesterday
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{
			"last_reported_aggregate_soc": "85%%",
			"intervals": [
				{
					"end_at": %d,
					"charge": {"enwh": 1000.0},
					"discharge": {"enwh": 500.0}
				},
				{
					"end_at": %d,
					"charge": {"enwh": 1500.0},
					"discharge": {"enwh": 750.0}
				}
			]
		}`, ts1, ts2)))
	}))
	defer server.Close()

	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	// Use yesterday as testDate to ensure all intervals are included
	charged, discharged, soc, err := client.GetBatteryDataForDate(ctx, yesterday)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Charged: 1000 + 1500 = 2500 Wh = 2.5 kWh
	if charged != 2.5 {
		t.Errorf("Expected charged %v kWh, got %v kWh", 2.5, charged)
	}

	// Discharged: 500 + 750 = 1250 Wh = 1.25 kWh
	if discharged != 1.25 {
		t.Errorf("Expected discharged %v kWh, got %v kWh", 1.25, discharged)
	}

	// SOC: 85%
	if soc != 85 {
		t.Errorf("Expected SOC %v%%, got %v%%", 85, soc)
	}
}

// TestGetBatteryDataForDate_NoSOC tests battery data when SOC field is missing
func TestGetBatteryDataForDate_NoSOC(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")
	
	// Get current day boundaries
	now := time.Now().In(tz)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)
	ts1 := dayStart.Add(8 * time.Hour).Unix()
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return battery telemetry without SOC field
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{
			"intervals": [
				{
					"end_at": %d,
					"charge": {"enwh": 1000.0},
					"discharge": {"enwh": 500.0}
				}
			]
		}`, ts1)))
	}))
	defer server.Close()

	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	charged, discharged, soc, err := client.GetBatteryDataForDate(ctx, time.Time{})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// SOC should default to 0 when missing
	if soc != 0 {
		t.Errorf("Expected SOC 0%% when field missing, got %v%%", soc)
	}

	// Values should still be calculated correctly
	if charged != 1.0 {
		t.Errorf("Expected charged 1.0 kWh, got %v kWh", charged)
	}
	if discharged != 0.5 {
		t.Errorf("Expected discharged 0.5 kWh, got %v kWh", discharged)
	}
}

// TestGetMetricsFromCloud tests fetching all metrics in one call
func TestGetMetricsFromCloud(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")
	
	// Get current day boundaries
	now := time.Now().In(tz)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)
	ts1 := dayStart.Add(8 * time.Hour).Unix()
	
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Verify authorization
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Return different responses based on endpoint
		path := r.URL.Path
		w.WriteHeader(http.StatusOK)

		if path == "/12345/energy_import_telemetry" {
			w.Write([]byte(fmt.Sprintf(`{"intervals":[[{"end_at":%d,"wh_imported":1000.0}]]}`, ts1)))
		} else if path == "/12345/energy_export_telemetry" {
			w.Write([]byte(fmt.Sprintf(`{"intervals":[[{"end_at":%d,"wh_exported":500.0}]]}`, ts1)))
		} else if path == "/12345/telemetry/production_meter" {
			w.Write([]byte(fmt.Sprintf(`{"intervals":[{"end_at":%d,"wh_del":2000.0}]}`, ts1)))
		} else if path == "/12345/telemetry/battery" {
			w.Write([]byte(fmt.Sprintf(`{
				"last_reported_aggregate_soc":"90%%",
				"intervals":[{
					"end_at":%d,
					"charge":{"enwh":800.0},
					"discharge":{"enwh":400.0}
				}]
			}`, ts1)))
		} else if path == "/12345/telemetry/consumption_meter" {
			w.Write([]byte(fmt.Sprintf(`{"intervals":[{"end_at":%d,"enwh":1500.0}]}`, ts1)))
		}
	}))
	defer server.Close()

	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	metrics, cacheUsed, err := client.GetMetricsFromCloud(ctx, time.Time{})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cacheUsed {
		t.Error("Expected cacheUsed=false for fresh API calls")
	}

	// Verify all metrics were fetched
	if metrics.GridImportToday != 1.0 {
		t.Errorf("Expected GridImport 1.0 kWh, got %v kWh", metrics.GridImportToday)
	}
	if metrics.GridExportToday != 0.5 {
		t.Errorf("Expected GridExport 0.5 kWh, got %v kWh", metrics.GridExportToday)
	}
	if metrics.ProductionToday != 2.0 {
		t.Errorf("Expected Production 2.0 kWh, got %v kWh", metrics.ProductionToday)
	}
	if metrics.BatteryChargedToday != 0.8 {
		t.Errorf("Expected BatteryCharged 0.8 kWh, got %v kWh", metrics.BatteryChargedToday)
	}
	if metrics.BatteryDischargedToday != 0.4 {
		t.Errorf("Expected BatteryDischarged 0.4 kWh, got %v kWh", metrics.BatteryDischargedToday)
	}
	if metrics.ConsumptionToday != 1.5 {
		t.Errorf("Expected Consumption 1.5 kWh, got %v kWh", metrics.ConsumptionToday)
	}
	if metrics.BatterySOC != 90 {
		t.Errorf("Expected BatterySOC 90%%, got %v%%", metrics.BatterySOC)
	}

	// Verify all endpoints were called (5 API calls)
	if requestCount != 5 {
		t.Errorf("Expected 5 API requests, got %v", requestCount)
	}
}

// TestGetMetricsFromCloud_PartialFailure tests handling when optional metrics fail
func TestGetMetricsFromCloud_PartialFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if path == "/12345/energy_import_telemetry" {
			// Simulate import endpoint failure (optional)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"server error"}`))
		} else if path == "/12345/energy_export_telemetry" {
			// Simulate export endpoint failure (optional)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"server error"}`))
		} else if path == "/12345/telemetry/production_meter" {
			// Production succeeds (required)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"intervals":[{"end_at":1737676800,"wh_del":2000.0}]}`))
		} else if path == "/12345/telemetry/battery" {
			// Battery fails (optional)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"server error"}`))
		} else if path == "/12345/telemetry/consumption_meter" {
			// Consumption succeeds
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"intervals":[{"end_at":1737676800,"enwh":1500.0}]}`))
		}
	}))
	defer server.Close()

	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	metrics, _, err := client.GetMetricsFromCloud(ctx, time.Time{})

	// Should succeed despite optional failures
	if err != nil {
		t.Fatalf("Expected no error for optional metric failures, got: %v", err)
	}

	// Optional metrics should be 0
	if metrics.GridImportToday != 0 {
		t.Errorf("Expected GridImport 0 (failed), got %v", metrics.GridImportToday)
	}
	if metrics.GridExportToday != 0 {
		t.Errorf("Expected GridExport 0 (failed), got %v", metrics.GridExportToday)
	}
	if metrics.BatteryChargedToday != 0 {
		t.Errorf("Expected BatteryCharged 0 (failed), got %v", metrics.BatteryChargedToday)
	}

	// Required metrics should have values
	if metrics.ProductionToday != 2.0 {
		t.Errorf("Expected Production 2.0 kWh, got %v kWh", metrics.ProductionToday)
	}
	if metrics.ConsumptionToday != 1.5 {
		t.Errorf("Expected Consumption 1.5 kWh, got %v kWh", metrics.ConsumptionToday)
	}
}

// TestGetMetricsFromCloud_ProductionFailure tests that production failure causes overall failure
func TestGetMetricsFromCloud_ProductionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if path == "/12345/energy_import_telemetry" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"intervals":[[{"end_at":1737676800,"wh_imported":1000.0}]]}`))
		} else if path == "/12345/energy_export_telemetry" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"intervals":[[{"end_at":1737676800,"wh_exported":500.0}]]}`))
		} else if path == "/12345/telemetry/production_meter" {
			// Production fails (required - should cause overall failure)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"server error"}`))
		}
	}))
	defer server.Close()

	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	_, _, err := client.GetMetricsFromCloud(ctx, time.Time{})

	// Production is required - failure should cause overall failure
	if err == nil {
		t.Fatal("Expected error for production failure, got nil")
	}

	if !contains(err.Error(), "failed to get production") {
		t.Errorf("Expected 'failed to get production' error, got: %v", err)
	}
}

// TestGetMetricsFromCloud_ContextCancellation tests context cancellation handling
func TestGetMetricsFromCloud_ContextCancellation(t *testing.T) {
	// Create server that delays responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"intervals":[]}`))
	}))
	defer server.Close()

	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	// Create context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before making request

	_, _, err := client.GetMetricsFromCloud(ctx, time.Time{})

	// Should get context cancellation error
	if err == nil {
		t.Fatal("Expected error for cancelled context, got nil")
	}

	// Error should indicate context cancellation
	if err != context.Canceled {
		t.Logf("Got error: %v (expected context.Canceled)", err)
	}
}

// TestGetProductionForDate_EmptyIntervals tests handling of empty intervals
func TestGetProductionForDate_EmptyIntervals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"intervals":[]}`))
	}))
	defer server.Close()

	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	production, err := client.GetProductionForDate(ctx, time.Time{})

	if err != nil {
		t.Fatalf("Expected no error for empty intervals, got: %v", err)
	}

	// Empty intervals should result in 0 kWh
	if production != 0 {
		t.Errorf("Expected production 0 kWh for empty intervals, got %v kWh", production)
	}
}

// TestGetEnergyImportForDate_EmptyIntervals tests empty intervals for import
func TestGetEnergyImportForDate_EmptyIntervals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"intervals":[[]]}`)) // Nested empty array
	}))
	defer server.Close()

	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	imported, err := client.GetEnergyImportForDate(ctx, time.Time{})

	if err != nil {
		t.Fatalf("Expected no error for empty intervals, got: %v", err)
	}

	// Empty intervals should result in 0 kWh
	if imported != 0 {
		t.Errorf("Expected import 0 kWh for empty intervals, got %v kWh", imported)
	}
}

// TestGetProductionForDate_InvalidJSON tests handling of malformed JSON
func TestGetProductionForDate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	_, err := client.GetProductionForDate(ctx, time.Time{})

	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

// TestGetBatteryDataForDate_InvalidJSON tests battery endpoint with malformed JSON
func TestGetBatteryDataForDate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	_, _, _, err := client.GetBatteryDataForDate(ctx, time.Time{})

	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}

	if !contains(err.Error(), "parsing battery response JSON") {
		t.Errorf("Expected 'parsing battery response JSON' error, got: %v", err)
	}
}

// TestGetMetricsFromCloud_ConsumptionFallback tests consumption calculation fallback
func TestGetMetricsFromCloud_ConsumptionFallback(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")
	
	// Get current day boundaries
	now := time.Now().In(tz)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)
	ts1 := dayStart.Add(8 * time.Hour).Unix()
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if path == "/12345/energy_import_telemetry" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf(`{"intervals":[[{"end_at":%d,"wh_imported":2000.0}]]}`, ts1)))
		} else if path == "/12345/energy_export_telemetry" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf(`{"intervals":[[{"end_at":%d,"wh_exported":500.0}]]}`, ts1)))
		} else if path == "/12345/telemetry/production_meter" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf(`{"intervals":[{"end_at":%d,"wh_del":3000.0}]}`, ts1)))
		} else if path == "/12345/telemetry/battery" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf(`{
				"intervals":[{
					"end_at":%d,
					"charge":{"enwh":1000.0},
					"discharge":{"enwh":500.0}
				}]
			}`, ts1)))
		} else if path == "/12345/telemetry/consumption_meter" {
			// Consumption endpoint fails - should fall back to calculation
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"server error"}`))
		}
	}))
	defer server.Close()

	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	metrics, _, err := client.GetMetricsFromCloud(ctx, time.Time{})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Consumption = Production + Import - Export - Charged + Discharged
	// = 3.0 + 2.0 - 0.5 - 1.0 + 0.5 = 4.0 kWh
	expectedConsumption := 4.0
	if metrics.ConsumptionToday != expectedConsumption {
		t.Errorf("Expected consumption %v kWh (calculated), got %v kWh", expectedConsumption, metrics.ConsumptionToday)
	}
}

// TestBuildTelemetryURL tests URL construction
func TestBuildTelemetryURL(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL("https://api.test.com/api/v4/systems", "12345", "test-key", "test-token", tz)

	// Create test timestamps
	dayStart := time.Date(2026, 1, 20, 0, 0, 0, 0, tz)
	dayEnd := time.Date(2026, 1, 20, 23, 59, 59, 0, tz)

	url := client.buildTelemetryURL("telemetry/production_meter", dayStart, dayEnd)

	// Verify URL structure
	expectedPrefix := "https://api.test.com/api/v4/systems/12345/telemetry/production_meter?key=test-key&start_at="
	if !contains(url, expectedPrefix) {
		t.Errorf("Expected URL to start with %q, got: %v", expectedPrefix, url)
	}

	// Verify timestamps are included
	if !contains(url, fmt.Sprintf("start_at=%d", dayStart.Unix())) {
		t.Errorf("Expected start_at=%d in URL, got: %v", dayStart.Unix(), url)
	}
	if !contains(url, fmt.Sprintf("end_at=%d", dayEnd.Unix())) {
		t.Errorf("Expected end_at=%d in URL, got: %v", dayEnd.Unix(), url)
	}
}

// TestNewEnlightenCloudClient tests client constructor
func TestNewEnlightenCloudClient(t *testing.T) {
	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClient("12345", "test-key", "test-token", tz)

	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	if client.systemID != "12345" {
		t.Errorf("Expected systemID '12345', got %q", client.systemID)
	}

	if client.apiKey != "test-key" {
		t.Errorf("Expected apiKey 'test-key', got %q", client.apiKey)
	}

	if client.accessToken != "test-token" {
		t.Errorf("Expected accessToken 'test-token', got %q", client.accessToken)
	}

	if client.baseURL != constants.EnphaseAPIv4SystemsURL {
		t.Errorf("Expected baseURL %q, got %q", constants.EnphaseAPIv4SystemsURL, client.baseURL)
	}

	if client.timezone != tz {
		t.Error("Expected timezone to match provided timezone")
	}

	if client.httpClient == nil {
		t.Error("Expected non-nil httpClient")
	}
}

// TestNewEnlightenCloudClientWithBaseURL tests client constructor with custom base URL
func TestNewEnlightenCloudClientWithBaseURL(t *testing.T) {
	customURL := "https://custom.api.com/v4/systems"
	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(customURL, "12345", "test-key", "test-token", tz)

	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	if client.baseURL != customURL {
		t.Errorf("Expected baseURL %q, got %q", customURL, client.baseURL)
	}
}

// TestCacheUsedTracking tests that cacheUsed flag is properly tracked
func TestCacheUsedTracking(t *testing.T) {
	// Save original cache state
	originalTestMode := cache.TestMode()
	originalCacheDisabled := cache.CacheDisabled()
	defer func() {
		cache.ResetState()
		if originalTestMode {
			cache.SetTestMode(true)
		}
		if originalCacheDisabled {
			cache.SetCacheDisabled(true)
		}
	}()

	// Disable cache and test mode for this test
	cache.SetCacheDisabled(true)
	cache.SetTestMode(false)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"intervals":[{"end_at":1737676800,"wh_del":1500.0}]}`))
	}))
	defer server.Close()

	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(server.URL, "12345", "test-key", "test-token", tz)

	ctx := context.Background()
	_, err := client.GetProductionForDate(ctx, time.Time{})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// With cache disabled, cacheUsed should be false
	if client.cacheUsed {
		t.Error("Expected cacheUsed=false when cache is disabled")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
