// Package aggregator - aggregator_bench_test.go
//
// PURPOSE
// -------
// Performance benchmarks for aggregator functions.
// Measures multi-system data aggregation performance.
//
// RUNNING BENCHMARKS
// ------------------
// Run all benchmarks:
//   go test -bench=. ./internal/aggregator
//
// With memory profiling:
//   go test -bench=. -benchmem -cpuprofile=cpu.prof ./internal/aggregator

// Package aggregator - aggregator_bench_test.go
//
// TEST SETUP
// ----------
// This file contains benchmark tests for aggregation performance measurement.
// Uses mock clients to isolate aggregation logic from API call overhead.
//
// BENCHMARK PLAN
// --------------
// 1. Single-System Aggregation
//    - Benchmark aggregating metrics from one system
//    - Baseline performance measurement
//
// 2. Multi-System Aggregation
//    - Benchmark 2 systems (typical use case)
//    - Benchmark 5 systems (large deployment)
//    - Verify linear scaling with system count
//
// 3. Memory Usage
//    - Measure allocations per aggregation
//    - Verify slice capacity pre-allocation works
//
// RUNNING BENCHMARKS
// ------------------
// Run aggregator benchmarks:
//   go test -bench=. ./internal/aggregator
//
// Run with memory profiling:
//   go test -bench=. -benchmem -memprofile=mem.prof ./internal/aggregator
//   go tool pprof mem.prof
//
// Compare different system counts:
//   go test -bench=BenchmarkGetAggregatedMetrics ./internal/aggregator
//
// PERFORMANCE EXPECTATIONS
// ------------------------
// Aggregation should be fast (< 1 µs) since it's just:
// - Iterating over systems
// - Summing float64 values
// - No I/O or complex calculations
//
// PATTERN USED
// ------------
// - Pattern 9: Benchmark Tests (testing.B)
// - Pattern 2: Mock Objects (MockCloudClient)
//
// See TESTING.md for detailed pattern explanations.
package aggregator

import (
	"context"
	"testing"
	"time"

	"enphase-monitor/internal/api"
	"enphase-monitor/internal/types"
)

// mockCloudClient for benchmarking (returns pre-computed metrics)
type mockCloudClientBench struct {
	metrics *api.LocalMetrics
}

func (m *mockCloudClientBench) GetMetricsFromCloud(ctx context.Context, date time.Time) (*api.LocalMetrics, bool, error) {
	return m.metrics, false, nil
}

func (m *mockCloudClientBench) GetEnergyImportForDate(ctx context.Context, date time.Time) (float64, error) {
	return m.metrics.GridImportToday, nil
}

func (m *mockCloudClientBench) GetEnergyExportForDate(ctx context.Context, date time.Time) (float64, error) {
	return m.metrics.GridExportToday, nil
}

func (m *mockCloudClientBench) GetProductionForDate(ctx context.Context, date time.Time) (float64, error) {
	return m.metrics.ProductionToday, nil
}

func (m *mockCloudClientBench) GetConsumptionForDate(ctx context.Context, date time.Time) (float64, error) {
	return m.metrics.ConsumptionToday, nil
}

func (m *mockCloudClientBench) GetBatteryDataForDate(ctx context.Context, date time.Time) (float64, float64, int, error) {
	return m.metrics.BatteryChargedToday, m.metrics.BatteryDischargedToday, m.metrics.BatterySOC, nil
}

// BenchmarkGetAggregatedMetrics_SingleSystem benchmarks single system aggregation
func BenchmarkGetAggregatedMetrics_SingleSystem(b *testing.B) {
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

	mockFactory := func(systemID, apiKey, accessToken string, tz *time.Location) api.CloudClient {
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
		_, err := agg.GetAggregatedMetrics(ctx, systems, apiConfig, time.Time{}, reportTZ)
		if err != nil {
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

	mockFactory := func(systemID, apiKey, accessToken string, tz *time.Location) api.CloudClient {
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
		_, err := agg.GetAggregatedMetrics(ctx, systems, apiConfig, time.Time{}, reportTZ)
		if err != nil {
			b.Fatalf("GetAggregatedMetrics failed: %v", err)
		}
	}
}

// BenchmarkNetImportCalculation benchmarks the net import calculation
func BenchmarkNetImportCalculation(b *testing.B) {
	gridImport := 125.5
	gridExport := 45.8

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gridImport - gridExport
	}
}
