// trueup.go implements True-Up Mode (--true-up flag), which fetches energy metrics
// for a full true-up year period in a single API batch using Lifetime Data endpoints.
package app

import (
	"context"
	"fmt"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/timezone"
)

// RunTrueUp calculates and displays energy metrics for a true-up year period.
// trueUpStartStr must be YYYY-MM-DD — the utility-defined start date of the true-up year.
// Full calendar months are used; the query covers the first day of the start month
// through the True-Up Window end in a single API batch (4 metrics per system, battery excluded).
func RunTrueUp(ctx context.Context, rc RunConfig, trueUpStartStr string) error {
	startDate, err := timezone.ParseDateInTimezone(trueUpStartStr, rc.ReportTZ)
	if err != nil {
		return fmt.Errorf("invalid --true-up date %q: use YYYY-MM-DD format", trueUpStartStr)
	}

	// Normalize to the first day of the start month (full months only).
	trueUpStart := time.Date(startDate.Year(), startDate.Month(), 1, 0, 0, 0, 0, rc.ReportTZ)

	now := time.Now().In(rc.ReportTZ)
	endDay := trueUpWindowEnd(trueUpStart, now)
	endDate := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), 23, 59, 59, 0, rc.ReportTZ)

	aggSystems, aggAPIConfig := GetAggregatorTypes(rc.Cfg)
	metrics, err := rc.Agg.GetAggregatedMetrics(ctx, aggSystems, aggAPIConfig, trueUpStart, constants.QueryModeTrueUp, rc.ReportTZ)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}

	// Pass the original startDate (not trueUpStart) so the display can show
	// the user-provided date on the "True-Up Start" line while still computing
	// the data range from the normalized first-of-month boundary.
	report := buildTrueUpReport(metrics, startDate, endDate)
	rc.Disp.ShowTrueUpReport(report)
	return nil
}

// trueUpWindowEnd returns the last day of a True-Up Window.
// For Current Periods (cycle end is still in the future), this is yesterday.
// For Past True-Up Periods, this is the last day of the 12-month window.
func trueUpWindowEnd(trueUpStart, now time.Time) time.Time {
	cycleEnd := trueUpStart.AddDate(1, 0, 0).AddDate(0, 0, -1)
	yesterday := now.AddDate(0, 0, -1)
	if yesterday.Before(cycleEnd) {
		return yesterday
	}
	return cycleEnd
}

// buildTrueUpReport converts AggregatedMetrics from a single true-up batch into a TrueUpReport.
func buildTrueUpReport(m *aggregator.AggregatedMetrics, startDate, endDate time.Time) *aggregator.TrueUpReport {
	report := &aggregator.TrueUpReport{
		StartDate:   startDate,
		EndDate:     endDate,
		Timestamp:   m.Timestamp,
		CacheUsed:   m.CacheUsed,
		GridImport:  m.GridImportToday,
		GridExport:  m.GridExportToday,
		Production:  m.ProductionToday,
		Consumption: m.ConsumptionToday,
		NetFlow:     m.GridImportToday - m.GridExportToday,
	}
	for _, sys := range m.Systems {
		report.Systems = append(report.Systems, aggregator.TrueUpSystemReport{
			Name:        sys.Name,
			ID:          sys.ID,
			GridImport:  sys.GridImportToday,
			GridExport:  sys.GridExportToday,
			Production:  sys.ProductionToday,
			Consumption: sys.ConsumptionToday,
			NetFlow:     sys.GridImportToday - sys.GridExportToday,
		})
	}
	return report
}
