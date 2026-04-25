// Package app - trueup_test.go
//
// TEST PLAN
// ---------
// 1. Query Schedule Tests (buildTrueUpQueryScheduleAt)
//   - Same-year case: correct month range produced
//   - Same-year case: single month when start == current month
//   - Two-year case: months in start year + single year query
//   - Two-year case: start in October produces 3 months + year query
//   - Two-year case: start in December produces 1 month + year query
//   - Verify all queries use the first day of each month
//   - Verify last query in two-year case is QueryTypeYear for current year
//
// 2. Metric Accumulation Tests (accumulateTrueUpMetrics)
//   - First accumulation initialises system entries
//   - Subsequent accumulations sum values correctly
//   - NetFlow = GridImport - GridExport is recomputed after each call
//   - Systems are matched by ID, not by position
//   - Combined totals track the sum across all systems
//
// 3. Context Cancellation Test (waitWithCountdown)
//   - Already-cancelled context returns immediately with an error
package app

import (
	"context"
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/constants"
)

// fixedNow is a stable reference time used across schedule tests:
// April 25, 2026 at noon UTC.
var fixedNow = time.Date(2026, time.April, 25, 12, 0, 0, 0, time.UTC)

// =============================================================================
// Query Schedule Tests
// =============================================================================

func TestBuildTrueUpQuerySchedule_SameYear_MultipleMonths(t *testing.T) {
	// True-up started Jan 15 of the current year; we're in April.
	// Expected: Jan, Feb, Mar, Apr — four month queries.
	startDate := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	queries := buildTrueUpQueryScheduleAt(startDate, fixedNow, time.UTC)

	if len(queries) != 4 {
		t.Fatalf("expected 4 queries, got %d", len(queries))
	}
	for _, q := range queries {
		if q.QueryType != constants.QueryTypeMonth {
			t.Errorf("expected QueryTypeMonth, got %v for %s", q.QueryType, q.Label)
		}
	}
	// First query must be Jan 1 (full month)
	wantFirst := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if !queries[0].Date.Equal(wantFirst) {
		t.Errorf("first query date = %v, want %v", queries[0].Date, wantFirst)
	}
	// Last query must be Apr 1
	wantLast := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	if !queries[3].Date.Equal(wantLast) {
		t.Errorf("last query date = %v, want %v", queries[3].Date, wantLast)
	}
}

func TestBuildTrueUpQuerySchedule_SameYear_SingleMonth(t *testing.T) {
	// True-up started in the same month we are currently in.
	// Expected: one query for the current month only.
	startDate := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	queries := buildTrueUpQueryScheduleAt(startDate, fixedNow, time.UTC)

	if len(queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(queries))
	}
	if queries[0].QueryType != constants.QueryTypeMonth {
		t.Errorf("expected QueryTypeMonth, got %v", queries[0].QueryType)
	}
	wantDate := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	if !queries[0].Date.Equal(wantDate) {
		t.Errorf("query date = %v, want %v", queries[0].Date, wantDate)
	}
}

func TestBuildTrueUpQuerySchedule_TwoYears_JanuaryStart(t *testing.T) {
	// True-up started Jan 15 of the previous year.
	// Expected: 12 monthly queries (Jan–Dec 2025) + 1 year query (2026) = 13.
	startDate := time.Date(2025, time.January, 15, 0, 0, 0, 0, time.UTC)
	queries := buildTrueUpQueryScheduleAt(startDate, fixedNow, time.UTC)

	if len(queries) != 13 {
		t.Fatalf("expected 13 queries, got %d", len(queries))
	}
	// First 12 must all be month queries
	for i := 0; i < 12; i++ {
		if queries[i].QueryType != constants.QueryTypeMonth {
			t.Errorf("queries[%d]: expected QueryTypeMonth, got %v", i, queries[i].QueryType)
		}
	}
	// Last must be a year query for Jan 1, 2026
	last := queries[12]
	if last.QueryType != constants.QueryTypeYear {
		t.Errorf("last query: expected QueryTypeYear, got %v", last.QueryType)
	}
	wantYear := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if !last.Date.Equal(wantYear) {
		t.Errorf("last query date = %v, want %v", last.Date, wantYear)
	}
}

