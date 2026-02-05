package app

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/api"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/display"
	"enphase-monitor/internal/types"
)

// mockCloudClient implements api.CloudClient for testing
type mockCloudClient struct {
	metrics *api.LocalMetrics
	err     error
}

func (m *mockCloudClient) GetMetricsFromCloud(ctx context.Context, date time.Time, queryType constants.QueryType) (*api.LocalMetrics, bool, error) {
	if m.err != nil {
		return nil, false, m.err
	}
	return m.metrics, false, nil
}

func (m *mockCloudClient) GetEnergyImportForDate(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	return 0, nil
}

func (m *mockCloudClient) GetEnergyExportForDate(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	return 0, nil
}

func (m *mockCloudClient) GetProductionForDate(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	return 0, nil
}

func (m *mockCloudClient) GetConsumptionForDate(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	return 0, nil
}

func (m *mockCloudClient) GetBatteryDataForDate(ctx context.Context, testDate time.Time, queryType constants.QueryType) (charged float64, discharged float64, soc int, err error) {
	return 0, 0, 0, nil
}

// createMockAggregator creates an aggregator with a mock cloud client
func createMockAggregator(metrics *api.LocalMetrics, err error) *aggregator.DataAggregator {
	mockGetter := func(ctx context.Context, apiConfig *types.APIConfig) (string, error) {
		return "mock-token", nil
	}
	return aggregator.NewDataAggregatorWithFactory(mockGetter, func(systemID, apiKey, accessToken string, tz *time.Location) api.CloudClient {
		return &mockCloudClient{metrics: metrics, err: err}
	})
}

// TestRunOnce_Success tests successful single execution
func TestRunOnce_Success(t *testing.T) {
	ctx := context.Background()

	// Create mock metrics
	mockMetrics := &api.LocalMetrics{
		ProductionToday:  10.0,
		ConsumptionToday: 8.0,
		GridImportToday:  5.0,
		GridExportToday:  3.0,
	}

	// Create mock aggregator
	agg := createMockAggregator(mockMetrics, nil)

	// Create config
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "Test System", ID: "12345"},
		},
		API: &types.APIConfig{
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RefreshToken: "test-refresh",
		},
		RefreshIntervalSeconds: 3600,
	}

	// Create display with buffer
	tz := time.UTC
	buf := &bytes.Buffer{}
	disp := display.NewDisplayWithWriter(display.GetDefaultColors(), tz, buf)

	// Run once
	testDate := time.Time{} // Use zero time for "today"

	err := RunOnce(ctx, agg, disp, cfg, testDate, constants.QueryTypeDay, false, tz)
	if err != nil {
		t.Fatalf("RunOnce() error = %v, want nil", err)
	}

	// Verify output contains expected metrics
	output := buf.String()
	if !strings.Contains(output, "ENPHASE") {
		t.Error("RunOnce() output missing header")
	}
	if !strings.Contains(output, "10.0 kWh") {
		t.Error("RunOnce() output missing production value")
	}
}

// TestFetchAndDisplay_Success tests successful metric fetching and display
func TestFetchAndDisplay_Success(t *testing.T) {
	ctx := context.Background()

	// Create mock metrics
	mockMetrics := &api.LocalMetrics{
		ProductionToday:  15.0,
		ConsumptionToday: 12.0,
		GridImportToday:  7.0,
		GridExportToday:  4.0,
	}

	// Create mock aggregator
	agg := createMockAggregator(mockMetrics, nil)

	// Create config
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "Test System", ID: "12345"},
		},
		API: &types.APIConfig{
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RefreshToken: "test-refresh",
		},
		RefreshIntervalSeconds: 3600,
	}

	// Create display with buffer
	tz := time.UTC
	buf := &bytes.Buffer{}
	disp := display.NewDisplayWithWriter(display.GetDefaultColors(), tz, buf)

	// Test fetchAndDisplay
	testDate := time.Time{}
	if err := fetchAndDisplay(ctx, agg, disp, cfg, testDate, constants.QueryTypeDay, tz); err != nil {
		t.Fatalf("fetchAndDisplay: %v", err)
	}

	// Verify output
	output := buf.String()
	if !strings.Contains(output, "ENPHASE") {
		t.Error("fetchAndDisplay() output missing header")
	}
	if !strings.Contains(output, "15.0 kWh") {
		t.Error("fetchAndDisplay() output missing production value")
	}
}

