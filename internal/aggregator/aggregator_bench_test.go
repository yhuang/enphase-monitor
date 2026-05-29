package aggregator

import (
	"context"
	"testing"
	"time"

	"enphase-monitor/internal/api"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/types"
)

// mockCloudClientBench returns pre-computed metrics instantly (no I/O)
type mockCloudClientBench struct {
	metrics *api.LocalMetrics
}

func (m *mockCloudClientBench) GetMetricsFromCloud(ctx context.Context, date time.Time, queryMode constants.QueryMode) (*api.LocalMetrics, bool, error) {
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	return m.metrics, false, nil
}

func (m *mockCloudClientBench) GetEnergyImportForDate(ctx context.Context, date time.Time, queryMode constants.QueryMode) (float64, error) {
	return m.metrics.GridImportToday, nil
}

func (m *mockCloudClientBench) GetEnergyExportForDate(ctx context.Context, date time.Time, queryMode constants.QueryMode) (float64, error) {
	return m.metrics.GridExportToday, nil
}

func (m *mockCloudClientBench) GetProductionForDate(ctx context.Context, date time.Time, queryMode constants.QueryMode) (float64, error) {
	return m.metrics.ProductionToday, nil
}

func (m *mockCloudClientBench) GetConsumptionForDate(ctx context.Context, date time.Time, queryMode constants.QueryMode) (float64, error) {
	return m.metrics.ConsumptionToday, nil
}

func (m *mockCloudClientBench) GetBatteryDataForDate(ctx context.Context, date time.Time, queryMode constants.QueryMode) (float64, float64, int, error) {
	return m.metrics.BatteryChargedToday, m.metrics.BatteryDischargedToday, m.metrics.BatterySOC, nil
}

// BenchmarkGetAggregatedMetrics_SingleSystem benchmarks single system aggregation
func BenchmarkGetAggregatedMetrics_SingleSystem(b *testing.B) {
	// Everything before b.ResetTimer() is setup. This time is NOT included
	// in the benchmark results. Put expensive setup here.

	systems := []types.SystemConfig{
		{Name: "System1", ID: "111"},
	}

	apiConfig := &types.APIConfig{
		Key:          "test-key",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RefreshToken: "test-refresh",
	}

	mockMetrics := &api.LocalMetrics{
		ProductionToday:        25.5,
		ConsumptionToday:       30.2,
		GridImportToday:        10.5,
		GridExportToday:        5.8,
		BatteryChargedToday:    8.5,
		BatteryDischargedToday: 6.2,
		BatterySOC:             75,
	}

	mockFactory := func(systemID, systemName, apiKey, accessToken string, tz *time.Location) CloudClient {
		return &mockCloudClientBench{metrics: mockMetrics}
	}

	getToken := func(ctx context.Context, apiConfig *types.APIConfig) (string, error) {
		return "mock-token", nil
	}

	agg := NewDataAggregatorWithFactory(getToken, mockFactory)
	ctx := context.Background()
	reportTZ := time.UTC

	// b.ResetTimer() tells Go: "Start measuring from HERE, not from the
	// beginning of the function." Without this, setup time would be included
	// in your benchmark results, making them misleading.
	b.ResetTimer()

	// b.N is controlled by the Go testing framework. It starts small and
	// increases until the benchmark runs long enough for stable measurement.
	// You MUST use b.N - don't hardcode a number like 1000.
	for i := 0; i < b.N; i++ {
		_, err := agg.GetAggregatedMetrics(ctx, systems, apiConfig, time.Time{}, constants.QueryModeDay, reportTZ)
		if err != nil {
			// b.Fatalf stops the benchmark if something breaks
			// Use this instead of ignoring errors
			b.Fatalf("GetAggregatedMetrics failed: %v", err)
		}
	}
}

// BenchmarkGetAggregatedMetrics_MultiSystem benchmarks multi-system aggregation
func BenchmarkGetAggregatedMetrics_MultiSystem(b *testing.B) {
	systems := []types.SystemConfig{
		{Name: "System1", ID: "111"},
		{Name: "System2", ID: "222"},
		{Name: "System3", ID: "333"},
		{Name: "System4", ID: "444"},
	}

	apiConfig := &types.APIConfig{
		Key:          "test-key",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RefreshToken: "test-refresh",
	}

	mockMetrics := &api.LocalMetrics{
		ProductionToday:        25.5,
		ConsumptionToday:       30.2,
		GridImportToday:        10.5,
		GridExportToday:        5.8,
		BatteryChargedToday:    8.5,
		BatteryDischargedToday: 6.2,
		BatterySOC:             75,
	}

	mockFactory := func(systemID, systemName, apiKey, accessToken string, tz *time.Location) CloudClient {
		return &mockCloudClientBench{metrics: mockMetrics}
	}

	getToken := func(ctx context.Context, apiConfig *types.APIConfig) (string, error) {
		return "mock-token", nil
	}

	agg := NewDataAggregatorWithFactory(getToken, mockFactory)
	ctx := context.Background()
	reportTZ := time.UTC

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := agg.GetAggregatedMetrics(ctx, systems, apiConfig, time.Time{}, constants.QueryModeDay, reportTZ)
		if err != nil {
			b.Fatalf("GetAggregatedMetrics failed: %v", err)
		}
	}
}

// BenchmarkNetFlowCalculation benchmarks the net flow calculation
func BenchmarkNetFlowCalculation(b *testing.B) {
	gridImport := 125.5
	gridExport := 45.8

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// The underscore prevents the compiler from optimizing away
		// this calculation entirely (since the result is "used")
		_ = gridImport - gridExport
	}
}
