// rawweather.go re-fetches past days' weather purely to archive the raw Open-Meteo
// payloads (the weather cache keeps only parsed fields), and cross-checks each
// fresh value against the history record already written for that day.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/geocode"
	"enphase-monitor/internal/history"
	"enphase-monitor/internal/weather"
)

// weatherPause spaces requests so ~400 back-to-back calls stay well inside
// Open-Meteo's free-tier limits.
const weatherPause = 150 * time.Millisecond

// RawWeatherResult summarises RefetchWeatherRaw.
type RawWeatherResult struct {
	Fetched int      // days fetched and archived
	Failed  []string // dates whose fetch failed
	Differ  []string // dates whose fresh values differ from the history record
}

// HistoryDates lists the YYYY-MM-DD dates of enphase-*.json records in dir,
// restricted to [from, to] when those are non-zero.
func HistoryDates(dir string, from, to time.Time) ([]string, error) {
	paths, err := filepath.Glob(history.Enphase.RecordPath(dir, "*"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range paths {
		d := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), "enphase-"), ".json")
		if _, err := time.Parse(constants.DateFormat, d); err != nil {
			continue
		}
		// Compare calendar dates as strings: from/to carry the report timezone, d does not.
		if (!from.IsZero() && d < from.Format(constants.DateFormat)) || (!to.IsZero() && d > to.Format(constants.DateFormat)) {
			continue
		}
		out = append(out, d)
	}
	sort.Strings(out)
	return out, nil
}

// RefetchWeatherRaw fetches each date live (the client must have BypassCache set,
// and rawstore enabled so each payload is archived) and compares the result with
// the weather stored in that day's history record. Progress goes to out.
func RefetchWeatherRaw(ctx context.Context, w *weather.Client, coords geocode.Coordinates, dates []string, historyDir string, loc *time.Location, out io.Writer) RawWeatherResult {
	var res RawWeatherResult
	for i, d := range dates {
		if ctx.Err() != nil {
			break
		}
		day, err := time.ParseInLocation(constants.DateFormat, d, loc)
		if err != nil {
			res.Failed = append(res.Failed, d)
			continue
		}
		got, err := w.DailyWeather(ctx, coords, day)
		if err != nil {
			fmt.Fprintf(out, "✗ %s: %v\n", d, err)
			res.Failed = append(res.Failed, d)
			continue
		}
		res.Fetched++
		if msg := weatherDiff(historyDir, d, got); msg != "" {
			fmt.Fprintf(out, "≠ %s: %s\n", d, msg)
			res.Differ = append(res.Differ, d)
		}
		if (i+1)%50 == 0 {
			fmt.Fprintf(out, "… %d/%d days\n", i+1, len(dates))
		}
		time.Sleep(weatherPause)
	}
	return res
}

// weatherDiff describes how got differs from the weather in d's history record,
// or "" when they agree (or the record has no weather to compare).
func weatherDiff(dir, date string, got weather.DailyWeather) string {
	data, err := os.ReadFile(history.Enphase.RecordPath(dir, date))
	if err != nil {
		return ""
	}
	var rec struct {
		Weather *struct {
			TempHigh    float64 `json:"temp_high"`
			TempLow     float64 `json:"temp_low"`
			WeatherCode int     `json:"weather_code"`
		} `json:"weather"`
	}
	if json.Unmarshal(data, &rec) != nil || rec.Weather == nil {
		return ""
	}
	var diffs []string
	if math.Abs(rec.Weather.TempHigh-got.TempHigh) > 0.05 {
		diffs = append(diffs, fmt.Sprintf("high %.1f→%.1f", rec.Weather.TempHigh, got.TempHigh))
	}
	if math.Abs(rec.Weather.TempLow-got.TempLow) > 0.05 {
		diffs = append(diffs, fmt.Sprintf("low %.1f→%.1f", rec.Weather.TempLow, got.TempLow))
	}
	if rec.Weather.WeatherCode != got.WeatherCode {
		diffs = append(diffs, fmt.Sprintf("code %d→%d", rec.Weather.WeatherCode, got.WeatherCode))
	}
	return strings.Join(diffs, ", ")
}

// ApplyRawWeatherResult summarises ApplyRawWeather.
type ApplyRawWeatherResult struct {
	Updated   []string // dates whose record weather was rewritten
	Unchanged int      // dates whose record already matched the raw payload
	Failed    []string // dates with no usable raw payload or an unreadable record
}

