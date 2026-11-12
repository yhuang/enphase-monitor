// verifyenphase.go is the live check that Enphase raw archiving works: it fetches
// one past day from the API, confirms each response was archived (file present,
// manifest line, hash), and confirms the day's totals re-derived from that fetch
// reproduce the stored history record. It never writes history/.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/history"
	"enphase-monitor/internal/rawstore"
	"enphase-monitor/internal/timezone"
)

// energyTolerance is the largest accepted kWh gap between a fresh fetch and the
// stored record (values are stored to a few decimals).
const energyTolerance = 0.0005

// dailyEndpointsPerSystem is the minimum number of API responses one system's
// day query produces (import, export, production, consumption).
const dailyEndpointsPerSystem = 4

// VerifyEnphaseResult summarises VerifyEnphaseArchive.
type VerifyEnphaseResult struct {
	Archived int      // responses archived by this run
	Problems []string // anything wrong; empty means the check passed
}

// VerifyEnphaseArchive fetches day live (spending API quota: about 10 calls for
// two systems) and checks the raw archive and the stored record. day must be a
// completed past day with an existing history record. It returns an error only for
// setup problems; check results are in VerifyEnphaseResult.Problems.
func VerifyEnphaseArchive(ctx context.Context, rc RunConfig, day time.Time, out io.Writer) (VerifyEnphaseResult, error) {
	var res VerifyEnphaseResult
	day = dayStart(day, rc.ReportTZ)
	dateStr := day.Format(constants.DateFormat)
	if !timezone.IsPastPeriod(day, constants.QueryModeDay, rc.ReportTZ) {
		return res, fmt.Errorf("%s is not a completed past day; choose a date before today", dateStr)
	}
	stored, err := readEnphaseRecord(dateStr)
	if err != nil {
		return res, fmt.Errorf("no stored record to compare against for %s: %w (run a backfill for that day first)", dateStr, err)
	}

	before, err := rawstore.ManifestLen()
	if err != nil {
		return res, err
	}

	// Always live, like a backfill: a cache hit would archive nothing.
	cache.SetCacheDisabled(true)
	systems := GetSystems(rc.Cfg)
	fmt.Fprintf(out, "Fetching %s live for %d system(s)…\n", dateStr, len(systems))
	metrics, err := rc.Agg.GetAggregatedMetrics(ctx, systems, rc.Pool, day, constants.QueryModeDay, rc.ReportTZ)
	if err != nil {
		return res, err
	}
	if len(metrics.Systems) != len(systems) {
		return res, fmt.Errorf("incomplete fetch: %d of %d systems returned data", len(metrics.Systems), len(systems))
	}

	// 1. Archiving: new, intact, parseable files for this run.
	entries, err := rawstore.EntriesSince(before)
	if err != nil {
		return res, err
	}
	for _, e := range entries {
		if e.Source != "enphase" {
			continue
		}
		res.Archived++
		if err := rawstore.VerifyEntry(e); err != nil {
			res.Problems = append(res.Problems, err.Error())
			continue
		}
		data, err := os.ReadFile(filepath.Join(rawstore.Dir, e.File))
		var probe map[string]any
		if err != nil || json.Unmarshal(data, &probe) != nil {
			res.Problems = append(res.Problems, fmt.Sprintf("%s: archived body is not valid JSON", e.File))
		}
	}
	if want := dailyEndpointsPerSystem * len(systems); res.Archived < want {
		res.Problems = append(res.Problems, fmt.Sprintf("only %d response(s) archived, expected at least %d (%d per system)", res.Archived, want, dailyEndpointsPerSystem))
	}

	// 2. Processing: totals from this fetch reproduce the stored record.
	fresh, err := history.FromMetrics(metrics, rc.ReportTZ)
	if err != nil {
		return res, err
	}
	res.Problems = append(res.Problems, diffRecords(stored, fresh)...)
	return res, nil
}

// readEnphaseRecord loads history/enphase-<date>.json.
func readEnphaseRecord(date string) (history.DayRecord, error) {
	return readEnphaseRecordFrom(HistoryDir, date)
}

// readEnphaseRecordFrom loads <dir>/enphase-<date>.json.
func readEnphaseRecordFrom(dir, date string) (history.DayRecord, error) {
	var rec history.DayRecord
	data, err := os.ReadFile(history.Enphase.RecordPath(dir, date))
	if err != nil {
		return rec, err
	}
	return rec, json.Unmarshal(data, &rec)
}

// diffRecords lists energy fields (day totals and per-system) that differ by more
// than energyTolerance. Weather is not compared: it is not fetched live here.
func diffRecords(stored, fresh history.DayRecord) []string {
	var out []string
	cmp := func(what string, a, b float64) {
		if math.Abs(a-b) > energyTolerance {
			out = append(out, fmt.Sprintf("%s: stored %.4f kWh vs fresh %.4f kWh", what, a, b))
		}
	}
	cmp("total production", stored.Totals.ProductionKWh, fresh.Totals.ProductionKWh)
	cmp("total consumption", stored.Totals.ConsumptionKWh, fresh.Totals.ConsumptionKWh)
	cmp("total grid import", stored.Totals.GridImportKWh, fresh.Totals.GridImportKWh)
	cmp("total grid export", stored.Totals.GridExportKWh, fresh.Totals.GridExportKWh)
	if len(stored.Systems) != len(fresh.Systems) {
		return append(out, fmt.Sprintf("system count: stored %d vs fresh %d", len(stored.Systems), len(fresh.Systems)))
	}
	byID := map[string]history.SystemRec{}
	for _, s := range stored.Systems {
		byID[s.ID] = s
	}
	for _, f := range fresh.Systems {
		s, ok := byID[f.ID]
		if !ok {
			out = append(out, fmt.Sprintf("system %s is not in the stored record", f.ID))
			continue
		}
		cmp("system "+f.ID+" production", s.ProductionKWh, f.ProductionKWh)
		cmp("system "+f.ID+" consumption", s.ConsumptionKWh, f.ConsumptionKWh)
		cmp("system "+f.ID+" grid import", s.GridImportKWh, f.GridImportKWh)
		cmp("system "+f.ID+" grid export", s.GridExportKWh, f.GridExportKWh)
	}
	return out
}
