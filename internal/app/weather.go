// weather.go enriches Day-Mode reports with the day's weather.
//
// The enrichment is best-effort and strictly supplementary: any failure leaves
// the energy report untouched. It runs only for Day Mode — month, year,
// true-up, and cache-only reports never trigger a weather lookup.
//
// Location is read from the cache only (CachedPrimaryCoordinates); reports never
// make the /systems call that resolves coordinates, because that call would
// compete with — and be starved by — the per-minute telemetry budget on a live
// day. Resolving/caching the location is done once, out of band, by --init. If
// the cache is absent (never initialized or cleared), the report prints a notice
// to run --init and skips weather.
package app

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/geocode"
	"enphase-monitor/internal/weather"
)

// CoordinateProvider returns the systems' coordinates from the location cache,
// without network access. ok is false when --init has not populated the cache.
type CoordinateProvider interface {
	CachedPrimaryCoordinates() (geocode.Coordinates, bool)
}

// WeatherProvider fetches weather for a coordinate: the daily aggregate for a
// given date, and the instantaneous current conditions.
type WeatherProvider interface {
	DailyWeather(ctx context.Context, coords geocode.Coordinates, date time.Time) (weather.DailyWeather, error)
	CurrentWeather(ctx context.Context, coords geocode.Coordinates) (weather.CurrentWeather, error)
}

// initNoticeOnce ensures the "run --init" notice prints at most once per process
// (so continuous mode does not repeat it every tick).
var initNoticeOnce sync.Once

// enrichWithTemperature populates the metrics' weather for Day-Mode reports. It
// is a no-op (and never errors) for any other mode, when providers are absent,
// or when the location cache is unavailable.
func enrichWithTemperature(ctx context.Context, rc RunConfig, metrics *aggregator.AggregatedMetrics) {
	if metrics.QueryMode != constants.QueryModeDay {
		return
	}
	if rc.Location == nil || rc.Weather == nil {
		return
	}

	coords, ok := rc.Location.CachedPrimaryCoordinates()
	if !ok {
		initNoticeOnce.Do(func() {
			fmt.Fprintln(os.Stderr, "Note: weather data unavailable — run `enphase-monitor --init` once to enable it.")
		})
		return
	}

	// A zero QueryDate means "today"; weather needs a concrete date in the
	// report timezone.
	now := time.Now().In(rc.ReportTZ)
	date := metrics.QueryDate
	if date.IsZero() {
		date = now
	}

	w, err := rc.Weather.DailyWeather(ctx, coords, date)
	if err != nil {
		if rc.Debug {
			fmt.Fprintf(os.Stderr, "WARNING: could not fetch weather: %v\n", err)
		}
		return
	}

	dw := &aggregator.DailyWeather{
		TempHigh:        w.TempHigh,
		TempLow:         w.TempLow,
		TempUnit:        w.TempUnit,
		WeatherCode:     w.WeatherCode,
		Condition:       w.Condition,
		CloudCoverPct:   w.CloudCoverPct,
		PrecipitationMM: w.PrecipitationMM,
		SolarRadiation:  w.SolarRadiation,
	}

	// For a report on today, overlay the present conditions so the display
	// matches what the user sees now, rather than the whole-day aggregate (which
	// blends in the forecast for hours not yet elapsed). The daily aggregates
	// above are kept for the report's range/irradiance and the dataset.
	if sameDate(date, now) {
		if cur, err := rc.Weather.CurrentWeather(ctx, coords); err == nil {
			dw.HasCurrent = true
			dw.CurrentTemp = cur.Temp
			dw.CurrentCondition = cur.Condition
			dw.CurrentCloudCoverPct = cur.CloudCoverPct
			dw.CurrentPrecipitationMM = cur.PrecipitationMM
		} else if rc.Debug {
			fmt.Fprintf(os.Stderr, "WARNING: could not fetch current conditions: %v\n", err)
		}
	}

	metrics.Weather = dw
}

// sameDate reports whether a and b fall on the same calendar day.
func sameDate(a, b time.Time) bool {
	return a.Format("2006-01-02") == b.Format("2006-01-02")
}
