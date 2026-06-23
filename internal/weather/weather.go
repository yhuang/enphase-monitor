// Package weather fetches daily weather (temperature range, condition, cloud
// cover, precipitation, and solar radiation) from the free Open-Meteo API, for
// correlating Enphase energy reports with the weather.
//
// ENDPOINT SELECTION
// ------------------
// Open-Meteo splits historical weather across two endpoints:
//   - The forecast API serves today and the recent past (~92 days back).
//   - The archive API (ERA5 reanalysis) serves older dates but lags ~5 days.
//
// DailyWeather picks the right one by the age of the requested date, so a caller
// can ask for any single day without knowing which dataset holds it.
//
// CACHING
// -------
// Past days never change, so their results are cached on disk indefinitely.
// Today's values are still evolving, so they are cached briefly (todayCacheTTL)
// to avoid refetching on every continuous-mode tick. Open-Meteo's free tier is
// generous and unrelated to the Enphase API budget, but caching keeps usage
// polite and reports fast.
//
// COMMERCIAL USE
// --------------
// Open-Meteo's free tier is for non-commercial use; a commercial deployment
// needs their paid plan (or another provider). The base URLs are swappable.
package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"enphase-monitor/internal/geocode"
)

// Default Open-Meteo endpoints. Vars (not consts) so callers/tests can swap them.
var (
	defaultForecastURL = "https://api.open-meteo.com/v1/forecast"
	defaultArchiveURL  = "https://archive-api.open-meteo.com/v1/archive"
)

// dailyVariables are the Open-Meteo daily fields requested in a single call.
// daily weather_code is kept as a fallback for the displayed condition; the
// primary source is the hourly array (see hourlyVariables / mostPrevalentCondition).
const dailyVariables = "weather_code,temperature_2m_max,temperature_2m_min,cloud_cover_mean,precipitation_sum,shortwave_radiation_sum"

// hourlyVariables are the Open-Meteo hourly fields requested alongside the daily
// block (same call). The hourly weather codes drive the most-prevalent condition
// label, which reflects the day better than the daily aggregate's most-severe code.
const hourlyVariables = "weather_code"

const (
	// forecastCutoffDays is the age (in days) at or under which the forecast
	// endpoint is used; older dates use the archive endpoint. Kept comfortably
	// inside the forecast API's ~92-day window and past the archive's ~5-day lag.
	forecastCutoffDays = 60
	// todayCacheTTL bounds how long today's (still-changing) values are reused.
	todayCacheTTL = time.Hour
	// requestTimeout bounds a single Open-Meteo request.
	requestTimeout = 10 * time.Second
	// mjPerKWh converts the daily shortwave_radiation_sum (MJ/m²) to kWh/m².
	mjPerKWh = 3.6
)

// DailyWeather is the weather summary for one calendar day.
type DailyWeather struct {
	Date            time.Time
	TempHigh        float64
	TempLow         float64
	TempUnit        string  // display symbol, e.g. "°F" or "°C"
	WeatherCode     int     // WMO interpretation code representative of Condition
	Condition       string  // most prevalent label across the day's hourly codes (falls back to the daily aggregate when hourly data is unavailable)
	CloudCoverPct   float64 // mean cloud cover, %
	PrecipitationMM float64 // total precipitation, mm
	SolarRadiation  float64 // daily shortwave radiation, kWh/m²
}

// Client fetches daily weather from Open-Meteo. Construct with NewClient;
// exported fields may be overridden afterward (tests inject mock servers, a
// temp cache dir, and a fixed clock).
type Client struct {
	ForecastURL string
	ArchiveURL  string
	Unit        string // Open-Meteo temperature_unit value: "fahrenheit" or "celsius"
	HTTPClient  *http.Client
	CacheDir    string
	now         func() time.Time
}

// NewClient returns a Client with production defaults (Fahrenheit, on-disk cache
// under cacheDir).
func NewClient(cacheDir string) *Client {
	return &Client{
		ForecastURL: defaultForecastURL,
		ArchiveURL:  defaultArchiveURL,
		Unit:        "fahrenheit",
		HTTPClient:  &http.Client{Timeout: requestTimeout},
		CacheDir:    cacheDir,
		now:         time.Now,
	}
}

