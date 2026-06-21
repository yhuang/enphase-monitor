// Package history persists per-day energy+weather records as JSON files for
// later offline analysis (e.g. feeding a year of data to an ML model to
// correlate production/consumption with weather).
//
// Each record is one calendar day, written to <dir>/<YYYY-MM-DD>.json. The
// schema is deliberately flat and self-describing — every numeric field names
// its unit (_kwh, _mm, _pct) — so a downstream consumer can interpret a record
// without external documentation.
//
// Records hold only aggregated daily values, not the raw API intervals. Battery
// charge/discharge/SOC are intentionally omitted: they are unavailable for
// historical dates (the lifetime endpoints that serve past days carry no
// battery data), so they would always be zero in a backfill.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/constants"
)

// IndexFileName is the backfill manifest written alongside the day records. It
// is a dotfile so that a consumer globbing the dataset (history/*.json) never
// picks it up — its schema differs from a DayRecord.
const IndexFileName = ".index.json"

// notAttemptedReason annotates a missing day that the current run did not try
// (e.g. it fell outside the requested range or was absent from a prior run).
const notAttemptedReason = "not attempted in last run"

// DayRecord is the JSON document written for a single calendar day.
type DayRecord struct {
	Date    string      `json:"date"` // "YYYY-MM-DD"
	Totals  Totals      `json:"totals"`
	Systems []SystemRec `json:"systems"`
	Weather *WeatherRec `json:"weather,omitempty"`
}

// Totals holds the day's combined energy across all systems, in kWh.
type Totals struct {
	ProductionKWh  float64 `json:"production_kwh"`
	ConsumptionKWh float64 `json:"consumption_kwh"`
	GridImportKWh  float64 `json:"grid_import_kwh"`
	GridExportKWh  float64 `json:"grid_export_kwh"`
	NetFlowKWh     float64 `json:"net_flow_kwh"` // import − export; positive = net import, negative = net export
}

// SystemRec holds one system's daily energy values, in kWh.
type SystemRec struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ProductionKWh  float64 `json:"production_kwh"`
	ConsumptionKWh float64 `json:"consumption_kwh"`
	GridImportKWh  float64 `json:"grid_import_kwh"`
	GridExportKWh  float64 `json:"grid_export_kwh"`
	NetFlowKWh     float64 `json:"net_flow_kwh"`
}

// WeatherRec holds the day's weather aggregates from Open-Meteo. Omitted from
// the record entirely when the weather lookup was unavailable.
type WeatherRec struct {
	TempHigh            float64 `json:"temp_high"`
	TempLow             float64 `json:"temp_low"`
	TempUnit            string  `json:"temp_unit"`    // display symbol, e.g. "°F"
	WeatherCode         int     `json:"weather_code"` // precise WMO code; see weather_codes.json legend (project root)
	Condition           string  `json:"condition"`    // human-readable label (collapses code intensities)
	CloudCoverPct       float64 `json:"cloud_cover_pct"`
	PrecipitationMM     float64 `json:"precipitation_mm"`
	SolarRadiationKWhM2 float64 `json:"solar_radiation_kwh_m2"` // daily shortwave radiation sum
}

// FromMetrics converts post-enrichment AggregatedMetrics into a DayRecord.
//
// It returns an error if metrics.QueryDate is zero: a zero date means "today's
// live report", which has no stable calendar-day key to file under. Backfill
// always supplies a concrete past date, so this only guards against misuse.
func FromMetrics(metrics *aggregator.AggregatedMetrics, tz *time.Location) (DayRecord, error) {
	if metrics.QueryDate.IsZero() {
		return DayRecord{}, errors.New("cannot build history record: query date is zero (today's live report has no stable date key)")
	}

	rec := DayRecord{
		Date: metrics.QueryDate.In(tz).Format(constants.DateFormat),
		Totals: Totals{
			ProductionKWh:  metrics.ProductionToday,
			ConsumptionKWh: metrics.ConsumptionToday,
			GridImportKWh:  metrics.GridImportToday,
			GridExportKWh:  metrics.GridExportToday,
			NetFlowKWh:     metrics.NetFlowToday,
		},
		Systems: make([]SystemRec, 0, len(metrics.Systems)),
	}

	for _, s := range metrics.Systems {
		rec.Systems = append(rec.Systems, SystemRec{
			ID:             s.ID,
			Name:           s.Name,
			ProductionKWh:  s.ProductionToday,
			ConsumptionKWh: s.ConsumptionToday,
			GridImportKWh:  s.GridImportToday,
			GridExportKWh:  s.GridExportToday,
			NetFlowKWh:     s.NetFlowToday,
		})
	}

	if w := metrics.Weather; w != nil {
		rec.Weather = &WeatherRec{
			TempHigh:            w.TempHigh,
			TempLow:             w.TempLow,
			TempUnit:            w.TempUnit,
			WeatherCode:         w.WeatherCode,
			Condition:           w.Condition,
			CloudCoverPct:       w.CloudCoverPct,
			PrecipitationMM:     w.PrecipitationMM,
			SolarRadiationKWhM2: w.SolarRadiation,
		}
	}

	return rec, nil
}