// ApplyRawWeather rebuilds the weather block of each enphase-<date>.json record
// from the newest archived Open-Meteo payload for that date (rawWeatherDir), using
// the same parser as live fetches. It makes no network calls and rewrites only
// records whose weather actually changes; energy fields are left exactly as read.
// With dryRun nothing is written: Updated lists the dates that would change, which
// is how --verify-weather-raw detects history that has drifted from its raw source.
func ApplyRawWeather(dates []string, historyDir, rawWeatherDir string, loc *time.Location, out io.Writer, dryRun bool) ApplyRawWeatherResult {
	var res ApplyRawWeatherResult
	prov := readWeatherProvenance(historyDir)
	defer func() {
		if dryRun {
			return
		}
		if err := writeWeatherProvenance(historyDir, prov); err != nil {
			fmt.Fprintf(out, "WARNING: could not write %s: %v\n", WeatherProvenanceFile, err)
		}
	}()
	for _, d := range dates {
		fail := func(format string, args ...any) {
			fmt.Fprintf(out, "✗ %s: %s\n", d, fmt.Sprintf(format, args...))
			res.Failed = append(res.Failed, d)
		}
		day, err := time.ParseInLocation(constants.DateFormat, d, loc)
		if err != nil {
			fail("bad date")
			continue
		}
		payloads, _ := filepath.Glob(filepath.Join(rawWeatherDir, "daily_"+d+"_*.json"))
		if len(payloads) == 0 {
			fail("no raw weather payload in %s", rawWeatherDir)
			continue
		}
		sort.Strings(payloads) // names end in a UTC timestamp: last = newest
		payload := payloads[len(payloads)-1]
		body, err := os.ReadFile(payload)
		if err != nil {
			fail("%v", err)
			continue
		}
		w, err := weather.ParseDailyPayload(body, day, "°F")
		if err != nil {
			fail("%v", err)
			continue
		}

		path := history.Enphase.RecordPath(historyDir, d)
		data, err := os.ReadFile(path)
		if err != nil {
			fail("%v", err)
			continue
		}
		var rec history.DayRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			fail("reading record: %v", err)
			continue
		}
		next := &history.WeatherRec{
			TempHigh: w.TempHigh, TempLow: w.TempLow, TempUnit: w.TempUnit,
			WeatherCode: w.WeatherCode, Condition: w.Condition,
			CloudCoverPct: w.CloudCoverPct, HumidityPct: w.HumidityPct,
			HumidityMaxPct: w.HumidityMaxPct, HumidityMinPct: w.HumidityMinPct,
			PrecipitationMM: w.PrecipitationMM, SolarRadiationKWhM2: w.SolarRadiation,
		}
		entry := weatherProvenanceDay{
			RawFile:            "weather/" + filepath.Base(payload),
			SHA256:             sha256Hex(body),
			SolarRadiationZero: next.SolarRadiationKWhM2 == 0,
		}
		if rec.Weather != nil && *rec.Weather == *next {
			res.Unchanged++
			if old, ok := prov.Days[d]; ok { // keep the history of an earlier rebuild
				entry.Status, entry.Previous = old.Status, old.Previous
			} else {
				entry.Status = weatherMatches
			}
			prov.Days[d] = entry
			continue
		}
		if dryRun {
			fmt.Fprintf(out, "≠ %s: history weather differs from the newest raw payload\n", d)
			res.Updated = append(res.Updated, d)
			continue
		}
		entry.Status, entry.Previous = weatherRebuilt, rec.Weather
		rec.Weather = next
		if err := history.WriteRecord(historyDir, history.Enphase, d, rec); err != nil {
			fail("%v", err)
			continue
		}
		prov.Days[d] = entry
		res.Updated = append(res.Updated, d)
	}
	return res
}

// WeatherProvenanceFile is the sidecar, next to the day records, that says where
// each record's weather block came from. Its dot prefix keeps it out of
// enphase-*.json globs, like the dataset index.
const WeatherProvenanceFile = ".weather-provenance.json"

// Weather provenance statuses.
const (
	weatherRebuilt = "rebuilt_from_raw" // the record held different values and was rewritten from the raw payload
	weatherMatches = "matches_raw"      // the record already equalled what the raw payload yields
)

// weatherProvenanceDay describes one day's weather block.
type weatherProvenanceDay struct {
	Status             string              `json:"status"`
	RawFile            string              `json:"raw_file"` // under raw/, e.g. weather/daily_2025-11-08_….json
	SHA256             string              `json:"sha256"`
	SolarRadiationZero bool                `json:"solar_radiation_zero"` // the stored radiation is exactly 0
	Previous           *history.WeatherRec `json:"previous,omitempty"`   // values before the rebuild (status rebuilt_from_raw only)
}

// weatherProvenance is the sidecar document.
type weatherProvenance struct {
	Note string                          `json:"note"`
	Days map[string]weatherProvenanceDay `json:"days"`
}

const weatherProvenanceNote = "Provenance of the weather block in each history/enphase-*.json. " +
	"rebuilt_from_raw: the record held different values and was rewritten from the archived Open-Meteo payload " +
	"(previous = the replaced values); matches_raw: the record already equalled what that payload yields. " +
	"Either way the stored weather equals the newest payload in raw/weather. Maintained by --raw-apply-weather."

func readWeatherProvenance(dir string) weatherProvenance {
	prov := weatherProvenance{Note: weatherProvenanceNote, Days: map[string]weatherProvenanceDay{}}
	if data, err := os.ReadFile(filepath.Join(dir, WeatherProvenanceFile)); err == nil {
		var existing weatherProvenance
		if json.Unmarshal(data, &existing) == nil && existing.Days != nil {
			prov.Days = existing.Days
		}
	}
	return prov
}

func writeWeatherProvenance(dir string, prov weatherProvenance) error {
	data, err := json.MarshalIndent(prov, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, WeatherProvenanceFile), append(data, '\n'), 0o644)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
