// verifyenphaseraw.go is the offline Enphase integrity check: it re-derives each
// history day's per-system energy from the archived raw API responses, with the
// same parsers the live fetch uses, and compares the result with the stored
// record. No network access.
package app

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"enphase-monitor/internal/api"
	"enphase-monitor/internal/history"
	"enphase-monitor/internal/rawstore"
)

// enphaseDayFields maps each Day Mode endpoint to the record field it feeds.
var enphaseDayFields = []struct {
	endpoint string
	label    string
	get      func(history.SystemRec) float64
}{
	{"energy_import_telemetry", "grid import", func(s history.SystemRec) float64 { return s.GridImportKWh }},
	{"energy_export_telemetry", "grid export", func(s history.SystemRec) float64 { return s.GridExportKWh }},
	{"telemetry/production_meter", "production", func(s history.SystemRec) float64 { return s.ProductionKWh }},
	{"telemetry/consumption_meter", "consumption", func(s history.SystemRec) float64 { return s.ConsumptionKWh }},
}

// VerifyEnphaseRawResult summarises VerifyEnphaseRaw.
type VerifyEnphaseRawResult struct {
	Manifest   rawstore.VerifyReport
	Days       int      // history days examined
	Complete   int      // days with every system/endpoint raw body present and matching
	Mismatches []string // "<date> <system> <metric>: ..." findings
	NoRaw      []string // dates with at least one missing raw body
}

// OK reports whether the manifest is clean and every examined day reproduced.
func (r VerifyEnphaseRawResult) OK() bool {
	return r.Manifest.OK() && len(r.Mismatches) == 0 && len(r.NoRaw) == 0
}

// VerifyEnphaseRaw checks raw/enphase against the manifest, then re-derives every
// history/enphase-*.json day from the newest archived body per (system, endpoint,
// date) and compares it with the record. Findings are written to out.
func VerifyEnphaseRaw(historyDir string, out io.Writer) (VerifyEnphaseRawResult, error) {
	var res VerifyEnphaseRawResult
	var err error
	if res.Manifest, err = rawstore.VerifySource("enphase"); err != nil {
		return res, err
	}
	rawstore.PrintReport(out, "enphase", res.Manifest)

	entries, err := rawstore.Entries("enphase")
	if err != nil {
		return res, err
	}
	newest := map[string]rawstore.Entry{} // "system|endpoint|date" → newest entry
	for _, e := range entries {
		sys, ep, date := requestKey(e.Request)
		if sys == "" || ep == "" || date == "" {
			continue
		}
		k := sys + "|" + ep + "|" + date
		if cur, ok := newest[k]; !ok || e.FetchedAt >= cur.FetchedAt {
			newest[k] = e
		}
	}

	dates, err := HistoryDates(historyDir, time.Time{}, time.Time{})
	if err != nil {
		return res, err
	}
	for _, d := range dates {
		rec, err := readEnphaseRecordFrom(historyDir, d)
		if err != nil {
			return res, fmt.Errorf("reading record for %s: %w", d, err)
		}
		res.Days++
		complete := true
		for _, s := range rec.Systems {
			for _, f := range enphaseDayFields {
				e, ok := newest[s.ID+"|"+f.endpoint+"|"+d]
				if !ok {
					complete = false
					continue
				}
				body, err := os.ReadFile(filepath.Join(rawstore.Dir, e.File))
				if err != nil {
					complete = false
					continue
				}
				kwh, _, err := api.DayKWh(f.endpoint, body)
				if err != nil {
					res.Mismatches = append(res.Mismatches, fmt.Sprintf("%s system %s %s: raw body does not parse: %v", d, s.ID, f.label, err))
					complete = false
					continue
				}
				if want := f.get(s); math.Abs(kwh-want) > energyTolerance*2 {
					res.Mismatches = append(res.Mismatches, fmt.Sprintf("%s system %s %s: record %.4f kWh vs raw %.4f kWh", d, s.ID, f.label, want, kwh))
					complete = false
				}
			}
		}
		if complete {
			res.Complete++
		} else if !hasFinding(res.Mismatches, d) {
			res.NoRaw = append(res.NoRaw, d)
		}
	}
	for _, m := range res.Mismatches {
		fmt.Fprintf(out, "≠ %s\n", m)
	}
	for _, d := range res.NoRaw {
		fmt.Fprintf(out, "? %s: raw responses missing for at least one system/endpoint\n", d)
	}
	return res, nil
}

// requestKey pulls system, endpoint and date out of a manifest request such as
// "system=55 endpoint=telemetry/production_meter date=2025-11-08 url=…".
func requestKey(req string) (system, endpoint, date string) {
	for _, tok := range strings.Fields(req) {
		switch {
		case strings.HasPrefix(tok, "system="):
			system = strings.TrimPrefix(tok, "system=")
		case strings.HasPrefix(tok, "endpoint="):
			endpoint = strings.TrimPrefix(tok, "endpoint=")
		case strings.HasPrefix(tok, "date="):
			date = strings.TrimPrefix(tok, "date=")
		}
	}
	return
}

func hasFinding(findings []string, date string) bool {
	for _, f := range findings {
		if strings.HasPrefix(f, date+" ") {
			return true
		}
	}
	return false
}
