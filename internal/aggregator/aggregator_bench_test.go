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
//   - Benchmark aggregating metrics from one system
//   - Baseline performance measurement
//
// 2. Multi-System Aggregation
//   - Benchmark 2 systems (typical use case)
//   - Benchmark 5 systems (large deployment)
//   - Verify linear scaling with system count
//
// 3. Memory Usage
//   - Measure allocations per aggregation
//   - Verify slice capacity pre-allocation works
//
// RUNNING BENCHMARKS
// ------------------
// Run aggregator benchmarks:
//
//	go test -bench=. ./internal/aggregator
//
// Run with memory profiling:
//
//	go test -bench=. -benchmem -memprofile=mem.prof ./internal/aggregator
//	go tool pprof mem.prof
//
// Compare different system counts:
//
//	go test -bench=BenchmarkGetAggregatedMetrics ./internal/aggregator
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
// - Pattern 8: Benchmark Tests (testing.B)
// - Pattern 2: Mock Objects (MockCloudClient)
//
// See docs/TESTING.md for detailed pattern explanations.
//
// =============================================================================
// PATTERN 8: BENCHMARK TESTS WALKTHROUGH
// =============================================================================
//
// WHAT ARE BENCHMARK TESTS?
// -------------------------
// Benchmarks measure how fast code runs. Unlike regular tests that check
// correctness, benchmarks measure performance by running code many times.
//
// KEY DIFFERENCES FROM REGULAR TESTS:
// -----------------------------------
// 1. Function signature: Benchmark*(b *testing.B) instead of Test*(t *testing.T)
// 2. Uses b.N loop: The testing framework decides how many iterations
// 3. Uses b.ResetTimer(): Excludes setup time from measurements
// 4. Output shows: ns/op (nanoseconds per operation)
//
// HOW GO BENCHMARKS WORK:
// -----------------------
// 1. Go runs your benchmark with a small b.N (like 1, 100, 10000)
// 2. It measures total time taken
// 3. It adjusts b.N up or down to get stable measurements
// 4. Repeats until results stabilize
// 5. Reports average time per operation (ns/op)
//
// RUNNING BENCHMARKS:
// -------------------
//
//	go test -bench=.                    # Run all benchmarks
//	go test -bench=BenchmarkNetImport   # Run specific benchmark
//	go test -bench=. -benchmem          # Also show memory allocations
//	go test -bench=. -count=5           # Run 5 times for consistency
//
// INTERPRETING OUTPUT:
// --------------------
//
//	BenchmarkNetImportCalculation-8   1000000000   0.3150 ns/op
//	│                            │   │            │
//	│                            │   │            └─ Time per operation
//	│                            │   └─ Number of iterations
//	│                            └─ Number of CPU cores
//	└─ Benchmark name
//
// =============================================================================
package aggregator

import (
	"context"
	"testing"
	"time"

	"enphase-monitor/internal/api"
	"enphase-monitor/internal/types"
)

// =============================================================================
// MOCK OBJECT FOR BENCHMARKS
// =============================================================================
//
// WHY USE MOCKS IN BENCHMARKS?
// ----------------------------
// Benchmarks should measure only the code you care about. If your aggregator
// calls real API endpoints, you're benchmarking:
//   - Network latency (variable, slow)
//   - API server response time (variable)
//   - JSON parsing (maybe you care, maybe not)
//   - Your aggregation logic (what you actually want to measure)
//
// By using a mock that returns instantly, we isolate just the aggregation logic.
//
// =============================================================================

// mockCloudClientBench returns pre-computed metrics instantly (no I/O)
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

// =============================================================================
// BENCHMARK FUNCTION WALKTHROUGH
// =============================================================================
//
// ANATOMY OF A BENCHMARK FUNCTION:
// --------------------------------
// 1. Function name starts with "Benchmark" (required)
// 2. Takes *testing.B parameter (not *testing.T)
// 3. Has setup code BEFORE b.ResetTimer()
// 4. Has a loop: for i := 0; i < b.N; i++ { ... }
// 5. The code inside the loop is what gets measured
//
// =============================================================================

// BenchmarkGetAggregatedMetrics_SingleSystem benchmarks single system aggregation
func BenchmarkGetAggregatedMetrics_SingleSystem(b *testing.B) {
	// =========================================================================
	// SETUP PHASE (not measured)
	// =========================================================================
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

	mockFactory := func(systemID, apiKey, accessToken string, tz *time.Location) api.CloudClient {
		return &mockCloudClientBench{metrics: mockMetrics}
	}

	getToken := func(ctx context.Context, apiConfig *types.APIConfig) (string, error) {
		return "mock-token", nil
	}

	agg := NewDataAggregatorWithFactory(getToken, mockFactory)
	ctx := context.Background()
	reportTZ := time.UTC

	// =========================================================================
	// RESET TIMER - Critical!
	// =========================================================================
	// b.ResetTimer() tells Go: "Start measuring from HERE, not from the
	// beginning of the function." Without this, setup time would be included
	// in your benchmark results, making them misleading.
	b.ResetTimer()

	// =========================================================================
	// MEASUREMENT LOOP
	// =========================================================================
	// b.N is controlled by the Go testing framework. It starts small and
	// increases until the benchmark runs long enough for stable measurement.
	// You MUST use b.N - don't hardcode a number like 1000.
	for i := 0; i < b.N; i++ {
		_, err := agg.GetAggregatedMetrics(ctx, systems, apiConfig, time.Time{}, reportTZ)
		if err != nil {
			// b.Fatalf stops the benchmark if something breaks
			// Use this instead of ignoring errors
			b.Fatalf("GetAggregatedMetrics failed: %v", err)
		}
	}
}

// BenchmarkGetAggregatedMetrics_MultiSystem benchmarks multi-system aggregation
//
// COMPARING BENCHMARKS:
// ---------------------
// Running this alongside SingleSystem helps detect scaling issues.
// If SingleSystem is 1000 ns/op and MultiSystem (4 systems) is 4000 ns/op,
// that's linear scaling (good). If it's 16000 ns/op, something is O(n²) (bad).
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
//
// MICRO-BENCHMARKS:
// -----------------
// This is a micro-benchmark - it measures a tiny operation (subtraction).
// Micro-benchmarks are useful for:
//   - Establishing baselines (how fast CAN this be?)
//   - Verifying optimizations work
//   - Understanding compiler behavior
//
// CAUTION: Very fast operations (< 1 ns) can be optimized away by the
// compiler if you're not careful. The `_ = result` prevents this.
func BenchmarkNetImportCalculation(b *testing.B) {
	gridImport := 125.5
	gridExport := 45.8

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// The underscore prevents the compiler from optimizing away
		// this calculation entirely (since the result is "used")
		_ = gridImport - gridExport
	}
}