// TestFetchAndDisplay_ContextCancelled tests handling of cancelled context
func TestFetchAndDisplay_ContextCancelled(t *testing.T) {
	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Create mock aggregator that returns context error
	agg := createMockAggregator(nil, context.Canceled)

	// Create config
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "Test", ID: "12345"},
		},
		API: &types.APIConfig{
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RefreshToken: "test-refresh",
		},
	}

	// Create display with buffer
	tz := time.UTC
	buf := &bytes.Buffer{}
	disp := display.NewDisplayWithWriter(display.GetDefaultColors(), tz, buf)

	// Test fetchAndDisplay with cancelled context
	testDate := time.Time{}
	if err := fetchAndDisplay(ctx, agg, disp, cfg, testDate, constants.QueryTypeDay, tz); err != nil {
		t.Fatalf("fetchAndDisplay: %v", err)
	}

	// Should return silently (no error displayed)
	output := buf.String()
	if strings.Contains(output, "ERROR") {
		t.Error("fetchAndDisplay() should not display error for cancelled context")
	}
}

// TestFetchAndDisplay_Error tests error handling
func TestFetchAndDisplay_Error(t *testing.T) {
	ctx := context.Background()

	// Create mock aggregator that returns error
	mockErr := context.DeadlineExceeded
	agg := createMockAggregator(nil, mockErr)

	// Create config
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "Test", ID: "12345"},
		},
		API: &types.APIConfig{
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RefreshToken: "test-refresh",
		},
	}

	// Create display with buffer
	tz := time.UTC
	buf := &bytes.Buffer{}
	disp := display.NewDisplayWithWriter(display.GetDefaultColors(), tz, buf)

	// Test fetchAndDisplay with error
	testDate := time.Time{}
	if err := fetchAndDisplay(ctx, agg, disp, cfg, testDate, constants.QueryTypeDay, tz); err != nil {
		t.Fatalf("fetchAndDisplay: %v", err)
	}

	// Should display error
	output := buf.String()
	if !strings.Contains(output, "ERROR") {
		t.Error("fetchAndDisplay() should display error message")
	}
}

// TestRunContinuous_ImmediateExecution tests that continuous mode runs immediately
func TestRunContinuous_ImmediateExecution(t *testing.T) {
	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Create mock metrics
	mockMetrics := &api.LocalMetrics{
		ProductionToday:  20.0,
		ConsumptionToday: 15.0,
		GridImportToday:  8.0,
		GridExportToday:  5.0,
	}

	// Create mock aggregator
	agg := createMockAggregator(mockMetrics, nil)

	// Create config with short refresh interval
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "Test System", ID: "12345"},
		},
		API: &types.APIConfig{
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RefreshToken: "test-refresh",
		},
		RefreshIntervalSeconds: 1, // 1 second (won't actually fire in 100ms test)
	}

	// Create display with buffer
	tz := time.UTC
	buf := &bytes.Buffer{}
	disp := display.NewDisplayWithWriter(display.GetDefaultColors(), tz, buf)

	// Run continuous (will exit after 100ms due to context timeout)
	testDate := time.Time{}
	err := RunContinuous(ctx, agg, disp, cfg, testDate, constants.QueryTypeDay, tz)
	if err != nil {
		t.Fatalf("RunContinuous() error = %v, want nil", err)
	}

	// Verify output shows immediate execution
	output := buf.String()
	if !strings.Contains(output, "Starting continuous monitoring") {
		t.Error("RunContinuous() output missing start message")
	}
	if !strings.Contains(output, "Press Ctrl+C to stop") {
		t.Error("RunContinuous() output missing instruction message")
	}
	if !strings.Contains(output, "20.0 kWh") {
		t.Error("RunContinuous() should execute immediately and display metrics")
	}
	if !strings.Contains(output, "Shutting down gracefully") {
		t.Error("RunContinuous() output missing shutdown message")
	}
}

// TestRunContinuous_GracefulShutdown tests graceful shutdown on context cancellation
func TestRunContinuous_GracefulShutdown(t *testing.T) {
	// Create context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Create mock metrics
	mockMetrics := &api.LocalMetrics{
		ProductionToday: 30.0,
	}

	// Create mock aggregator
	agg := createMockAggregator(mockMetrics, nil)

	// Create config
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "Test", ID: "12345"},
		},
		API: &types.APIConfig{
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RefreshToken: "test-refresh",
		},
		RefreshIntervalSeconds: 3600,
	}

	// Create display with buffer
	tz := time.UTC
	buf := &bytes.Buffer{}
	disp := display.NewDisplayWithWriter(display.GetDefaultColors(), tz, buf)

	// Cancel context after a short delay (done channel so we can wait for goroutine exit)
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Run continuous
	testDate := time.Time{}
	err := RunContinuous(ctx, agg, disp, cfg, testDate, constants.QueryTypeDay, tz)
	if err != nil {
		t.Fatalf("RunContinuous() error = %v, want nil", err)
	}
	<-done // Wait for cancel goroutine to exit before test ends

	// Verify shutdown message
	output := buf.String()
	if !strings.Contains(output, "Shutting down gracefully") {
		t.Error("RunContinuous() should display graceful shutdown message")
	}
}

// Suppress unused import warning - io package is needed for display.NewDisplayWithWriter signature
var _ io.Writer
