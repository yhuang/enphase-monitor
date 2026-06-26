// history.go writes the aggregated PG&E daily usage into the shared history/
// directory as per-day records, alongside the Enphase records, using the "pge"
// dataset prefix (history/pge-YYYY-MM-DD.json + history/.pge-index.json).
package pge

import (
	"fmt"
	"time"

	"enphase-monitor/internal/history"
)

// IntervalRecord is one 15-minute metering slot within a day. Both import and
// export channels are merged into a single record per time slot so a consumer
// can compute net flow at interval granularity.
type IntervalRecord struct {
	Start     string  `json:"start"`      // RFC3339 in Pacific time
	End       string  `json:"end"`        // RFC3339 in Pacific time
	ImportKWh float64 `json:"import_kwh"` // energy drawn from the grid this interval
	ExportKWh float64 `json:"export_kwh"` // energy pushed to the grid this interval
}

// DayUsage is the daily aggregate built from interval readings before being
// written as a history record.
type DayUsage struct {
	Date      string
	ImportKWh float64
	ExportKWh float64
	NetKWh    float64 // import − export
	Intervals []IntervalRecord
}

// historyRecord is the JSON document written for one PG&E day. The schema is
// flat at the top level for the daily totals, with the 15-minute interval
// readings preserved in the intervals array so time-of-day analysis is possible.
// Cost is absent: the ESPI XML format carries no per-interval cost data.
type historyRecord struct {
	Date      string           `json:"date"`       // YYYY-MM-DD
	ImportKWh float64          `json:"import_kwh"` // daily total drawn from grid
	ExportKWh float64          `json:"export_kwh"` // daily total pushed to grid
	NetKWh    float64          `json:"net_kwh"`    // import − export; positive = net import
	Intervals []IntervalRecord `json:"intervals"`  // 15-min slots, sorted by start time
}

// WriteHistory writes one pge-YYYY-MM-DD.json record per day into dir and
// refreshes the .pge-index.json manifest for the covered range [from, to]. It
// returns the number of day records written. Existing records for the same dates
// are overwritten (a re-pull is authoritative).
//
// Days whose date falls outside [from, to] are silently skipped. PG&E's ESPI
// export sometimes includes a stray reading at the boundary of the next day
// (e.g. midnight June 1 when the requested range ends May 31); filtering here
// keeps the written set exactly equal to the requested range.
func WriteHistory(dir string, days []DayUsage, from, to time.Time, tz *time.Location) (int, error) {
	fromDate := from.In(tz).Format(dateFormat)
	toDate := to.In(tz).Format(dateFormat)
	written := 0
	for _, d := range days {
		if d.Date < fromDate || d.Date > toDate {
			continue
		}
		rec := historyRecord{
			Date:      d.Date,
			ImportKWh: d.ImportKWh,
			ExportKWh: d.ExportKWh,
			NetKWh:    d.NetKWh,
			Intervals: d.Intervals,
		}
		if err := history.WriteRecord(dir, history.PGE, d.Date, rec); err != nil {
			return written, fmt.Errorf("writing PG&E record for %s: %w", d.Date, err)
		}
		written++
	}

	if err := history.WriteIndex(dir, history.PGE, from, to, tz, nil); err != nil {
		return written, fmt.Errorf("writing PG&E history index: %w", err)
	}
	return written, nil
}
