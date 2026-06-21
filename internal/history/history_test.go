package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/constants"
)

// sampleMetrics builds an AggregatedMetrics for a fixed past day with two
// systems and weather, mirroring what a backfill day query produces.
func sampleMetrics(tz *time.Location) *aggregator.AggregatedMetrics {
	return &aggregator.AggregatedMetrics{
		QueryDate:        time.Date(2026, 1, 15, 0, 0, 0, 0, tz),
		QueryMode:        constants.QueryModeDay,
		ProductionToday:  44.6,
		ConsumptionToday: 49.1,
		GridImportToday:  34.7,
		GridExportToday:  24.3,
		NetFlowToday:     10.4,
		Systems: []aggregator.SystemMetrics{
			{
				Name:             "Right Subpanel",
				ID:               "5525881",
				ProductionToday:  20.8,
				ConsumptionToday: 26.7,
				GridImportToday:  18.4,
				GridExportToday:  11.0,
				NetFlowToday:     7.4,
				// Battery fields are intentionally set but must NOT appear in the record.
				BatterySOC:             82,
				BatteryChargedToday:    7.7,
				BatteryDischargedToday: 6.0,
			},
			{
				Name:             "Left Subpanel",
				ID:               "5392556",
				ProductionToday:  23.8,
				ConsumptionToday: 22.4,
				GridImportToday:  16.3,
				GridExportToday:  13.3,
				NetFlowToday:     3.0,
			},
		},
		Weather: &aggregator.DailyWeather{
			TempHigh:        58.2,
			TempLow:         42.1,
			TempUnit:        "°F",
			WeatherCode:     2,
			Condition:       "Partly Cloudy",
			CloudCoverPct:   34,
			PrecipitationMM: 0,
			SolarRadiation:  2.4,
		},
	}
}

func TestFromMetrics(t *testing.T) {
	tz := time.UTC
	rec, err := FromMetrics(sampleMetrics(tz), tz)
	if err != nil {
		t.Fatalf("FromMetrics returned error: %v", err)
	}

	if rec.Date != "2026-01-15" {
		t.Errorf("Date = %q, want 2026-01-15", rec.Date)
	}
	if rec.Totals.ProductionKWh != 44.6 {
		t.Errorf("Totals.ProductionKWh = %v, want 44.6", rec.Totals.ProductionKWh)
	}
	if rec.Totals.NetFlowKWh != 10.4 {
		t.Errorf("Totals.NetFlowKWh = %v, want 10.4", rec.Totals.NetFlowKWh)
	}
	if len(rec.Systems) != 2 {
		t.Fatalf("len(Systems) = %d, want 2", len(rec.Systems))
	}

	first := rec.Systems[0]
	if first.ID != "5525881" || first.Name != "Right Subpanel" {
		t.Errorf("Systems[0] id/name = %q/%q", first.ID, first.Name)
	}
	if first.ProductionKWh != 20.8 || first.GridExportKWh != 11.0 {
		t.Errorf("Systems[0] energy mismatch: %+v", first)
	}

	if rec.Weather == nil {
		t.Fatal("Weather is nil, want populated")
	}
	if rec.Weather.SolarRadiationKWhM2 != 2.4 || rec.Weather.TempUnit != "°F" || rec.Weather.WeatherCode != 2 {
		t.Errorf("Weather mismatch: %+v", rec.Weather)
	}
}

// TestFromMetrics_OmitsBatteryFields verifies the serialized JSON carries no
// battery keys, since they are unavailable for historical dates.
func TestFromMetrics_OmitsBatteryFields(t *testing.T) {
	rec, err := FromMetrics(sampleMetrics(time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("FromMetrics returned error: %v", err)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	lower := strings.ToLower(string(data))
	for _, banned := range []string{"battery", "soc", "charged", "discharged"} {
		if strings.Contains(lower, banned) {
			t.Errorf("record JSON unexpectedly contains %q: %s", banned, data)
		}
	}
}

// TestFromMetrics_ZeroDate rejects today's live report (zero query date).
func TestFromMetrics_ZeroDate(t *testing.T) {
	m := &aggregator.AggregatedMetrics{QueryMode: constants.QueryModeDay}
	if _, err := FromMetrics(m, time.UTC); err == nil {
		t.Fatal("expected error for zero QueryDate, got nil")
	}
}

// TestFromMetrics_NilWeather leaves the weather field absent.
func TestFromMetrics_NilWeather(t *testing.T) {
	m := sampleMetrics(time.UTC)
	m.Weather = nil
	rec, err := FromMetrics(m, time.UTC)
	if err != nil {
		t.Fatalf("FromMetrics returned error: %v", err)
	}
	if rec.Weather != nil {
		t.Errorf("Weather = %+v, want nil", rec.Weather)
	}
}

func TestWriteRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	rec, err := FromMetrics(sampleMetrics(time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("FromMetrics: %v", err)
	}
	if err := WriteRecord(dir, rec); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}

	path := filepath.Join(dir, "2026-01-15.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written record: %v", err)
	}

	var got DayRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshaling written record: %v", err)
	}
	if got.Date != "2026-01-15" || len(got.Systems) != 2 || got.Weather == nil {
		t.Errorf("round-tripped record mismatch: %+v", got)
	}
}