// unitSymbol maps the Open-Meteo unit value to a display symbol.
func (c *Client) unitSymbol() string {
	if strings.EqualFold(c.Unit, "celsius") {
		return "°C"
	}
	return "°F"
}

// DailyWeather returns the weather for date at coords, serving a fresh cache
// when possible and otherwise fetching from the appropriate Open-Meteo endpoint.
func (c *Client) DailyWeather(ctx context.Context, coords geocode.Coordinates, date time.Time) (DailyWeather, error) {
	dateStr := date.Format("2006-01-02")
	symbol := c.unitSymbol()

	if cached, ok := c.loadCache(coords, dateStr, symbol); ok {
		return cached, nil
	}

	w, err := c.fetch(ctx, coords, date, dateStr, symbol)
	if err != nil {
		return DailyWeather{}, err
	}

	c.saveCache(coords, dateStr, w)
	return w, nil
}

// CurrentWeather is the instantaneous conditions "right now". It exists only
// for the present moment (the forecast endpoint's current= block), so it is
// used for the live current-day report, never for past dates or the dataset.
type CurrentWeather struct {
	Temp            float64
	Unit            string // display symbol, e.g. "°F"
	WeatherCode     int
	Condition       string
	CloudCoverPct   float64
	PrecipitationMM float64
}

// currentVariables are the Open-Meteo current= fields requested.
const currentVariables = "temperature_2m,weather_code,cloud_cover,precipitation"

