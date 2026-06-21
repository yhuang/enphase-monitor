// backfill.go implements Backfill Mode: fetching a contiguous range of past
// days, one calendar day at a time, and persisting each as a JSON record under
// history/ for later offline analysis.
//
// Unlike the normal report path, Backfill Mode always makes live API calls
// (cache is disabled for the run). Historical day data is immutable, so the
// records it produces are authoritative rather than whatever happened to be
// cached. The credential pool's combined budget (one set per system, many sets
// available) comfortably absorbs a year of daily queries.
//
// The day-by-day fetch is intentionally sequential, not parallelized across the
// credential pool. A year backfill is a rare, run-in-the-background operation,
// and serial fetching keeps each key far below the per-minute rate-limit ceiling
// with no burst coordination — minimizing any chance of a 429. Do not parallelize
// this loop for speed without first solving rate-limit coordination across workers.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/history"
	"enphase-monitor/internal/timezone"
)

// HistoryDir is the directory, relative to the working directory, where backfill
// writes per-day JSON records. Fixed by convention (mirrors the cache/ layout).
const HistoryDir = "history"

// RunBackfill fetches every day from fromDate through the end date (inclusive),
// writing one history record per day. The end date is rc.TestDate when set,
// otherwise yesterday. force re-fetches and overwrites days already on disk;
// when false, existing records are skipped without any API calls.
//
// Per-day failures are reported and skipped so a single bad day does not abort
// the whole range. RunBackfill returns an error only for setup problems (an
// invalid range), not for individual-day failures.
func RunBackfill(ctx context.Context, rc RunConfig, fromDate time.Time, force bool) error {
	end := rc.TestDate
	if end.IsZero() {
		end = time.Now().In(rc.ReportTZ).AddDate(0, 0, -1)
	}

	// Normalize both bounds to midnight in the report timezone so the day-by-day
	// walk lands on calendar boundaries regardless of the parsed time-of-day.
	from := dayStart(fromDate, rc.ReportTZ)
	end = dayStart(end, rc.ReportTZ)

	if end.Before(from) {
		return fmt.Errorf("backfill range is empty: from %s is after end %s",
			from.Format(constants.DateFormat), end.Format(constants.DateFormat))
	}
	if !timezone.IsPastPeriod(end, constants.QueryModeDay, rc.ReportTZ) {
		return fmt.Errorf("backfill end date %s is not a completed past day; choose an end date before today",
			end.Format(constants.DateFormat))
	}

	// Backfill is authoritative: always hit the live API, never the cache.
	cache.SetCacheDisabled(true)

	// On a terminal, progress for written/skipped days redraws on a single line
	// (carriage return + clear-to-end-of-line) so a long range scrolls in place.
	// When stdout is redirected (not a TTY), fall back to plain newline-
	// terminated lines so no control codes leak into captured output. Errors and
	// the final summary always commit to their own lines.
	tty := isTerminal(os.Stdout)
	pending := false // true when an in-place progress line awaits a newline

	// redraw prints a transient progress line: overwriting in place on a TTY,
	// or an ordinary line otherwise.
	redraw := func(format string, args ...any) {
		if tty {
			fmt.Printf("\r\033[K"+format, args...)
			pending = true
		} else {
			fmt.Printf(format+"\n", args...)
		}
	}

	var written, skipped, errored int
	runErrors := make(map[string]string) // date -> error, for the manifest
	for day := from; !day.After(end); day = day.AddDate(0, 0, 1) {
		dateStr := day.Format(constants.DateFormat)
		path := filepath.Join(HistoryDir, dateStr+".json")

		if !force {
			if _, err := os.Stat(path); err == nil {
				redraw("• %s (already present, skipping)", dateStr)
				skipped++
				continue
			}
		}

		if err := backfillDay(ctx, rc, day); err != nil {
			if tty {
				fmt.Print("\r\033[K")
			}
			fmt.Printf("✗ %s: %v\n", dateStr, err)
			pending = false
			errored++
			runErrors[dateStr] = err.Error()
			// A cancelled context means shutdown; stop rather than churn errors.
			if ctx.Err() != nil {
				break
			}
			continue
		}
		redraw("✓ %s", dateStr)
		written++
	}

	if pending {
		fmt.Println()
	}

	// Refresh the manifest from disk so it reflects the whole dataset (this run
	// plus any prior runs), not just this run's counters.
	if err := history.WriteIndex(HistoryDir, from, end, rc.ReportTZ, runErrors); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not write history index: %v\n", err)
	}

	fmt.Printf("Backfill complete: %d written, %d skipped, %d errors\n", written, skipped, errored)
	if errored > 0 {
		fmt.Printf("%d day(s) missing — re-run the same command to retry just those (see %s).\n",
			errored, filepath.Join(HistoryDir, history.IndexFileName))
	}
	return nil
}

// isTerminal reports whether f refers to a character device (a terminal), used
// to decide whether in-place progress redraws are safe. Returns false when the
// stream is a pipe or regular file.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// backfillDay fetches one day's metrics, enriches them with weather, and writes
// the history record.
//
// Weather is a backfill invariant: a day whose weather could not be fetched is
// treated as a failure (no record written) rather than persisting a weatherless
// record. This keeps every History Record usable for weather correlation and
// lets a plain re-run retry the day. Contrast with a live --date report, where
// weather enrichment is purely best-effort.
func backfillDay(ctx context.Context, rc RunConfig, day time.Time) error {
	metrics, err := rc.Agg.GetAggregatedMetrics(ctx, GetSystems(rc.Cfg), rc.Pool, day, constants.QueryModeDay, rc.ReportTZ)
	if err != nil {
		return err
	}

	enrichWithTemperature(ctx, rc, metrics)

	record, err := history.FromMetrics(metrics, rc.ReportTZ)
	if err != nil {
		return err
	}
	if record.Weather == nil {
		return errors.New("weather unavailable for this day")
	}
	return history.WriteRecord(HistoryDir, record)
}

// dayStart returns midnight of t's calendar day in tz.
func dayStart(t time.Time, tz *time.Location) time.Time {
	t = t.In(tz)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, tz)
}