// WriteRecord writes record as indented JSON to <dir>/<date>.json, creating dir
// if needed. An existing file for the same date is overwritten.
func WriteRecord(dir string, record DayRecord) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating history directory %q: %w", dir, err)
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling history record for %s: %w", record.Date, err)
	}
	data = append(data, '\n')

	path := filepath.Join(dir, record.Date+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing history record %q: %w", path, err)
	}
	return nil
}

// Index is the backfill manifest (.index.json): a best-effort catalog of the
// dataset's coverage as of the last backfill, used to spot gaps without listing
// the directory.
type Index struct {
	UpdatedAt string       `json:"updated_at"` // RFC3339, report timezone
	Range     IndexRange   `json:"range"`      // window the dataset intends to cover
	Counts    IndexCounts  `json:"counts"`
	Missing   []MissingDay `json:"missing"` // absent days within Range, with reason
}

// IndexRange is the inclusive date window the manifest describes.
type IndexRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// IndexCounts summarizes present vs missing day records within the range.
type IndexCounts struct {
	Present int `json:"present"`
	Missing int `json:"missing"`
}

// MissingDay is one absent day in the dataset and why it is absent.
type MissingDay struct {
	Date  string `json:"date"`
	Error string `json:"error"`
}

// WriteIndex scans dir for day records and writes the .index.json manifest
// describing coverage over [from, to], expanded to include any records already
// on disk from prior runs. runErrors maps a date (YYYY-MM-DD) to the error from
// the current run; it annotates missing days the run attempted. Dates missing
// for any other reason are marked "not attempted in last run".
func WriteIndex(dir string, from, to time.Time, tz *time.Location, runErrors map[string]string) error {
	present, earliest, latest, err := scanPresentDates(dir, tz)
	if err != nil {
		return fmt.Errorf("scanning history directory %q: %w", dir, err)
	}

	// Grow the reported range to cover both the requested window and whatever is
	// already on disk, so the manifest reflects the whole dataset, not just this
	// run.
	rangeFrom, rangeTo := from.In(tz), to.In(tz)
	if !earliest.IsZero() && earliest.Before(rangeFrom) {
		rangeFrom = earliest
	}
	if !latest.IsZero() && latest.After(rangeTo) {
		rangeTo = latest
	}

	missing := []MissingDay{}
	for day := rangeFrom; !day.After(rangeTo); day = day.AddDate(0, 0, 1) {
		date := day.Format(constants.DateFormat)
		if present[date] {
			continue
		}
		reason := notAttemptedReason
		if e, ok := runErrors[date]; ok {
			reason = e
		}
		missing = append(missing, MissingDay{Date: date, Error: reason})
	}

	idx := Index{
		UpdatedAt: time.Now().In(tz).Format(time.RFC3339),
		Range:     IndexRange{From: rangeFrom.Format(constants.DateFormat), To: rangeTo.Format(constants.DateFormat)},
		Counts:    IndexCounts{Present: len(present), Missing: len(missing)},
		Missing:   missing,
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling history index: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(dir, IndexFileName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing history index %q: %w", path, err)
	}
	return nil
}

// scanPresentDates returns the set of dates (YYYY-MM-DD) that have a day record
// in dir, plus the earliest and latest such date (zero times when none exist).
// Non-date files — including the .index.json manifest — are ignored.
func scanPresentDates(dir string, tz *time.Location) (set map[string]bool, earliest, latest time.Time, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, time.Time{}, time.Time{}, nil
		}
		return nil, time.Time{}, time.Time{}, err
	}

	set = make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		date := strings.TrimSuffix(entry.Name(), ".json")
		day, perr := time.ParseInLocation(constants.DateFormat, date, tz)
		if perr != nil {
			continue // not a YYYY-MM-DD record (e.g. .index.json)
		}
		set[date] = true
		if earliest.IsZero() || day.Before(earliest) {
			earliest = day
		}
		if latest.IsZero() || day.After(latest) {
			latest = day
		}
	}
	return set, earliest, latest, nil
}
