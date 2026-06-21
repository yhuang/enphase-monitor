package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"enphase-monitor/internal/geocode"
)

var testCoords = geocode.Coordinates{Latitude: 37.6658, Longitude: -121.8755}

func fixedClock(s string) func() time.Time {
	now, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return func() time.Time { return now }
}

// dailyJSON builds an Open-Meteo daily response body.
func dailyJSON(date string, code int, max, min, cloud, precip, shortwave float64) string {
	return fmt.Sprintf(`{"daily":{"time":["%s"],"weather_code":[%d],"temperature_2m_max":[%g],"temperature_2m_min":[%g],"cloud_cover_mean":[%g],"precipitation_sum":[%g],"shortwave_radiation_sum":[%g]}}`,
		date, code, max, min, cloud, precip, shortwave)
}

// newTestClient wires a client to a forecast and an archive server, each with
// its own hit counter, plus a temp cache dir and a fixed clock.
func newTestClient(t *testing.T, forecastBody, archiveBody string) (*Client, *int, *int) {
	t.Helper()
	fHits, aHits := 0, 0
	forecast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fHits++
		_, _ = w.Write([]byte(forecastBody))
	}))
	archive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		aHits++
		_, _ = w.Write([]byte(archiveBody))
	}))
	t.Cleanup(forecast.Close)
	t.Cleanup(archive.Close)

	c := NewClient(t.TempDir())
	c.ForecastURL = forecast.URL
	c.ArchiveURL = archive.URL
	c.HTTPClient = forecast.Client()
	c.now = fixedClock("2026-06-18T12:00:00Z")
	return c, &fHits, &aHits
}