// TestWriteRecord_Overwrites confirms a second write replaces the file.
func TestWriteRecord_Overwrites(t *testing.T) {
	dir := t.TempDir()
	rec, _ := FromMetrics(sampleMetrics(time.UTC), time.UTC)
	if err := WriteRecord(dir, rec); err != nil {
		t.Fatalf("first WriteRecord: %v", err)
	}
	rec.Totals.ProductionKWh = 99.9
	if err := WriteRecord(dir, rec); err != nil {
		t.Fatalf("second WriteRecord: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "2026-01-15.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got DayRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Totals.ProductionKWh != 99.9 {
		t.Errorf("ProductionKWh = %v, want 99.9 (overwrite failed)", got.Totals.ProductionKWh)
	}
}

// writeDayFile drops a minimal day record so index scanning has something to find.
func writeDayFile(t *testing.T, dir, date string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, date+".json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("writing %s: %v", date, err)
	}
}

// readIndex reads and decodes the manifest written to dir.
func readIndex(t *testing.T, dir string) Index {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, IndexFileName))
	if err != nil {
		t.Fatalf("reading index: %v", err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("unmarshaling index: %v", err)
	}
	return idx
}

func TestWriteIndex(t *testing.T) {
	dir := t.TempDir()
	// Present: Jan 13 and Jan 15. Missing within range: Jan 14 (attempted, failed)
	// and Jan 16 (not attempted this run).
	writeDayFile(t, dir, "2026-01-13")
	writeDayFile(t, dir, "2026-01-15")

	from := time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
	runErrors := map[string]string{"2026-01-14": "API request failed with status 503"}

	if err := WriteIndex(dir, from, to, time.UTC, runErrors); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}

	idx := readIndex(t, dir)
	if idx.Range.From != "2026-01-13" || idx.Range.To != "2026-01-16" {
		t.Errorf("Range = %s..%s, want 2026-01-13..2026-01-16", idx.Range.From, idx.Range.To)
	}
	if idx.Counts.Present != 2 || idx.Counts.Missing != 2 {
		t.Errorf("Counts = %+v, want present 2 missing 2", idx.Counts)
	}
	if len(idx.Missing) != 2 {
		t.Fatalf("Missing len = %d, want 2", len(idx.Missing))
	}
	if idx.Missing[0].Date != "2026-01-14" || idx.Missing[0].Error != "API request failed with status 503" {
		t.Errorf("Missing[0] = %+v, want attempted-error for 01-14", idx.Missing[0])
	}
	if idx.Missing[1].Date != "2026-01-16" || idx.Missing[1].Error != notAttemptedReason {
		t.Errorf("Missing[1] = %+v, want not-attempted for 01-16", idx.Missing[1])
	}
}

// TestWriteIndex_ExpandsRangeForPriorRecords confirms records on disk outside
// the requested window still widen the reported range (incremental runs).
func TestWriteIndex_ExpandsRangeForPriorRecords(t *testing.T) {
	dir := t.TempDir()
	writeDayFile(t, dir, "2025-12-30") // from a prior run, before this run's window

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := WriteIndex(dir, from, to, time.UTC, nil); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}

	idx := readIndex(t, dir)
	if idx.Range.From != "2025-12-30" {
		t.Errorf("Range.From = %s, want 2025-12-30 (expanded for prior record)", idx.Range.From)
	}
	if idx.Counts.Present != 1 {
		t.Errorf("Counts.Present = %d, want 1", idx.Counts.Present)
	}
}

// TestWriteIndex_IgnoredByDatasetGlob confirms the manifest is a dotfile and is
// not itself parsed as a day record on a later scan.
func TestWriteIndex_DotfileNotCountedAsRecord(t *testing.T) {
	dir := t.TempDir()
	writeDayFile(t, dir, "2026-01-15")
	from := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if err := WriteIndex(dir, from, from, time.UTC, nil); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}
	// Second write must still count exactly one record, not two (ignoring .index.json).
	if err := WriteIndex(dir, from, from, time.UTC, nil); err != nil {
		t.Fatalf("WriteIndex 2: %v", err)
	}
	idx := readIndex(t, dir)
	if idx.Counts.Present != 1 {
		t.Errorf("Counts.Present = %d, want 1 (manifest must not count itself)", idx.Counts.Present)
	}
}