func TestBuildTrueUpQuerySchedule_TwoYears_OctoberStart(t *testing.T) {
	// True-up started Oct 15 of the previous year.
	// Expected: Oct, Nov, Dec 2025 (3 months) + year 2026 = 4 queries.
	startDate := time.Date(2025, time.October, 15, 0, 0, 0, 0, time.UTC)
	queries := buildTrueUpQueryScheduleAt(startDate, fixedNow, time.UTC)

	if len(queries) != 4 {
		t.Fatalf("expected 4 queries, got %d", len(queries))
	}
	wantMonths := []time.Month{time.October, time.November, time.December}
	for i, wantM := range wantMonths {
		if queries[i].Date.Month() != wantM {
			t.Errorf("queries[%d] month = %v, want %v", i, queries[i].Date.Month(), wantM)
		}
		if queries[i].QueryType != constants.QueryTypeMonth {
			t.Errorf("queries[%d]: expected QueryTypeMonth, got %v", i, queries[i].QueryType)
		}
	}
	if queries[3].QueryType != constants.QueryTypeYear {
		t.Errorf("last query: expected QueryTypeYear, got %v", queries[3].QueryType)
	}
}

func TestBuildTrueUpQuerySchedule_TwoYears_DecemberStart(t *testing.T) {
	// True-up started Dec 1 of the previous year.
	// Expected: Dec 2025 (1 month) + year 2026 = 2 queries.
	startDate := time.Date(2025, time.December, 1, 0, 0, 0, 0, time.UTC)
	queries := buildTrueUpQueryScheduleAt(startDate, fixedNow, time.UTC)

	if len(queries) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(queries))
	}
	if queries[0].QueryType != constants.QueryTypeMonth {
		t.Errorf("first query: expected QueryTypeMonth, got %v", queries[0].QueryType)
	}
	if queries[0].Date.Month() != time.December {
		t.Errorf("first query month = %v, want December", queries[0].Date.Month())
	}
	if queries[1].QueryType != constants.QueryTypeYear {
		t.Errorf("second query: expected QueryTypeYear, got %v", queries[1].QueryType)
	}
}

func TestBuildTrueUpQuerySchedule_FirstDayOfMonth(t *testing.T) {
	// All month queries must use the first day of the month regardless of the
	// start date's day-of-month, because we use full months.
	startDate := time.Date(2025, time.March, 17, 0, 0, 0, 0, time.UTC) // mid-month
	queries := buildTrueUpQueryScheduleAt(startDate, fixedNow, time.UTC)

	for _, q := range queries {
		if q.QueryType == constants.QueryTypeMonth && q.Date.Day() != 1 {
			t.Errorf("month query date %v: day = %d, want 1", q.Date, q.Date.Day())
		}
	}
}

// =============================================================================
// Metric Accumulation Tests
// =============================================================================

func TestAccumulateTrueUpMetrics_FirstAccumulation(t *testing.T) {
	report := &aggregator.TrueUpReport{}
	metrics := &aggregator.AggregatedMetrics{
		GridImportToday:  100.0,
		GridExportToday:  40.0,
		ProductionToday:  80.0,
		ConsumptionToday: 160.0,
		Systems: []aggregator.SystemMetrics{
			{Name: "Left Panel", ID: "aaa", GridImportToday: 60, GridExportToday: 20, ProductionToday: 50, ConsumptionToday: 90},
			{Name: "Right Panel", ID: "bbb", GridImportToday: 40, GridExportToday: 20, ProductionToday: 30, ConsumptionToday: 70},
		},
	}

	accumulateTrueUpMetrics(report, metrics)

	if report.GridImport != 100.0 {
		t.Errorf("GridImport = %.1f, want 100.0", report.GridImport)
	}
	if report.GridExport != 40.0 {
		t.Errorf("GridExport = %.1f, want 40.0", report.GridExport)
	}
	if report.Production != 80.0 {
		t.Errorf("Production = %.1f, want 80.0", report.Production)
	}
	if report.Consumption != 160.0 {
		t.Errorf("Consumption = %.1f, want 160.0", report.Consumption)
	}
	if report.NetFlow != 60.0 {
		t.Errorf("NetFlow = %.1f, want 60.0 (net import)", report.NetFlow)
	}
	if len(report.Systems) != 2 {
		t.Fatalf("len(Systems) = %d, want 2", len(report.Systems))
	}
}

