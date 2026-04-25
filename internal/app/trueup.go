// trueup.go implements the --true-up query mode, which accumulates energy metrics
// across a true-up year period (full calendar months) with rate-limit-aware pacing.
package app

import (
	"context"
	"fmt"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/timezone"
)

const trueUpWaitSeconds = 65

type trueUpQuery struct {
	Date      time.Time
	QueryType constants.QueryType
	Label     string
}

// RunTrueUp calculates and displays energy metrics for a true-up year period.
// trueUpStartStr must be YYYY-MM-DD — the utility-defined start date of the true-up year.
// Full calendar months are used for accumulation; --date is ignored when --true-up is set.
// A 65-second wait is inserted between queries that triggered live API calls, respecting
// the 10 requests/minute rate limit.
func RunTrueUp(ctx context.Context, rc RunConfig, trueUpStartStr string) error {
	startDate, err := timezone.ParseDateInTimezone(trueUpStartStr, rc.ReportTZ)
	if err != nil {
		return fmt.Errorf("invalid --true-up date %q: use YYYY-MM-DD format", trueUpStartStr)
	}

	now := time.Now().In(rc.ReportTZ)
	yesterday := now.AddDate(0, 0, -1)
	endDate := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 0, rc.ReportTZ)

	report := &aggregator.TrueUpReport{
		StartDate: startDate,
		EndDate:   endDate,
	}

	queries := buildTrueUpQuerySchedule(startDate, rc.ReportTZ)
	aggSystems, aggAPIConfig := GetAggregatorTypes(rc.Cfg)
	needWait := false

	for i, q := range queries {
		if i > 0 && needWait {
			if err := waitWithCountdown(ctx, trueUpWaitSeconds, q.Label, i+1, len(queries)); err != nil {
				clearTrueUpStatus()
				return nil // context cancelled — exit cleanly
			}
		}

		printTrueUpStatus(fmt.Sprintf("Fetching %-30s [%d/%d]", q.Label+"...", i+1, len(queries)))

		metrics, err := rc.Agg.GetAggregatedMetrics(ctx, aggSystems, aggAPIConfig, q.Date, q.QueryType, rc.ReportTZ)
		if err != nil {
			clearTrueUpStatus()
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		needWait = !metrics.AllFromCache
		accumulateTrueUpMetrics(report, metrics)
	}

	clearTrueUpStatus()
	rc.Disp.ShowTrueUpReport(report)
	return nil
}

// buildTrueUpQuerySchedule returns the ordered list of API queries needed to cover
// the true-up year from startDate's month through the current month (same-year case)
// or through the current full year (two-year case).
func buildTrueUpQuerySchedule(startDate time.Time, tz *time.Location) []trueUpQuery {
	return buildTrueUpQueryScheduleAt(startDate, time.Now().In(tz), tz)
}

// buildTrueUpQueryScheduleAt is the testable core of buildTrueUpQuerySchedule;
// callers supply the reference "now" time so tests can use fixed dates.
func buildTrueUpQueryScheduleAt(startDate, now time.Time, tz *time.Location) []trueUpQuery {
	start := startDate.In(tz)
	startYear := start.Year()
	startMonth := start.Month()
	currentYear := now.Year()
	currentMonth := now.Month()

	yesterday := now.AddDate(0, 0, -1)

	var queries []trueUpQuery

	if startYear == currentYear {
		// Simple case: iterate month-by-month within the current year
		for m := startMonth; m <= currentMonth; m++ {
			d := time.Date(startYear, m, 1, 0, 0, 0, 0, tz)
			queries = append(queries, trueUpQuery{
				Date:      d,
				QueryType: constants.QueryTypeMonth,
				Label:     d.Format("January 2006"),
			})
		}
	} else {
		// Two-year case: all months in start year + a single year query for the current year
		for m := startMonth; m <= time.December; m++ {
			d := time.Date(startYear, m, 1, 0, 0, 0, 0, tz)
			queries = append(queries, trueUpQuery{
				Date:      d,
				QueryType: constants.QueryTypeMonth,
				Label:     d.Format("January 2006"),
			})
		}
		d := time.Date(currentYear, time.January, 1, 0, 0, 0, 0, tz)
		queries = append(queries, trueUpQuery{
			Date:      d,
			QueryType: constants.QueryTypeYear,
			Label:     fmt.Sprintf("Year %d (Jan–%s)", currentYear, yesterday.Format("Jan 2")),
		})
	}

	return queries
}

// accumulateTrueUpMetrics adds the metrics from one query period into the running report.
// Systems are matched by ID so ordering differences between queries are handled safely.
func accumulateTrueUpMetrics(report *aggregator.TrueUpReport, metrics *aggregator.AggregatedMetrics) {
	report.GridImport += metrics.GridImportToday
	report.GridExport += metrics.GridExportToday
	report.Production += metrics.ProductionToday
	report.Consumption += metrics.ConsumptionToday

	for _, sys := range metrics.Systems {
		found := false
		for i := range report.Systems {
			if report.Systems[i].ID == sys.ID {
				report.Systems[i].GridImport += sys.GridImportToday
				report.Systems[i].GridExport += sys.GridExportToday
				report.Systems[i].Production += sys.ProductionToday
				report.Systems[i].Consumption += sys.ConsumptionToday
				found = true
				break
			}
		}
		if !found {
			report.Systems = append(report.Systems, aggregator.TrueUpSystemReport{
				Name:        sys.Name,
				ID:          sys.ID,
				GridImport:  sys.GridImportToday,
				GridExport:  sys.GridExportToday,
				Production:  sys.ProductionToday,
				Consumption: sys.ConsumptionToday,
			})
		}
	}

	// Recompute net flows after each accumulation
	report.NetFlow = report.GridImport - report.GridExport
	for i := range report.Systems {
		report.Systems[i].NetFlow = report.Systems[i].GridImport - report.Systems[i].GridExport
	}
}

// printTrueUpStatus overwrites the current terminal line with a status message.
func printTrueUpStatus(msg string) {
	fmt.Printf("\r\033[K  %s", msg)
}

// clearTrueUpStatus erases the current terminal status line.
func clearTrueUpStatus() {
	fmt.Print("\r\033[K")
}

// waitWithCountdown counts down `seconds` seconds, updating the terminal status line
// each second. Returns a non-nil error if the context is cancelled during the wait.
func waitWithCountdown(ctx context.Context, seconds int, nextLabel string, nextIdx, total int) error {
	for remaining := seconds; remaining > 0; remaining-- {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		printTrueUpStatus(fmt.Sprintf("Waiting %2ds — next: %-30s [%d/%d]",
			remaining, nextLabel, nextIdx, total))
		time.Sleep(time.Second)
	}
	return nil
}