func day(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestDailyWeather_TodayUsesForecast(t *testing.T) {
	// code 2 = Partly cloudy; shortwave 24.48 MJ/m² → 6.8 kWh/m².
	c, fHits, aHits := newTestClient(t,
		dailyJSON("2026-06-18", 2, 78.4, 54.1, 41, 0, 24.48),
		dailyJSON("2026-06-18", 0, 0, 0, 0, 0, 0))

	w, err := c.DailyWeather(context.Background(), testCoords, day("2026-06-18"))
	if err != nil {
		t.Fatalf("DailyWeather: %v", err)
	}
	if w.TempHigh != 78.4 || w.TempLow != 54.1 {
		t.Errorf("got high=%v low=%v, want 78.4/54.1", w.TempHigh, w.TempLow)
	}
	if w.TempUnit != "°F" {
		t.Errorf("unit = %q, want °F", w.TempUnit)
	}
	if w.Condition != "Partly cloudy" {
		t.Errorf("condition = %q, want Partly cloudy", w.Condition)
	}
	if w.CloudCoverPct != 41 {
		t.Errorf("cloud = %v, want 41", w.CloudCoverPct)
	}
	if w.SolarRadiation != 6.8 {
		t.Errorf("solar = %v kWh/m², want 6.8", w.SolarRadiation)
	}
	if *fHits != 1 || *aHits != 0 {
		t.Errorf("forecast hits=%d archive hits=%d, want 1/0", *fHits, *aHits)
	}
}

func TestDailyWeather_OldDateUsesArchive(t *testing.T) {
	c, fHits, aHits := newTestClient(t,
		dailyJSON("2026-01-15", 0, 0, 0, 0, 0, 0),
		dailyJSON("2026-01-15", 3, 60.2, 41.0, 19, 0, 10.8))

	w, err := c.DailyWeather(context.Background(), testCoords, day("2026-01-15"))
	if err != nil {
		t.Fatalf("DailyWeather: %v", err)
	}
	if w.TempHigh != 60.2 || w.TempLow != 41.0 {
		t.Errorf("got high=%v low=%v, want 60.2/41.0", w.TempHigh, w.TempLow)
	}
	if w.Condition != "Overcast" {
		t.Errorf("condition = %q, want Overcast", w.Condition)
	}
	if w.SolarRadiation != 3 {
		t.Errorf("solar = %v kWh/m², want 3 (10.8 MJ / 3.6)", w.SolarRadiation)
	}
	if *fHits != 0 || *aHits != 1 {
		t.Errorf("forecast hits=%d archive hits=%d, want 0/1", *fHits, *aHits)
	}
}

func TestDailyWeather_BoundaryFallsBackToArchive(t *testing.T) {
	// A date inside the forecast window (50 days before the fixed clock) that the
	// forecast endpoint has no data for (null temps) but the archive does. fetch
	// must try the forecast first, then fall back to the archive — covering the
	// fuzzy forecast/archive boundary that left a gap of unavailable days.
	nullForecast := `{"daily":{"time":["2026-04-29"],"temperature_2m_max":[null],"temperature_2m_min":[null]}}`
	c, fHits, aHits := newTestClient(t, nullForecast, dailyJSON("2026-04-29", 3, 61.0, 44.0, 20, 0, 10.8))

	w, err := c.DailyWeather(context.Background(), testCoords, day("2026-04-29"))
	if err != nil {
		t.Fatalf("DailyWeather: %v", err)
	}
	if w.TempHigh != 61.0 || w.TempLow != 44.0 {
		t.Errorf("got high=%v low=%v, want 61.0/44.0 (from archive fallback)", w.TempHigh, w.TempLow)
	}
	if *fHits != 1 || *aHits != 1 {
		t.Errorf("forecast hits=%d archive hits=%d, want 1/1 (forecast then archive fallback)", *fHits, *aHits)
	}
}

func TestDailyWeather_PastDateCachedPermanently(t *testing.T) {
	c, _, aHits := newTestClient(t, "", dailyJSON("2026-01-15", 3, 60.2, 41.0, 19, 0, 10.8))

	for i := 0; i < 3; i++ {
		if _, err := c.DailyWeather(context.Background(), testCoords, day("2026-01-15")); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if *aHits != 1 {
		t.Errorf("archive hit %d times across 3 calls, want 1 (past date should cache)", *aHits)
	}
}

func TestDailyWeather_TodayCacheExpires(t *testing.T) {
	c, fHits, _ := newTestClient(t, dailyJSON("2026-06-18", 2, 78.4, 54.1, 41, 0, 24.48), "")

	if _, err := c.DailyWeather(context.Background(), testCoords, day("2026-06-18")); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Advance the clock past the today TTL; the next call must refetch.
	c.now = fixedClock("2026-06-18T14:00:00Z")
	if _, err := c.DailyWeather(context.Background(), testCoords, day("2026-06-18")); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if *fHits != 2 {
		t.Errorf("forecast hit %d times, want 2 (today cache should expire after TTL)", *fHits)
	}
}

func TestDailyWeather_NullTemperature(t *testing.T) {
	body := `{"daily":{"time":["2026-06-18"],"temperature_2m_max":[null],"temperature_2m_min":[null]}}`
	c, _, _ := newTestClient(t, body, "")

	if _, err := c.DailyWeather(context.Background(), testCoords, day("2026-06-18")); err == nil {
		t.Fatal("expected error for null temperatures, got nil")
	}
}

func TestDailyWeather_MissingOptionalFields(t *testing.T) {
	// Temperature present but condition/cloud/solar absent → still succeeds,
	// with a graceful "Unknown" condition.
	body := `{"daily":{"time":["2026-06-18"],"temperature_2m_max":[70],"temperature_2m_min":[50]}}`
	c, _, _ := newTestClient(t, body, "")

	w, err := c.DailyWeather(context.Background(), testCoords, day("2026-06-18"))
	if err != nil {
		t.Fatalf("DailyWeather: %v", err)
	}
	if w.Condition != "Unknown" {
		t.Errorf("condition = %q, want Unknown", w.Condition)
	}
	if w.SolarRadiation != 0 {
		t.Errorf("solar = %v, want 0 when absent", w.SolarRadiation)
	}
}

func TestDailyWeather_CelsiusUnitSymbol(t *testing.T) {
	c, _, _ := newTestClient(t, dailyJSON("2026-06-18", 1, 25.0, 12.0, 10, 0, 25.2), "")
	c.Unit = "celsius"

	w, err := c.DailyWeather(context.Background(), testCoords, day("2026-06-18"))
	if err != nil {
		t.Fatalf("DailyWeather: %v", err)
	}
	if w.TempUnit != "°C" {
		t.Errorf("unit = %q, want °C", w.TempUnit)
	}
}

func TestConditionFromCode(t *testing.T) {
	cases := map[int]string{
		0: "Clear", 2: "Partly cloudy", 3: "Overcast", 45: "Fog",
		61: "Rain", 75: "Snow", 95: "Thunderstorm", 199: "Unknown",
	}
	for code, want := range cases {
		if got := conditionFromCode(code); got != want {
			t.Errorf("conditionFromCode(%d) = %q, want %q", code, got, want)
		}
	}
}

// codePtrs builds a []*int from plain ints (no nils) for hourly-code tests.
func codePtrs(vals ...int) []*int {
	out := make([]*int, len(vals))
	for i := range vals {
		v := vals[i]
		out[i] = &v
	}
	return out
}

func TestMostPrevalentCondition(t *testing.T) {
	t.Run("groups by label so split rain beats a single most-common code", func(t *testing.T) {
		// Clear appears twice (code 0); Rain spans 3 different codes once each.
		// Code-level mode would be Clear (2); label-level mode is Rain (3).
		label, code, ok := mostPrevalentCondition(codePtrs(0, 0, 61, 63, 65))
		if !ok || label != "Rain" {
			t.Fatalf("label = %q, ok = %v; want Rain, true", label, ok)
		}
		if code != 61 {
			t.Errorf("representative code = %d, want 61 (earliest in the Rain group)", code)
		}
	})

	t.Run("ignores nil hours", func(t *testing.T) {
		label, code, ok := mostPrevalentCondition([]*int{nil, ptrInt(0), nil, ptrInt(0), ptrInt(3)})
		if !ok || label != "Clear" || code != 0 {
			t.Errorf("got (%q,%d,%v), want (Clear,0,true)", label, code, ok)
		}
	})

	t.Run("ties broken by earliest appearance", func(t *testing.T) {
		// Overcast (first at index 0) and Clear (first at index 2) each have 2 hours.
		label, _, ok := mostPrevalentCondition(codePtrs(3, 3, 0, 0))
		if !ok || label != "Overcast" {
			t.Errorf("label = %q, want Overcast (earlier first appearance)", label)
		}
	})

	t.Run("empty or all-nil returns ok=false", func(t *testing.T) {
		if _, _, ok := mostPrevalentCondition(nil); ok {
			t.Error("ok = true for nil input, want false")
		}
		if _, _, ok := mostPrevalentCondition([]*int{nil, nil}); ok {
			t.Error("ok = true for all-nil input, want false")
		}
	})
}

func ptrInt(v int) *int { return &v }

// TestDailyWeather_ConditionUsesHourlyMode verifies the displayed condition comes
// from the most prevalent hourly code, not the daily aggregate's most-severe code.
func TestDailyWeather_ConditionUsesHourlyMode(t *testing.T) {
	// Daily weather_code is 95 (Thunderstorm, the day's most severe), but 22 of 24
	// hours are Clear (0). The report should say "Clear".
	body := `{"daily":{"time":["2026-06-18"],"weather_code":[95],"temperature_2m_max":[80],"temperature_2m_min":[60],"cloud_cover_mean":[30],"precipitation_sum":[1],"shortwave_radiation_sum":[18]},` +
		`"hourly":{"weather_code":[0,0,0,0,0,0,0,0,0,0,95,0,0,0,0,0,0,0,0,0,0,0,0,95]}}`
	c, _, _ := newTestClient(t, body, "")

	w, err := c.DailyWeather(context.Background(), testCoords, day("2026-06-18"))
	if err != nil {
		t.Fatalf("DailyWeather: %v", err)
	}
	if w.Condition != "Clear" {
		t.Errorf("condition = %q, want Clear (most prevalent hour), not the daily severe code", w.Condition)
	}
	if w.WeatherCode != 0 {
		t.Errorf("WeatherCode = %d, want 0 (consistent with Clear)", w.WeatherCode)
	}
}

func TestCurrentWeather(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"current":{"time":"2026-06-19T08:30","temperature_2m":62.7,"weather_code":0,"cloud_cover":2,"precipitation":0}}`))
	}))
	defer server.Close()

	c := NewClient(t.TempDir())
	c.ForecastURL = server.URL
	c.HTTPClient = server.Client()

	cw, err := c.CurrentWeather(context.Background(), testCoords)
	if err != nil {
		t.Fatalf("CurrentWeather: %v", err)
	}
	if cw.Temp != 62.7 {
		t.Errorf("temp = %v, want 62.7", cw.Temp)
	}
	if cw.Condition != "Clear" {
		t.Errorf("condition = %q, want Clear", cw.Condition)
	}
	if cw.CloudCoverPct != 2 {
		t.Errorf("cloud = %v, want 2", cw.CloudCoverPct)
	}
	if cw.Unit != "°F" {
		t.Errorf("unit = %q, want °F", cw.Unit)
	}
}

func TestCurrentWeather_NullTemp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"current":{"time":"2026-06-19T08:30","temperature_2m":null}}`))
	}))
	defer server.Close()

	c := NewClient(t.TempDir())
	c.ForecastURL = server.URL
	c.HTTPClient = server.Client()

	if _, err := c.CurrentWeather(context.Background(), testCoords); err == nil {
		t.Fatal("expected error for null current temperature, got nil")
	}
}