func TestAccumulateTrueUpMetrics_SumsAcrossMultipleCalls(t *testing.T) {
	report := &aggregator.TrueUpReport{}

	month1 := &aggregator.AggregatedMetrics{
		GridImportToday:  100.0,
		GridExportToday:  50.0,
		ProductionToday:  200.0,
		ConsumptionToday: 250.0,
		Systems: []aggregator.SystemMetrics{
			{ID: "aaa", GridImportToday: 100, GridExportToday: 50, ProductionToday: 200, ConsumptionToday: 250},
		},
	}
	month2 := &aggregator.AggregatedMetrics{
		GridImportToday:  80.0,
		GridExportToday:  30.0,
		ProductionToday:  150.0,
		ConsumptionToday: 200.0,
		Systems: []aggregator.SystemMetrics{
			{ID: "aaa", GridImportToday: 80, GridExportToday: 30, ProductionToday: 150, ConsumptionToday: 200},
		},
	}

	accumulateTrueUpMetrics(report, month1)
	accumulateTrueUpMetrics(report, month2)

	if report.GridImport != 180.0 {
		t.Errorf("GridImport = %.1f, want 180.0", report.GridImport)
	}
	if report.GridExport != 80.0 {
		t.Errorf("GridExport = %.1f, want 80.0", report.GridExport)
	}
	if report.Production != 350.0 {
		t.Errorf("Production = %.1f, want 350.0", report.Production)
	}
	if report.Consumption != 450.0 {
		t.Errorf("Consumption = %.1f, want 450.0", report.Consumption)
	}
	// NetFlow recalculated: 180 - 80 = 100 (net import)
	if report.NetFlow != 100.0 {
		t.Errorf("NetFlow = %.1f, want 100.0", report.NetFlow)
	}
	if len(report.Systems) != 1 {
		t.Fatalf("len(Systems) = %d, want 1 (same system accumulated)", len(report.Systems))
	}
	if report.Systems[0].GridImport != 180.0 {
		t.Errorf("Systems[0].GridImport = %.1f, want 180.0", report.Systems[0].GridImport)
	}
}

func TestAccumulateTrueUpMetrics_NetFlowNegativeWhenNetExport(t *testing.T) {
	report := &aggregator.TrueUpReport{}
	metrics := &aggregator.AggregatedMetrics{
		GridImportToday: 30.0,
		GridExportToday: 80.0, // more exported than imported → net export
	}

	accumulateTrueUpMetrics(report, metrics)

	if report.NetFlow >= 0 {
		t.Errorf("NetFlow = %.1f, want negative (net export scenario)", report.NetFlow)
	}
	if report.NetFlow != -50.0 {
		t.Errorf("NetFlow = %.1f, want -50.0", report.NetFlow)
	}
}

func TestAccumulateTrueUpMetrics_SystemsMatchedByID(t *testing.T) {
	// Two calls with systems in reversed order — ID matching must keep them
	// accumulated into the correct buckets.
	report := &aggregator.TrueUpReport{}

	first := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{ID: "alpha", GridImportToday: 10},
			{ID: "beta", GridImportToday: 20},
		},
	}
	// Second call has beta before alpha
	second := &aggregator.AggregatedMetrics{
		Systems: []aggregator.SystemMetrics{
			{ID: "beta", GridImportToday: 5},
			{ID: "alpha", GridImportToday: 15},
		},
	}

	accumulateTrueUpMetrics(report, first)
	accumulateTrueUpMetrics(report, second)

	byID := make(map[string]float64)
	for _, s := range report.Systems {
		byID[s.ID] = s.GridImport
	}
	if byID["alpha"] != 25.0 {
		t.Errorf("alpha GridImport = %.1f, want 25.0", byID["alpha"])
	}
	if byID["beta"] != 25.0 {
		t.Errorf("beta GridImport = %.1f, want 25.0", byID["beta"])
	}
}

// =============================================================================
// Context Cancellation Test
// =============================================================================

func TestWaitWithCountdown_CancelledContextReturnsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	err := waitWithCountdown(ctx, 75, "February 2026", 2, 10)
	if err == nil {
		t.Error("waitWithCountdown() with cancelled context: expected non-nil error, got nil")
	}
}
