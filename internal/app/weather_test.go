package app

import (
	"context"
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/geocode"
	"enphase-monitor/internal/types"
	"enphase-monitor/internal/weather"
)

// failingCoord fails the test if its method is ever called.
type failingCoord struct{ t *testing.T }

func (f failingCoord) CachedPrimaryCoordinates() (geocode.Coordinates, bool) {
	f.t.Error("CachedPrimaryCoordinates should not be called")
	return geocode.Coordinates{}, false
}

// failingTemp fails the test if its methods are ever called.
type failingTemp struct{ t *testing.T }

func (f failingTemp) DailyWeather(context.Context, geocode.Coordinates, time.Time) (weather.DailyWeather, error) {
	f.t.Error("DailyWeather should not be called")
	return weather.DailyWeather{}, nil
}

func (f failingTemp) CurrentWeather(context.Context, geocode.Coordinates) (weather.CurrentWeather, error) {
	f.t.Error("CurrentWeather should not be called")
	return weather.CurrentWeather{}, nil
}

// stubCoord returns a fixed coordinate (cache hit) or reports a miss.
type stubCoord struct {
	coords geocode.Coordinates
	ok     bool
}

func (s stubCoord) CachedPrimaryCoordinates() (geocode.Coordinates, bool) { return s.coords, s.ok }

// stubTemp returns fixed results and records which methods were called.
type stubTemp struct {
	w             weather.DailyWeather
	cur           weather.CurrentWeather
	dailyCalled   bool
	currentCalled bool
}

func (s *stubTemp) DailyWeather(context.Context, geocode.Coordinates, time.Time) (weather.DailyWeather, error) {
	s.dailyCalled = true
	return s.w, nil
}

func (s *stubTemp) CurrentWeather(context.Context, geocode.Coordinates) (weather.CurrentWeather, error) {
	s.currentCalled = true
	return s.cur, nil
}

func minimalConfig() *config.Config {
	return &config.Config{Credentials: []*types.APIConfig{{Name: "key1", Key: "k"}}}
}

func cachedAt(coords geocode.Coordinates) stubCoord {
	return stubCoord{coords: coords, ok: true}
}

var siteCoords = geocode.Coordinates{Latitude: 37.6658, Longitude: -121.8755}

// TestEnrichWithTemperature_SkipsNonDayMode confirms month/year/true-up reports
// never trigger a weather lookup.
func TestEnrichWithTemperature_SkipsNonDayMode(t *testing.T) {
	for _, mode := range []constants.QueryMode{constants.QueryModeMonth, constants.QueryModeYear, constants.QueryModeTrueUp} {
		rc := RunConfig{
			Cfg:      minimalConfig(),
			ReportTZ: time.UTC,
			Location: failingCoord{t},
			Weather:  failingTemp{t},
		}
		metrics := &aggregator.AggregatedMetrics{QueryMode: mode}
		enrichWithTemperature(context.Background(), rc, metrics)
		if metrics.Weather != nil {
			t.Errorf("mode %v: Weather should be nil", mode)
		}
	}
}

// TestEnrichWithTemperature_SkipsWhenProvidersNil confirms a Day-Mode report
// with no providers wired is a safe no-op (cache-only and true-up paths).
func TestEnrichWithTemperature_SkipsWhenProvidersNil(t *testing.T) {
	rc := RunConfig{Cfg: minimalConfig(), ReportTZ: time.UTC}
	metrics := &aggregator.AggregatedMetrics{QueryMode: constants.QueryModeDay}

	enrichWithTemperature(context.Background(), rc, metrics)

	if metrics.Weather != nil {
		t.Error("Weather should be nil when providers are nil")
	}
}

// TestEnrichWithTemperature_SkipsWhenLocationUncached confirms that without an
// initialized location cache, no weather lookup happens and Weather stays nil.
func TestEnrichWithTemperature_SkipsWhenLocationUncached(t *testing.T) {
	wx := &stubTemp{}
	rc := RunConfig{
		Cfg:      minimalConfig(),
		ReportTZ: time.UTC,
		Location: stubCoord{ok: false}, // cache miss (--init not run)
		Weather:  wx,
	}
	metrics := &aggregator.AggregatedMetrics{QueryMode: constants.QueryModeDay}

	enrichWithTemperature(context.Background(), rc, metrics)

	if wx.dailyCalled {
		t.Error("weather should not be fetched when location cache is missing")
	}
	if metrics.Weather != nil {
		t.Error("Weather should be nil when location cache is missing")
	}
}

// TestEnrichWithTemperature_TodayOverlaysCurrent confirms a today report (zero
// QueryDate) fetches the current snapshot and overlays it on the daily range.
func TestEnrichWithTemperature_TodayOverlaysCurrent(t *testing.T) {
	wx := &stubTemp{
		w:   weather.DailyWeather{TempHigh: 77, TempLow: 59, TempUnit: "°F", Condition: "Overcast", SolarRadiation: 6.9},
		cur: weather.CurrentWeather{Temp: 63, Condition: "Clear", CloudCoverPct: 2},
	}
	rc := RunConfig{
		Cfg:      minimalConfig(),
		ReportTZ: time.UTC,
		Location: cachedAt(siteCoords),
		Weather:  wx,
	}
	// Zero QueryDate => today.
	metrics := &aggregator.AggregatedMetrics{QueryMode: constants.QueryModeDay}

	enrichWithTemperature(context.Background(), rc, metrics)

	if !wx.currentCalled {
		t.Fatal("current conditions should be fetched for a today report")
	}
	if metrics.Weather == nil || !metrics.Weather.HasCurrent {
		t.Fatal("Weather.HasCurrent should be true for a today report")
	}
	if metrics.Weather.CurrentTemp != 63 || metrics.Weather.CurrentCondition != "Clear" {
		t.Errorf("current = %v°/%q, want 63/Clear", metrics.Weather.CurrentTemp, metrics.Weather.CurrentCondition)
	}
	// Daily range is still carried for display + dataset.
	if metrics.Weather.TempHigh != 77 || metrics.Weather.TempLow != 59 {
		t.Errorf("range = %v/%v, want 77/59", metrics.Weather.TempHigh, metrics.Weather.TempLow)
	}
}

// TestEnrichWithTemperature_PastDateNoCurrent confirms a past-date report uses
// daily aggregates only and does not fetch current conditions.
func TestEnrichWithTemperature_PastDateNoCurrent(t *testing.T) {
	wx := &stubTemp{
		w: weather.DailyWeather{TempHigh: 60, TempLow: 41, TempUnit: "°F", Condition: "Overcast", SolarRadiation: 3.0},
	}
	rc := RunConfig{
		Cfg:      minimalConfig(),
		ReportTZ: time.UTC,
		Location: cachedAt(siteCoords),
		Weather:  wx,
	}
	pastDate := time.Now().In(time.UTC).AddDate(0, 0, -30)
	metrics := &aggregator.AggregatedMetrics{QueryMode: constants.QueryModeDay, QueryDate: pastDate}

	enrichWithTemperature(context.Background(), rc, metrics)

	if wx.currentCalled {
		t.Error("current conditions should NOT be fetched for a past-date report")
	}
	if metrics.Weather == nil {
		t.Fatal("Weather should be populated")
	}
	if metrics.Weather.HasCurrent {
		t.Error("HasCurrent should be false for a past-date report")
	}
	if metrics.Weather.Condition != "Overcast" {
		t.Errorf("condition = %q, want Overcast (daily)", metrics.Weather.Condition)
	}
}