// CurrentWeather fetches the present conditions at coords from the forecast
// endpoint. It is intentionally not cached — the value is, by definition, "now".
func (c *Client) CurrentWeather(ctx context.Context, coords geocode.Coordinates) (CurrentWeather, error) {
	params := url.Values{}
	params.Set("latitude", strconv.FormatFloat(coords.Latitude, 'f', -1, 64))
	params.Set("longitude", strconv.FormatFloat(coords.Longitude, 'f', -1, 64))
	params.Set("current", currentVariables)
	params.Set("timezone", "auto")
	params.Set("temperature_unit", c.Unit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ForecastURL+"?"+params.Encode(), nil)
	if err != nil {
		return CurrentWeather{}, fmt.Errorf("failed to create current-weather request: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return CurrentWeather{}, fmt.Errorf("failed to fetch current weather: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CurrentWeather{}, fmt.Errorf("current-weather request failed with status %d", resp.StatusCode)
	}

	var parsed struct {
		Current struct {
			Temperature2m *float64 `json:"temperature_2m"`
			WeatherCode   *int     `json:"weather_code"`
			CloudCover    *float64 `json:"cloud_cover"`
			Precipitation *float64 `json:"precipitation"`
		} `json:"current"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return CurrentWeather{}, fmt.Errorf("failed to decode current-weather response: %w", err)
	}
	if parsed.Current.Temperature2m == nil {
		return CurrentWeather{}, errors.New("no current temperature available")
	}

	cw := CurrentWeather{Temp: *parsed.Current.Temperature2m, Unit: c.unitSymbol(), Condition: "Unknown"}
	if parsed.Current.WeatherCode != nil {
		cw.WeatherCode = *parsed.Current.WeatherCode
		cw.Condition = conditionFromCode(*parsed.Current.WeatherCode)
	}
	if parsed.Current.CloudCover != nil {
		cw.CloudCoverPct = *parsed.Current.CloudCover
	}
	if parsed.Current.Precipitation != nil {
		cw.PrecipitationMM = *parsed.Current.Precipitation
	}
	return cw, nil
}

// openMeteoResponse is the subset of the Open-Meteo daily payload we read.
// Pointers distinguish a missing/null value from a real 0.
type openMeteoResponse struct {
	Daily struct {
		Time          []string   `json:"time"`
		Max           []*float64 `json:"temperature_2m_max"`
		Min           []*float64 `json:"temperature_2m_min"`
		WeatherCode   []*int     `json:"weather_code"`
		CloudCover    []*float64 `json:"cloud_cover_mean"`
		Precipitation []*float64 `json:"precipitation_sum"`
		Shortwave     []*float64 `json:"shortwave_radiation_sum"`
	} `json:"daily"`
	Hourly struct {
		WeatherCode []*int `json:"weather_code"`
	} `json:"hourly"`
}

// fetch queries the endpoint appropriate for date's age and parses the result.
// fetch retrieves one day's weather, choosing the forecast or archive endpoint by
// the date's age. That cutoff is approximate and the two datasets do not overlap
// perfectly, so a date near the boundary can be present in one endpoint but
// missing from the other (e.g. dates just inside the forecast window that the
// forecast API no longer serves but the archive does). When the primary endpoint
// yields no data, fetch retries once on the alternate before giving up.
func (c *Client) fetch(ctx context.Context, coords geocode.Coordinates, date time.Time, dateStr, symbol string) (DailyWeather, error) {
	primary := c.endpointFor(date)
	w, err := c.fetchFrom(ctx, primary, coords, date, dateStr, symbol)
	if err == nil {
		return w, nil
	}

	alt := c.ArchiveURL
	if primary == c.ArchiveURL {
		alt = c.ForecastURL
	}
	if alt != "" && alt != primary {
		if w2, altErr := c.fetchFrom(ctx, alt, coords, date, dateStr, symbol); altErr == nil {
			return w2, nil
		}
	}
	return DailyWeather{}, err
}

// fetchFrom fetches one day's weather from a specific Open-Meteo endpoint.
func (c *Client) fetchFrom(ctx context.Context, base string, coords geocode.Coordinates, date time.Time, dateStr, symbol string) (DailyWeather, error) {
	params := url.Values{}
	params.Set("latitude", strconv.FormatFloat(coords.Latitude, 'f', -1, 64))
	params.Set("longitude", strconv.FormatFloat(coords.Longitude, 'f', -1, 64))
	params.Set("start_date", dateStr)
	params.Set("end_date", dateStr)
	params.Set("daily", dailyVariables)
	params.Set("hourly", hourlyVariables)
	params.Set("timezone", "auto")
	params.Set("temperature_unit", c.Unit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+params.Encode(), nil)
	if err != nil {
		return DailyWeather{}, fmt.Errorf("failed to create weather request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return DailyWeather{}, fmt.Errorf("failed to fetch weather for %s: %w", dateStr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return DailyWeather{}, fmt.Errorf("weather request for %s failed with status %d", dateStr, resp.StatusCode)
	}

	var parsed openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return DailyWeather{}, fmt.Errorf("failed to decode weather response for %s: %w", dateStr, err)
	}

	// Temperature is required; the other fields are best-effort enrichment.
	if len(parsed.Daily.Max) == 0 || len(parsed.Daily.Min) == 0 ||
		parsed.Daily.Max[0] == nil || parsed.Daily.Min[0] == nil {
		return DailyWeather{}, fmt.Errorf("no temperature data available for %s", dateStr)
	}

	w := DailyWeather{
		Date:     date,
		TempHigh: *parsed.Daily.Max[0],
		TempLow:  *parsed.Daily.Min[0],
		TempUnit: symbol,
	}
	// Prefer the most prevalent condition across the day's hourly codes (most
	// reflective of the day). Fall back to the daily aggregate code (most severe
	// of the day) only when hourly data is unavailable.
	if label, code, ok := mostPrevalentCondition(parsed.Hourly.WeatherCode); ok {
		w.WeatherCode = code
		w.Condition = label
	} else if code := firstInt(parsed.Daily.WeatherCode); code != nil {
		w.WeatherCode = *code
		w.Condition = conditionFromCode(*code)
	} else {
		w.Condition = "Unknown"
	}
	if v := firstFloat(parsed.Daily.CloudCover); v != nil {
		w.CloudCoverPct = *v
	}
	if v := firstFloat(parsed.Daily.Precipitation); v != nil {
		w.PrecipitationMM = *v
	}
	if v := firstFloat(parsed.Daily.Shortwave); v != nil {
		w.SolarRadiation = *v / mjPerKWh // MJ/m² → kWh/m²
	}
	return w, nil
}

func firstFloat(s []*float64) *float64 {
	if len(s) == 0 {
		return nil
	}
	return s[0]
}

func firstInt(s []*int) *int {
	if len(s) == 0 {
		return nil
	}
	return s[0]
}

// mostPrevalentCondition returns the condition that best reflects the day: the
// label occupying the most hours of the day's hourly weather codes. Because
// several WMO codes share one label (e.g. 61/63/65 are all "Rain"), hours are
// tallied per label rather than per raw code. It also returns a representative
// code for the winning label (the most frequent code within it) so the numeric
// WeatherCode stays consistent with the displayed Condition. Ties are broken by
// earliest first appearance, which makes the result deterministic regardless of
// map iteration order. ok is false when there are no hourly codes (all nil/empty).
func mostPrevalentCondition(hourly []*int) (label string, code int, ok bool) {
	type labelStat struct {
		hours     int
		firstSeen int
		codeCount map[int]int
		codeFirst map[int]int
	}
	stats := make(map[string]*labelStat)
	for i, p := range hourly {
		if p == nil {
			continue
		}
		c := *p
		lab := conditionFromCode(c)
		s := stats[lab]
		if s == nil {
			s = &labelStat{firstSeen: i, codeCount: map[int]int{}, codeFirst: map[int]int{}}
			stats[lab] = s
		}
		s.hours++
		s.codeCount[c]++
		if _, seen := s.codeFirst[c]; !seen {
			s.codeFirst[c] = i
		}
	}
	if len(stats) == 0 {
		return "", 0, false
	}

	// Winning label: most hours, ties broken by earliest first appearance.
	for lab, s := range stats {
		if label == "" || s.hours > stats[label].hours ||
			(s.hours == stats[label].hours && s.firstSeen < stats[label].firstSeen) {
			label = lab
		}
	}

	// Representative code within the label: most frequent, ties by earliest.
	win := stats[label]
	bestCount, bestFirst := -1, 0
	for cc, cnt := range win.codeCount {
		if cnt > bestCount || (cnt == bestCount && win.codeFirst[cc] < bestFirst) {
			code, bestCount, bestFirst = cc, cnt, win.codeFirst[cc]
		}
	}
	return label, code, true
}

// conditionFromCode maps a WMO weather interpretation code to a short label.
func conditionFromCode(code int) string {
	switch code {
	case 0:
		return "Clear"
	case 1:
		return "Mainly clear"
	case 2:
		return "Partly cloudy"
	case 3:
		return "Overcast"
	case 45, 48:
		return "Fog"
	case 51, 53, 55:
		return "Drizzle"
	case 56, 57:
		return "Freezing drizzle"
	case 61, 63, 65:
		return "Rain"
	case 66, 67:
		return "Freezing rain"
	case 71, 73, 75:
		return "Snow"
	case 77:
		return "Snow grains"
	case 80, 81, 82:
		return "Rain showers"
	case 85, 86:
		return "Snow showers"
	case 95:
		return "Thunderstorm"
	case 96, 99:
		return "Thunderstorm w/ hail"
	default:
		return "Unknown"
	}
}

// WMOCodeLegend returns the authoritative WMO weather-interpretation-code table
// for the subset of codes the Open-Meteo API actually emits, keyed by code.
//
// This is deliberately distinct from conditionFromCode: that function is the
// lossy *display* collapse (61/63/65 all become "Rain"), whereas this legend
// preserves the full standard meaning (61 = slight, 63 = moderate, 65 = heavy
// rain). The intensity distinction matters for downstream analysis of the
// History Record's weather_code, so do NOT dedupe the two — they serve
// different purposes. Source: Open-Meteo WMO Weather interpretation codes (WW).
func WMOCodeLegend() map[int]string {
	return map[int]string{
		0:  "Clear sky",
		1:  "Mainly clear",
		2:  "Partly cloudy",
		3:  "Overcast",
		45: "Fog",
		48: "Depositing rime fog",
		51: "Drizzle: light",
		53: "Drizzle: moderate",
		55: "Drizzle: dense",
		56: "Freezing drizzle: light",
		57: "Freezing drizzle: dense",
		61: "Rain: slight",
		63: "Rain: moderate",
		65: "Rain: heavy",
		66: "Freezing rain: light",
		67: "Freezing rain: heavy",
		71: "Snow fall: slight",
		73: "Snow fall: moderate",
		75: "Snow fall: heavy",
		77: "Snow grains",
		80: "Rain showers: slight",
		81: "Rain showers: moderate",
		82: "Rain showers: violent",
		85: "Snow showers: slight",
		86: "Snow showers: heavy",
		95: "Thunderstorm: slight or moderate",
		96: "Thunderstorm with slight hail",
		99: "Thunderstorm with heavy hail",
	}
}

// CodeLegendFileName is the project-root reference decoding the weather_code
// field that appears in weather output (e.g. History Records). It lives at the
// root, not under history/, because it is a general weather reference, not a
// dataset artifact. Written by --init.
const CodeLegendFileName = "weather_codes.json"

// WriteCodeLegend serializes the WMO weather-code legend (WMOCodeLegend) to path
// as indented JSON. The legend is a fixed standard, so this is a plain write,
// overwriting any existing file.
func WriteCodeLegend(path string) error {
	data, err := json.MarshalIndent(WMOCodeLegend(), "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling weather-code legend: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing weather-code legend %q: %w", path, err)
	}
	return nil
}

// endpointFor returns the forecast endpoint for today/recent dates and the
// archive endpoint for older ones.
func (c *Client) endpointFor(date time.Time) string {
	daysAgo := int(c.now().Sub(date).Hours() / 24)
	if daysAgo <= forecastCutoffDays {
		return c.ForecastURL
	}
	return c.ArchiveURL
}

// --- caching ---

type cacheEntry struct {
	FetchedAt       time.Time `json:"fetched_at"`
	Date            string    `json:"date"`
	TempHigh        float64   `json:"temp_high"`
	TempLow         float64   `json:"temp_low"`
	TempUnit        string    `json:"temp_unit"`
	WeatherCode     int       `json:"weather_code"`
	Condition       string    `json:"condition"`
	CloudCoverPct   float64   `json:"cloud_cover_pct"`
	PrecipitationMM float64   `json:"precipitation_mm"`
	SolarRadiation  float64   `json:"solar_radiation"`
}

// cachePath builds a stable per-coordinate, per-date cache filename.
func (c *Client) cachePath(coords geocode.Coordinates, dateStr string) string {
	name := fmt.Sprintf("weather_%s_%s_%s.json",
		strconv.FormatFloat(coords.Latitude, 'f', 4, 64),
		strconv.FormatFloat(coords.Longitude, 'f', 4, 64),
		dateStr)
	return filepath.Join(c.CacheDir, name)
}

// loadCache returns a cached result when present, matching the requested unit,
// and still fresh (always fresh for past days; within TTL for today).
func (c *Client) loadCache(coords geocode.Coordinates, dateStr, symbol string) (DailyWeather, bool) {
	data, err := os.ReadFile(c.cachePath(coords, dateStr))
	if err != nil {
		return DailyWeather{}, false
	}
	var e cacheEntry
	if err := json.Unmarshal(data, &e); err != nil {
		return DailyWeather{}, false
	}
	if e.Date != dateStr || e.TempUnit != symbol {
		return DailyWeather{}, false
	}
	if c.isToday(dateStr) && c.now().Sub(e.FetchedAt) > todayCacheTTL {
		return DailyWeather{}, false
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return DailyWeather{}, false
	}
	return DailyWeather{
		Date:            date,
		TempHigh:        e.TempHigh,
		TempLow:         e.TempLow,
		TempUnit:        e.TempUnit,
		WeatherCode:     e.WeatherCode,
		Condition:       e.Condition,
		CloudCoverPct:   e.CloudCoverPct,
		PrecipitationMM: e.PrecipitationMM,
		SolarRadiation:  e.SolarRadiation,
	}, true
}

// saveCache persists a result. Best effort.
func (c *Client) saveCache(coords geocode.Coordinates, dateStr string, w DailyWeather) {
	if c.CacheDir == "" {
		return
	}
	e := cacheEntry{
		FetchedAt:       c.now(),
		Date:            dateStr,
		TempHigh:        w.TempHigh,
		TempLow:         w.TempLow,
		TempUnit:        w.TempUnit,
		WeatherCode:     w.WeatherCode,
		Condition:       w.Condition,
		CloudCoverPct:   w.CloudCoverPct,
		PrecipitationMM: w.PrecipitationMM,
		SolarRadiation:  w.SolarRadiation,
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return
	}
	// Best-effort: a failed cache write just means the next run re-fetches this
	// day's weather, so it is intentionally not propagated.
	_ = os.WriteFile(c.cachePath(coords, dateStr), append(data, '\n'), 0o644)
}

// isToday reports whether dateStr is the current date on the client's clock.
func (c *Client) isToday(dateStr string) bool {
	return dateStr == c.now().Format("2006-01-02")
}
