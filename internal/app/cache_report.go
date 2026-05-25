package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"enphase-monitor/internal/api"
	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/timezone"
)

// ErrCacheIncomplete is returned by RunCacheReport when one or more required
// endpoints are not cached. The diagnostic has already been printed to stdout;
// the caller should exit non-zero without printing the error again.
var ErrCacheIncomplete = errors.New("cached report unavailable: one or more endpoints not cached")

// RunCacheReport checks the on-disk cache for all required endpoints and either
// serves a fully-cached report or prints a diagnostic listing what is missing.
// No live API calls are made regardless of the outcome.
//
// trueUpStart, when non-empty, selects the True-Up Mode path (mirrors RunTrueUp).
// Otherwise rc.TestDate and rc.QueryMode drive the query mode.
func RunCacheReport(ctx context.Context, rc RunConfig, trueUpStart string) error {
	systems, apiConfig := GetAggregatorTypes(rc.Cfg)
	if apiConfig == nil || apiConfig.Key == "" {
		return fmt.Errorf("api.key required to check cache")
	}

	// Determine the effective date and query mode for the cache pre-check.
	// For true-up, mirror the normalization RunTrueUp applies: first of start month.
	effectiveDate := rc.TestDate
	effectiveQueryMode := rc.QueryMode
	if trueUpStart != "" {
		startDate, err := timezone.ParseDateInTimezone(trueUpStart, rc.ReportTZ)
		if err != nil {
			return fmt.Errorf("invalid --true-up date %q: use YYYY-MM-DD format", trueUpStart)
		}
		effectiveDate = time.Date(startDate.Year(), startDate.Month(), 1, 0, 0, 0, 0, rc.ReportTZ)
		effectiveQueryMode = constants.QueryModeTrueUp
	}

	// Check each system's cache coverage.
	statuses := make([]api.SystemCacheStatus, 0, len(systems))
	allComplete := true
	for _, sys := range systems {
		s := api.CheckCacheForSystem(sys.ID, sys.Name, apiConfig.Key, effectiveDate, effectiveQueryMode, rc.ReportTZ)
		statuses = append(statuses, s)
		if !s.AllRequiredPresent() {
			allComplete = false
		}
	}

	if !allComplete {
		printCacheDiagnostic(statuses, effectiveDate, effectiveQueryMode, trueUpStart, rc.TestDate)
		return ErrCacheIncomplete
	}

	// All required endpoints are cached — enable Validation Mode and run the normal path.
	if rc.Debug {
		fmt.Println("CACHE MODE: Serving report from cache, no live API calls")
	}
	cache.SetValidationMode(true)

	if trueUpStart != "" {
		return RunTrueUp(ctx, rc, trueUpStart)
	}
	return RunOnce(ctx, rc, false /* validationMode */)
}

func printCacheDiagnostic(statuses []api.SystemCacheStatus, effectiveDate time.Time, effectiveQueryMode constants.QueryMode, trueUpStart string, originalDate time.Time) {
	label := describePeriod(effectiveDate, effectiveQueryMode, trueUpStart)
	fmt.Printf("CACHE INCOMPLETE for %s:\n\n", label)

	for _, s := range statuses {
		name := s.SystemName
		if name == "" {
			name = s.SystemID
		}
		fmt.Printf("  System %q (%s):\n", name, s.SystemID)
		for _, e := range s.Endpoints {
			var marker, detail string
			switch {
			case e.Present:
				marker = "✓"
				detail = fmt.Sprintf("cached %s ago", formatAge(e.Age))
			case e.Required:
				marker = "✗"
				detail = "not cached"
			default:
				marker = "-"
				detail = "not cached (optional)"
			}
			fmt.Printf("    %s  %-35s %s\n", marker, e.Endpoint, detail)
		}
		fmt.Println()
	}

	fmt.Println("To populate the cache, run:")
	fmt.Printf("  %s\n", populateCommand(trueUpStart, originalDate, effectiveQueryMode))
}

func describePeriod(date time.Time, queryMode constants.QueryMode, trueUpStart string) string {
	if trueUpStart != "" {
		return trueUpStart + " (true-up)"
	}
	if date.IsZero() {
		return "today"
	}
	switch queryMode {
	case constants.QueryModeYear:
		return date.Format("2006")
	case constants.QueryModeMonth:
		return date.Format("2006-01")
	default:
		return date.Format(constants.DateFormat)
	}
}

func populateCommand(trueUpStart string, originalDate time.Time, queryMode constants.QueryMode) string {
	if trueUpStart != "" {
		return "./enphase-monitor --true-up " + trueUpStart
	}
	if originalDate.IsZero() {
		return "./enphase-monitor"
	}
	switch queryMode {
	case constants.QueryModeYear:
		return "./enphase-monitor --date " + originalDate.Format("2006")
	case constants.QueryModeMonth:
		return "./enphase-monitor --date " + originalDate.Format("2006-01")
	default:
		return "./enphase-monitor --date " + originalDate.Format(constants.DateFormat)
	}
}

func formatAge(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
