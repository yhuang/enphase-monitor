// sanity.go cross-checks freshly aggregated PG&E days before they are written, so
// a parser or feed regression shows up as a warning at pull time instead of as a
// silently wrong year of data.
package pge

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"enphase-monitor/internal/history"
)

// expectedIntervals is the number of 15-minute slots in the local calendar day
// date: 96 normally, 92 on spring-forward and 100 on fall-back.
func expectedIntervals(date string, loc *time.Location) int {
	day, err := time.ParseInLocation(dateFormat, date, loc)
	if err != nil {
		return 96
	}
	return int(day.AddDate(0, 0, 1).Sub(day) / (15 * time.Minute))
}

// enphaseTolerance is the largest accepted gap between PG&E's and Enphase's net
// (import − export) for a date, as a fraction of that date's larger gross flow.
// Net alone is a poor yardstick (it can be near zero while both flows are large),
// and the two meters legitimately differ by a few percent.
const enphaseTolerance = 0.10

// CheckDays returns human-readable warnings for days within [from, to]:
//   - a negative daily import;
//   - an interval with import < 0 and export > 0 (the signature of summing the
//     gross and net registers);
//   - an interval count other than the local day's real length (96, or 92/100 on
//     DST days);
//   - PG&E net differing from Enphase net by more than enphaseTolerance.
//
// compareDir is the history directory holding enphase-<date>.json; empty, or a
// date with no Enphase record, skips only the Enphase comparison.
func CheckDays(days []DayUsage, compareDir string, from, to time.Time, loc *time.Location) []string {
	fromDate, toDate := from.In(loc).Format(dateFormat), to.In(loc).Format(dateFormat)
	var warns []string
	for _, d := range days {
		if d.Date < fromDate || d.Date > toDate {
			continue
		}
		if d.ImportKWh < 0 {
			warns = append(warns, fmt.Sprintf("%s: daily import is negative (%.3f kWh)", d.Date, d.ImportKWh))
		}
		bad := 0
		for _, iv := range d.Intervals {
			if iv.ImportKWh < 0 && iv.ExportKWh > 0 {
				bad++
			}
		}
		if bad > 0 {
			warns = append(warns, fmt.Sprintf("%s: %d interval(s) have import < 0 and export > 0 (gross and net registers mixed?)", d.Date, bad))
		}
		if n, want := len(d.Intervals), expectedIntervals(d.Date, loc); n != want {
			warns = append(warns, fmt.Sprintf("%s: %d intervals, expected %d for this local day (missing data, or PG&E dropped a DST hour)", d.Date, n, want))
		}
		if w := compareEnphase(d, compareDir); w != "" {
			warns = append(warns, w)
		}
	}
	return warns
}

// compareEnphase returns a warning when d's net disagrees with the Enphase record
// for the same date, or "" when it agrees or there is nothing to compare.
func compareEnphase(d DayUsage, dir string) string {
	if dir == "" {
		return ""
	}
	data, err := os.ReadFile(history.Enphase.RecordPath(dir, d.Date))
	if err != nil {
		return ""
	}
	var rec struct {
		Totals struct {
			Import float64 `json:"grid_import_kwh"`
			Export float64 `json:"grid_export_kwh"`
		} `json:"totals"`
	}
	if json.Unmarshal(data, &rec) != nil {
		return ""
	}
	enphaseNet := rec.Totals.Import - rec.Totals.Export
	scale := math.Max(math.Max(rec.Totals.Import, rec.Totals.Export), math.Abs(enphaseNet))
	if scale == 0 {
		return ""
	}
	if diff := math.Abs(d.NetKWh - enphaseNet); diff > enphaseTolerance*scale {
		return fmt.Sprintf("%s: PG&E net %.1f kWh vs Enphase net %.1f kWh differ by %.1f kWh (>%.0f%% of %.1f kWh)",
			d.Date, d.NetKWh, enphaseNet, diff, enphaseTolerance*100, scale)
	}
	return ""
}
