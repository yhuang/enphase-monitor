// Package app - trueup_test.go
//
// TEST PLAN
// ---------
// 1. buildTrueUpReport Tests
//   - All combined fields are mapped correctly from AggregatedMetrics
//   - NetFlow = GridImport - GridExport for combined totals
//   - NetFlow = GridImport - GridExport for each per-system entry
//   - Start/End dates are passed through unchanged
//   - Each system in AggregatedMetrics produces a TrueUpSystemReport entry
//   - Net export scenario: NetFlow is negative when export > import
package app

import (
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
)

var (
	trueUpStart = time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	trueUpEnd   = time.Date(2026, time.April, 24, 23, 59, 59, 0, time.UTC)
)

func makeTestMetrics() *aggregator.AggregatedMetrics {
	return &aggregator.AggregatedMetrics{
		GridImportToday:  300.0,
		GridExportToday:  120.0,
		ProductionToday:  500.0,
		ConsumptionToday: 680.0,
		Systems: []aggregator.SystemMetrics{
			{
				Name:             "Left Panel",
				ID:               "aaa",
				GridImportToday:  180.0,
				GridExportToday:  70.0,
				ProductionToday:  300.0,
				ConsumptionToday: 410.0,
			},
			{
				Name:             "Right Panel",
				ID:               "bbb",
				GridImportToday:  120.0,
				GridExportToday:  50.0,
				ProductionToday:  200.0,
				ConsumptionToday: 270.0,
			},
		},
	}
}

func TestTrueUpWindowEnd_InProgressCycle(t *testing.T) {
	start := time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, time.March, 15, 10, 0, 0, 0, time.UTC) // within the 12-month window
	got := trueUpWindowEnd(start, now)
	want := now.AddDate(0, 0, -1) // yesterday
	if !got.Equal(want) {
		t.Errorf("got %v, want yesterday %v", got, want)
	}
}

func TestTrueUpWindowEnd_ClosedCycle(t *testing.T) {
	start := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, time.May, 21, 10, 0, 0, 0, time.UTC) // well past the 12-month window
	got := trueUpWindowEnd(start, now)
	want := time.Date(2024, time.December, 31, 0, 0, 0, 0, time.UTC) // last day of 12-month window
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTrueUpWindowEnd_ExactBoundary(t *testing.T) {
	start := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	// now = the day after the cycle end (2026-01-01); yesterday = 2025-12-31 = cycleEnd
	now := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
	got := trueUpWindowEnd(start, now)
	want := time.Date(2025, time.December, 31, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildTrueUpReport_CombinedFieldsMapped(t *testing.T) {
	m := makeTestMetrics()
	report := buildTrueUpReport(m, trueUpStart, trueUpEnd)

	if report.GridImport != 300.0 {
		t.Errorf("GridImport = %.1f, want 300.0", report.GridImport)
	}
	if report.GridExport != 120.0 {
		t.Errorf("GridExport = %.1f, want 120.0", report.GridExport)
	}
	if report.Production != 500.0 {
		t.Errorf("Production = %.1f, want 500.0", report.Production)
	}
	if report.Consumption != 680.0 {
		t.Errorf("Consumption = %.1f, want 680.0", report.Consumption)
	}
}

func TestBuildTrueUpReport_NetFlowIsImportMinusExport(t *testing.T) {
	m := makeTestMetrics()
	report := buildTrueUpReport(m, trueUpStart, trueUpEnd)

	want := m.GridImportToday - m.GridExportToday // 180.0
	if report.NetFlow != want {
		t.Errorf("NetFlow = %.1f, want %.1f", report.NetFlow, want)
	}
}

func TestBuildTrueUpReport_NetFlowNegativeOnNetExport(t *testing.T) {
	m := &aggregator.AggregatedMetrics{
		GridImportToday: 50.0,
		GridExportToday: 200.0,
	}
	report := buildTrueUpReport(m, trueUpStart, trueUpEnd)

	if report.NetFlow >= 0 {
		t.Errorf("NetFlow = %.1f, want negative (net export)", report.NetFlow)
	}
	if report.NetFlow != -150.0 {
		t.Errorf("NetFlow = %.1f, want -150.0", report.NetFlow)
	}
}

func TestBuildTrueUpReport_DatesPassedThrough(t *testing.T) {
	m := makeTestMetrics()
	report := buildTrueUpReport(m, trueUpStart, trueUpEnd)

	if !report.StartDate.Equal(trueUpStart) {
		t.Errorf("StartDate = %v, want %v", report.StartDate, trueUpStart)
	}
	if !report.EndDate.Equal(trueUpEnd) {
		t.Errorf("EndDate = %v, want %v", report.EndDate, trueUpEnd)
	}
}

func TestBuildTrueUpReport_SystemsConverted(t *testing.T) {
	m := makeTestMetrics()
	report := buildTrueUpReport(m, trueUpStart, trueUpEnd)

	if len(report.Systems) != 2 {
		t.Fatalf("len(Systems) = %d, want 2", len(report.Systems))
	}

	byID := make(map[string]aggregator.TrueUpSystemReport)
	for _, s := range report.Systems {
		byID[s.ID] = s
	}

	left := byID["aaa"]
	if left.Name != "Left Panel" {
		t.Errorf("aaa.Name = %q, want %q", left.Name, "Left Panel")
	}
	if left.GridImport != 180.0 {
		t.Errorf("aaa.GridImport = %.1f, want 180.0", left.GridImport)
	}
	if left.GridExport != 70.0 {
		t.Errorf("aaa.GridExport = %.1f, want 70.0", left.GridExport)
	}
	if left.Production != 300.0 {
		t.Errorf("aaa.Production = %.1f, want 300.0", left.Production)
	}
	if left.Consumption != 410.0 {
		t.Errorf("aaa.Consumption = %.1f, want 410.0", left.Consumption)
	}
}

func TestBuildTrueUpReport_PerSystemNetFlow(t *testing.T) {
	m := makeTestMetrics()
	report := buildTrueUpReport(m, trueUpStart, trueUpEnd)

	byID := make(map[string]aggregator.TrueUpSystemReport)
	for _, s := range report.Systems {
		byID[s.ID] = s
	}

	// aaa: 180 - 70 = 110
	if byID["aaa"].NetFlow != 110.0 {
		t.Errorf("aaa.NetFlow = %.1f, want 110.0", byID["aaa"].NetFlow)
	}
	// bbb: 120 - 50 = 70
	if byID["bbb"].NetFlow != 70.0 {
		t.Errorf("bbb.NetFlow = %.1f, want 70.0", byID["bbb"].NetFlow)
	}
}
