package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/api"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/credentials"
	"enphase-monitor/internal/display"
	"enphase-monitor/internal/types"
)

// mockCloudClient implements aggregator.CloudClient for testing
type mockCloudClient struct {
	metrics *api.LocalMetrics
	err     error
}

func (m *mockCloudClient) GetMetricsFromCloud(ctx context.Context, date time.Time, queryMode constants.QueryMode) (*api.LocalMetrics, bool, error) {
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	if m.err != nil {
		return nil, false, m.err
	}
	return m.metrics, false, nil
}

func (m *mockCloudClient) GetEnergyImportForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	return 0, nil
}

func (m *mockCloudClient) GetEnergyExportForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	return 0, nil
}

func (m *mockCloudClient) GetProductionForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	return 0, nil
}

func (m *mockCloudClient) GetConsumptionForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	return 0, nil
}

func (m *mockCloudClient) GetBatteryDataForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (charged float64, discharged float64, soc int, err error) {
	return 0, 0, 0, nil
}

// makeRunConfig creates a RunConfig for tests with QueryModeDay and zero TestDate.
func makeRunConfig(agg *aggregator.DataAggregator, disp *display.Display, cfg *config.Config, tz *time.Location) RunConfig {
	return RunConfig{Agg: agg, Pool: credentials.NewPool(cfg.Credentials), Disp: disp, Cfg: cfg, QueryMode: constants.QueryModeDay, ReportTZ: tz}
}

// createMockAggregator creates an aggregator with a mock cloud client
func createMockAggregator(metrics *api.LocalMetrics, err error) *aggregator.DataAggregator {
	mockGetter := func(ctx context.Context, apiConfig *types.APIConfig) (string, error) {
		return "mock-token", nil
	}
	return aggregator.NewDataAggregatorWithFactory(mockGetter, func(systemID, systemName, apiKey, accessToken, credName string, tz *time.Location, budget api.BudgetTracker) aggregator.CloudClient {
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
		Credentials: []*types.APIConfig{{
			Name:         "key1",
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RefreshToken: "test-refresh",
		}},
		RefreshIntervalSeconds: 3600,
	}

	// Create display with buffer
	tz := time.UTC
	buf := &bytes.Buffer{}
	disp := display.NewDisplayWithWriter(display.GetDefaultColors(), tz, buf)

	err := RunOnce(ctx, makeRunConfig(agg, disp, cfg, tz))
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
