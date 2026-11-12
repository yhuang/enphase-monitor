package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"enphase-monitor/internal/history"
	"enphase-monitor/internal/rawstore"
)

func TestHistoryDates_RangeUsesCalendarDates(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"2025-11-07", "2025-11-08", "2025-11-09"} {
		if err := os.WriteFile(filepath.Join(dir, "enphase-"+d+".json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("no tzdata")
	}
	day := time.Date(2025, 11, 8, 0, 0, 0, 0, la) // local midnight is 08:00Z
	got, err := HistoryDates(dir, day, day)
	if err != nil || len(got) != 1 || got[0] != "2025-11-08" {
		t.Fatalf("HistoryDates = %v, %v; want [2025-11-08]", got, err)
	}
	all, _ := HistoryDates(dir, time.Time{}, time.Time{})
	if len(all) != 3 {
		t.Errorf("unbounded = %v, want 3 days", all)
	}
}

func TestApplyRawWeather_UpdatesWeatherOnlyAndOnlyWhenChanged(t *testing.T) {
	hist, raw := t.TempDir(), t.TempDir()
	const rec = `{"date":"2025-11-08","totals":{"production_kwh":1,"consumption_kwh":2,"grid_import_kwh":3,"grid_export_kwh":4,"net_flow_kwh":-1},"systems":[{"id":"9","name":"A","production_kwh":1,"consumption_kwh":2,"grid_import_kwh":3,"grid_export_kwh":4,"net_flow_kwh":-1}],"weather":{"temp_high":70,"temp_low":50,"temp_unit":"°F","weather_code":0,"condition":"Clear","cloud_cover_pct":0,"humidity_pct":0,"humidity_max_pct":0,"humidity_min_pct":0,"precipitation_mm":0,"solar_radiation_kwh_m2":0}}`
	const payload = `{"daily":{"time":["2025-11-08"],"temperature_2m_max":[68.5],"temperature_2m_min":[51.25],"weather_code":[3],"cloud_cover_mean":[40],"relative_humidity_2m_mean":[60],"relative_humidity_2m_max":[90],"relative_humidity_2m_min":[30],"precipitation_sum":[0],"shortwave_radiation_sum":[10.8]},"hourly":{"weather_code":[3,3,3]}}`
	if err := os.WriteFile(filepath.Join(hist, "enphase-2025-11-08.json"), []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(raw, "daily_2025-11-08_20261112T060221.000000Z.json"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	loc := time.UTC
	res := ApplyRawWeather([]string{"2025-11-08"}, hist, raw, loc, io.Discard, false)
	if len(res.Updated) != 1 || len(res.Failed) != 0 {
		t.Fatalf("first apply = %+v", res)
	}
	data, _ := os.ReadFile(filepath.Join(hist, "enphase-2025-11-08.json"))
	var got history.DayRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Weather.TempHigh != 68.5 || got.Weather.TempLow != 51.25 || got.Weather.WeatherCode != 3 || got.Weather.CloudCoverPct != 40 {
		t.Errorf("weather = %+v", got.Weather)
	}
	if got.Totals.GridImportKWh != 3 || got.Systems[0].ID != "9" {
		t.Errorf("energy fields changed: %+v", got)
	}
	if res := ApplyRawWeather([]string{"2025-11-08"}, hist, raw, loc, io.Discard, false); len(res.Updated) != 0 || res.Unchanged != 1 {
		t.Errorf("second apply = %+v, want unchanged", res)
	}
	// Provenance: the first apply recorded a rebuild with the replaced values, and the
	// second (no-op) apply must not erase that history.
	provData, err := os.ReadFile(filepath.Join(hist, WeatherProvenanceFile))
	if err != nil {
		t.Fatalf("no provenance sidecar: %v", err)
	}
	var prov weatherProvenance
	if err := json.Unmarshal(provData, &prov); err != nil {
		t.Fatal(err)
	}
	day := prov.Days["2025-11-08"]
	if day.Status != weatherRebuilt || day.Previous == nil || day.Previous.TempHigh != 70 ||
		len(day.SHA256) != 64 || day.RawFile != "weather/daily_2025-11-08_20261112T060221.000000Z.json" || day.SolarRadiationZero {
		t.Errorf("provenance = %+v, want rebuilt_from_raw with previous temp_high 70", day)
	}
	if res := ApplyRawWeather([]string{"2025-11-09"}, hist, raw, loc, io.Discard, false); len(res.Failed) != 1 {
		t.Errorf("missing payload should fail loudly: %+v", res)
	}
}

func TestDiffRecords(t *testing.T) {
	a := history.DayRecord{Totals: history.Totals{GridImportKWh: 10}, Systems: []history.SystemRec{{ID: "1", GridImportKWh: 10}}}
	b := a
	if d := diffRecords(a, b); len(d) != 0 {
		t.Errorf("identical records differ: %v", d)
	}
	b = history.DayRecord{Totals: history.Totals{GridImportKWh: 10.5}, Systems: []history.SystemRec{{ID: "1", GridImportKWh: 10.5}}}
	if d := diffRecords(a, b); len(d) != 2 {
		t.Errorf("diffs = %v, want total and system import", d)
	}
}

func TestVerifyEnphaseRaw_ReproducesAndCatchesWrongRecord(t *testing.T) {
	root := t.TempDir()
	rawstore.Dir = filepath.Join(root, "raw")
	t.Cleanup(func() { rawstore.Dir = "" })
	hist := filepath.Join(root, "history")
	if err := os.MkdirAll(hist, 0o755); err != nil {
		t.Fatal(err)
	}
	// A consumption response of 1500 Wh + 500 Wh = 2 kWh for system 7.
	body := `{"system_id":7,"granularity":"day","intervals":[{"end_at":1,"enwh":1500},{"end_at":2,"enwh":500}]}`
	if _, err := rawstore.Save("enphase", "7_telemetry_consumption_meter_2025-11-08", ".json", []byte(body),
		"system=7 endpoint=telemetry/consumption_meter date=2025-11-08 url=x"); err != nil {
		t.Fatal(err)
	}
	write := func(consumption float64) {
		rec := history.DayRecord{Date: "2025-11-08", Systems: []history.SystemRec{{ID: "7", ConsumptionKWh: consumption}}}
		data, _ := json.Marshal(rec)
		if err := os.WriteFile(filepath.Join(hist, "enphase-2025-11-08.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write(2)
	res, err := VerifyEnphaseRaw(hist, io.Discard)
	if err != nil || len(res.Mismatches) != 0 || !res.Manifest.OK() {
		t.Fatalf("matching record: %+v, %v", res, err)
	}
	// The other three endpoints have no raw body here, so the day is incomplete.
	if len(res.NoRaw) != 1 {
		t.Errorf("NoRaw = %v, want the one incomplete day", res.NoRaw)
	}

	write(3) // a record that no longer matches its raw source
	res, _ = VerifyEnphaseRaw(hist, io.Discard)
	if len(res.Mismatches) != 1 {
		t.Errorf("Mismatches = %v, want 1 (record 3 vs raw 2 kWh)", res.Mismatches)
	}
}