func TestDailyWeather_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	c := NewClient(t.TempDir())
	// Point both endpoints at the failing server so the alternate-endpoint
	// fallback also fails and the error propagates.
	c.ForecastURL = server.URL
	c.ArchiveURL = server.URL
	c.HTTPClient = server.Client()
	c.now = fixedClock("2026-06-18T12:00:00Z")

	if _, err := c.DailyWeather(context.Background(), testCoords, day("2026-06-18")); err == nil {
		t.Fatal("expected error on 429, got nil")
	}
	// No cache file should be written when the fetch fails.
	if _, err := os.Stat(filepath.Join(c.CacheDir, "weather_37.6658_-121.8755_2026-06-18.json")); !os.IsNotExist(err) {
		t.Errorf("expected no cache file after failed fetch, stat err = %v", err)
	}
}

// TestWMOCodeLegendComplete guards against a lossy/incomplete legend: every WMO
// code Open-Meteo emits must be defined individually (intensities not collapsed).
func TestWMOCodeLegendComplete(t *testing.T) {
	legend := WMOCodeLegend()
	want := []int{0, 1, 2, 3, 45, 48, 51, 53, 55, 56, 57, 61, 63, 65, 66, 67, 71, 73, 75, 77, 80, 81, 82, 85, 86, 95, 96, 99}
	if len(legend) != len(want) {
		t.Errorf("legend has %d codes, want %d", len(legend), len(want))
	}
	for _, code := range want {
		if legend[code] == "" {
			t.Errorf("WMO code %d missing from legend", code)
		}
	}
	// Intensities must be distinct, not collapsed to one label.
	if legend[61] == legend[65] {
		t.Errorf("rain intensities collapsed: 61 and 65 both %q", legend[61])
	}
}

func TestWriteCodeLegend(t *testing.T) {
	path := filepath.Join(t.TempDir(), CodeLegendFileName)
	if err := WriteCodeLegend(path); err != nil {
		t.Fatalf("WriteCodeLegend: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading legend: %v", err)
	}
	// JSON object keys are strings; map[int]string marshals numeric codes as
	// quoted keys, which is what a consumer indexes weather_code against.
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshaling legend: %v", err)
	}
	if got["61"] != "Rain: slight" || got["65"] != "Rain: heavy" {
		t.Errorf("legend mismatch: 61=%q 65=%q", got["61"], got["65"])
	}
}
