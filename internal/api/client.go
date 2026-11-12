// Package api provides the HTTP client and types for the Enphase Enlighten Cloud API v4.
//
// PURPOSE
// -------
// This package implements the HTTP client for the Enphase Enlighten Cloud API v4.
// It handles all communication with Enphase's cloud servers to fetch energy metrics
// for a specific System.
//
// WHY CLOUD API?
// --------------
// The Enlighten Cloud API v4 provides:
//   - Remote access: Works from anywhere with internet (no local network required)
//   - Past Period queries: Query any past date (not just today)
//   - Reliable data: Aggregated and validated by Enphase servers
//   - Standardized format: Consistent JSON responses across all Systems
//
// RAW ARCHIVE
// -----------
// Every successful live response is also saved untouched under raw/enphase/ (via
// internal/rawstore), named <system>_<endpoint>_<date>_<fetch timestamp>.json with a
// manifest line recording system, endpoint, date and the redacted URL. The cache
// holds the same bodies but is a cache (clearable, overwritable); raw/ is the audit
// trail. Cache hits make no request and archive nothing.
// DayKWh converts an archived Day Mode body into kWh with the exact parsers the live
// fetch uses, which is what --verify-enphase-raw re-derives history from. Known
// limit: a Day Mode response covers start_at..start_at+86399 s, so the interval
// ending exactly at local midnight (the day's final 15 minutes) is in no day's response.
//
// AUTHENTICATION
// --------------
// The Cloud API uses OAuth 2.0 with two authentication components:
//  1. API Key: Passed as a query parameter (?key=...) on every request
//  2. Access Token: OAuth Bearer token in Authorization header
//
// The access token is obtained via OAuth refresh token flow (see oauth.go).
// Tokens are cached in memory to avoid unnecessary refresh calls.
//
// DATA FORMAT
// -----------
// The API returns Interval Data in 15-minute intervals. Each interval contains:
//   - end_at: Unix timestamp of interval end
//   - enwh: Energy in watt-hours for that interval
//   - devices_reporting: Number of devices that reported data
//
// To get daily totals, we:
//  1. Request all intervals for the target date (start_at to end_at)
//  2. Filter intervals that fall within the date boundaries (configured timezone)
//  3. Sum the enwh values from all matching intervals
//  4. Convert watt-hours to kilowatt-hours (divide by 1000)
//
// RESPONSE FORMATS
// ----------------
// The API uses two different response structures:
//
//	Format 1 - Nested Arrays (import/export endpoints):
//	{
//	  "intervals": [
//	    [ { "end_at": 123, "wh_imported": 100 }, ... ],  // Array of arrays
//	    [ { "end_at": 124, "wh_imported": 150 }, ... ]
//	  ]
//	}
//
//	Format 2 - Flat Arrays (production/consumption/battery):
//	{
//	  "intervals": [
//	    { "end_at": 123, "enwh": 100 },  // Single array
//	    { "end_at": 124, "enwh": 150 }
//	  ]
//	}
//
// See internal/parser/parser.go for parsing logic that handles both formats.
//
// API ENDPOINTS
// -------------
// This client uses the following Enlighten Cloud API v4 endpoints:
//
// INTERVAL DATA ENDPOINTS (15-minute data, single-day per call):
//
//	Production:
//	GET /api/v4/systems/{system_id}/telemetry/production_meter
//	Returns: Array of intervals with enwh (Production in Wh per interval)
//
//	Consumption:
//	GET /api/v4/systems/{system_id}/telemetry/consumption_meter
//	Returns: Array of intervals with enwh (Consumption in Wh per interval)
//
//	Grid Import:
//	GET /api/v4/systems/{system_id}/energy_import_telemetry
//	Returns: Nested array of intervals with wh_imported (Grid Import in Wh)
//
//	Grid Export:
//	GET /api/v4/systems/{system_id}/energy_export_telemetry
//	Returns: Nested array of intervals with wh_exported (Grid Export in Wh)
//
//	Battery (Charge / Discharge / SOC):
//	GET /api/v4/systems/{system_id}/telemetry/battery
//	Returns: Array of intervals with charge/discharge per interval plus SOC
//
// LIFETIME DATA ENDPOINTS (daily aggregated data; no per-call day limit):
//
//	Production Lifetime:
//	GET /api/v4/systems/{system_id}/energy_lifetime
//	Returns: {"production": [18205, 20777, ...]} - array of daily Wh values
//
//	Consumption Lifetime:
//	GET /api/v4/systems/{system_id}/consumption_lifetime
//	Returns: {"consumption": [18205, 20777, ...]} - array of daily Wh values
//
//	Import Lifetime:
//	GET /api/v4/systems/{system_id}/energy_import_lifetime
//	Returns: {"import": [18205, 20777, ...]} - array of daily Wh values
//
//	Export Lifetime:
//	GET /api/v4/systems/{system_id}/energy_export_lifetime
//	Returns: {"export": [18205, 20777, ...]} - array of daily Wh values
//
// NOTE: Lifetime Data endpoint (battery_lifetime) is NOT used. Battery data is fetched only via
// the Interval Data endpoint (telemetry/battery) and only for today's live Day Mode query.
// Month, Year, and True-Up Mode queries skip battery entirely.
//
// ENDPOINT SELECTION STRATEGY:
//   - Single-day queries: Use Interval Data endpoints (better granularity, 96 data points).
//   - Month, Year, and True-Up Mode queries: use Lifetime Data endpoints (daily aggregated; the per-call limit applies only to Interval Data).
//     NOTE: The Interval Data endpoints only return one calendar day per call regardless of
//     the end_at parameter (API returns granularity=day and ignores wider ranges).
//   - Current Period queries (month-to-date, year-to-date, current True-Up): Data is capped to yesterday (the last
//     complete day). Today's partial data is excluded — the Lifetime Data endpoints only contain
//     completed days. Use lifetimeEndDate() to get the correct end date.
//
// All Interval Data endpoints accept query parameters:
//   - start_at: Unix timestamp (start of date range)
//   - end_at: Unix timestamp (end of date range)
//   - key: API key (required)
//
// All Lifetime Data endpoints accept query parameters:
//   - start_date: Date string in YYYY-MM-DD format
//   - key: API key (required)
//
// RATE LIMITING & API BUDGET
// --------------------------
// The Enphase Cloud API v4 enforces 10 requests/minute and
// constants.MaxRequestsPerMonth per calendar month (1000) per API key.
//  1. Past Period serving: Past Period responses are always served from cache
//     without making a live call — the data is immutable. Detected via
//     isImmutablePeriod. Current Period queries are live-first.
//  2. Monthly tracking: RecordAPICall increments the per-credential monthly
//     counter in credentials.Pool on every successful live call.
//  3. Errors propagate: on a transport error, 429, or 503, the client returns
//     the error. A 429 reaches the aggregator, which fails over to a spare
//     credential; Backfill Mode fails the day so it is retried rather than
//     written from stale data.
//
// ERROR HANDLING
// --------------
// - 401 Unauthorized: Invalid or expired access token (oauth.go handles refresh)
// - 429 Too Many Requests: Rate limit exceeded — returned so the pool fails over
// - 500 Server Error: Returned to caller
// - Network errors: Returned to caller for handling
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/parser"
	"enphase-monitor/internal/rawstore"
	"enphase-monitor/internal/timezone"
	"enphase-monitor/internal/urlbuilder"
)

// BudgetTracker records per-credential monthly API usage. Implemented by
// credentials.Pool in production; tests may inject a stub.
type BudgetTracker interface {
	RecordAPICall(credentialName string)
}

type unlimitedBudget struct{}

func (unlimitedBudget) RecordAPICall(string) {}

// EnlightenCloudClient handles communication with Enphase Enlighten Cloud API v4.
// It manages authentication, request formatting, and response parsing for
// a specific system ID.
//
// TESTABILITY: The baseURL field allows dependency injection for testing.
// Production code uses EnphaseAPIv4SystemsURL constant, tests can inject mock server URL.
type EnlightenCloudClient struct {
	baseURL        string // Base URL for API requests (injectable for testing)
	systemID       string
	systemName     string
	apiKey         string
	accessToken    string
	timezone       *time.Location // Timezone for reporting/queries
	httpClient     *http.Client
	cacheUsed      bool // Tracks if cache was used for the last request
	budget         BudgetTracker
	credentialName string
}

// WithSystemName sets the display name used in warning messages and returns the client.
func (c *EnlightenCloudClient) WithSystemName(name string) *EnlightenCloudClient {
	c.systemName = name
	return c
}

// WithBudget attaches a per-credential budget tracker and returns the client.
func (c *EnlightenCloudClient) WithBudget(tracker BudgetTracker, credentialName string) *EnlightenCloudClient {
	c.budget = tracker
	c.credentialName = credentialName
	return c
}

func (c *EnlightenCloudClient) budgetTracker() BudgetTracker {
	if c.budget != nil {
		return c.budget
	}
	return unlimitedBudget{}
}

// NewEnlightenCloudClient creates a new client for Enlighten Cloud API with API key and OAuth token.
// Uses the production Enphase API URL.
func NewEnlightenCloudClient(systemID, apiKey, accessToken string, timezone *time.Location) *EnlightenCloudClient {
	return &EnlightenCloudClient{
		baseURL:     constants.EnphaseAPIv4SystemsURL,
		systemID:    systemID,
		apiKey:      apiKey,
		accessToken: accessToken,
		timezone:    timezone,
		httpClient: &http.Client{
			Timeout: constants.APIRequestTimeout,
		},
	}
}

// NewEnlightenCloudClientWithBaseURL creates a client with a custom base URL (for testing).
// This constructor enables dependency injection for testing with mock HTTP servers.
func NewEnlightenCloudClientWithBaseURL(baseURL, systemID, apiKey, accessToken string, timezone *time.Location) *EnlightenCloudClient {
	return &EnlightenCloudClient{
		baseURL:     baseURL,
		systemID:    systemID,
		apiKey:      apiKey,
		accessToken: accessToken,
		timezone:    timezone,
		httpClient: &http.Client{
			Timeout: constants.APIRequestTimeout,
		},
	}
}

// lifetimeEndDate returns the end date string to use when filtering lifetime API responses.
// For ongoing periods it returns yesterday so the total covers only complete days —
// Lifetime Data does not contain today's partial data.
// For Past Periods the period's own end date is used (all days are already complete).
func (c *EnlightenCloudClient) lifetimeEndDate(testDate time.Time, queryMode constants.QueryMode) string {
	if timezone.IsPastPeriod(testDate, queryMode, c.timezone) {
		_, periodEnd := timezone.GetBoundaries(testDate, queryMode, c.timezone)
		return periodEnd.Format(constants.DateFormat)
	}
	yesterday := time.Now().In(c.timezone).AddDate(0, 0, -1)
	return yesterday.Format(constants.DateFormat)
}

// fetchTelemetryData is a helper method that reduces redundant code across Get*ForDate methods.
// It handles the common pattern of: make request, track cache usage, read body, close response.
// This helper eliminates ~15 lines of boilerplate per method (5 methods = ~75 lines saved).
// queryMode selects Day, Month, Year, or True-Up Mode.
func (c *EnlightenCloudClient) fetchTelemetryData(ctx context.Context, endpoint string, testDate time.Time, queryMode constants.QueryMode) ([]byte, error) {
	periodStart, periodEnd := timezone.GetBoundaries(testDate, queryMode, c.timezone)
	// Use client's baseURL for dependency injection (testability)
	reqURL := c.buildTelemetryURL(endpoint, periodStart, periodEnd)

	resp, cacheUsed, err := c.makeCachedAPIRequest(ctx, reqURL, testDate, queryMode)
	c.cacheUsed = cacheUsed
	if err != nil {
		return nil, fmt.Errorf("%s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	return parser.ReadResponseBody(resp.Body)
}

// buildTelemetryURL constructs a URL using the client's base URL and parameters.
// This method uses the injected baseURL for testability.
func (c *EnlightenCloudClient) buildTelemetryURL(endpoint string, dayStart, dayEnd time.Time) string {
	return urlbuilder.BuildTelemetryURL(c.baseURL, c.systemID, endpoint, c.apiKey, dayStart, dayEnd)
}

// intervalMetric describes one energy metric: the Interval Data endpoint for Day Mode,
// the Lifetime Data endpoint for Month/Year/True-Up Mode, the interval field to sum, and
// how to parse the Day Mode response shape. Capturing the per-metric differences as data
// lets the four Get*ForDate methods share a single fetch path.
type intervalMetric struct {
	dayEndpoint      string                                           // Interval Data endpoint (Day Mode)
	lifetimeEndpoint string                                           // Lifetime Data endpoint (Month/Year/True-Up Mode)
	field            string                                           // constants.Field* summed from each interval
	parseDay         func([]byte) ([]parser.TelemetryInterval, error) // Day Mode response parser
}

var (
	importMetric = intervalMetric{
		dayEndpoint:      "energy_import_telemetry",
		lifetimeEndpoint: "energy_import_lifetime",
		field:            constants.FieldWhImported,
		parseDay:         parser.ParseNestedTelemetryResponse,
	}
	exportMetric = intervalMetric{
		dayEndpoint:      "energy_export_telemetry",
		lifetimeEndpoint: "energy_export_lifetime",
		field:            constants.FieldWhExported,
		parseDay:         parser.ParseNestedTelemetryResponse,
	}
	productionMetric = intervalMetric{
		dayEndpoint:      "telemetry/production_meter",
		lifetimeEndpoint: "energy_lifetime",
		field:            constants.FieldWhDel,
		parseDay:         parseProductionIntervals,
	}
	consumptionMetric = intervalMetric{
		dayEndpoint:      "telemetry/consumption_meter",
		lifetimeEndpoint: "consumption_lifetime",
		field:            constants.FieldEnwh,
		parseDay:         parser.ParseTelemetryResponse,
	}
)

// parseProductionIntervals parses the production_meter response, which has appeared in
// both the nested and the flat interval shapes; it tries nested first and falls back to
// flat. (Import/export are always nested; consumption is always flat.)
func parseProductionIntervals(body []byte) ([]parser.TelemetryInterval, error) {
	intervals, err := parser.ParseNestedTelemetryResponse(body)
	if err != nil {
		return parser.ParseTelemetryResponse(body)
	}
	return intervals, nil
}

// fetchMetric returns a metric total in kWh for the period. Month, Year, and True-Up Mode
// use the Lifetime Data endpoint (daily aggregated, complete days only); Day Mode uses the
// Interval Data endpoint (better granularity, 96 data points) and the metric's parser.
func (c *EnlightenCloudClient) fetchMetric(ctx context.Context, m intervalMetric, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	if queryMode == constants.QueryModeMonth || queryMode == constants.QueryModeYear || queryMode == constants.QueryModeTrueUp {
		return c.fetchLifetimeMetric(ctx, m.lifetimeEndpoint, testDate, queryMode)
	}

	bodyBytes, err := c.fetchTelemetryData(ctx, m.dayEndpoint, testDate, queryMode)
	if err != nil {
		return 0, err
	}
	intervals, err := m.parseDay(bodyBytes)
	if err != nil {
		return 0, err
	}
	return parser.SumIntervalValues(intervals, m.field) / constants.WhToKWh, nil
}

// fetchLifetimeMetric fetches a daily-aggregated metric from a Lifetime Data endpoint and
// sums the daily values within the period. The total covers complete days only (through
// yesterday for ongoing periods; see lifetimeEndDate).
func (c *EnlightenCloudClient) fetchLifetimeMetric(ctx context.Context, endpoint string, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	periodStart, _ := timezone.GetBoundaries(testDate, queryMode, c.timezone)
	startDateStr := periodStart.Format(constants.DateFormat)
	endDateStr := c.lifetimeEndDate(testDate, queryMode)

	reqURL := urlbuilder.BuildLifetimeURL(c.baseURL, c.systemID, endpoint, c.apiKey, startDateStr)
	resp, cacheUsed, err := c.makeCachedAPIRequest(ctx, reqURL, testDate, queryMode)
	c.cacheUsed = cacheUsed
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	bodyBytes, err := parser.ReadResponseBody(resp.Body)
	if err != nil {
		return 0, err
	}
	dailyIntervals, err := parser.ParseLifetimeResponse(bodyBytes)
	if err != nil {
		return 0, err
	}
	return parser.SumDailyIntervals(dailyIntervals, startDateStr, endDateStr) / constants.WhToKWh, nil
}

// GetEnergyImportForDate gets the total Grid Import for a specific date/period.
// If testDate is zero, uses today. queryMode selects Day, Month, Year, or True-Up Mode.
// For Month, Year, and True-Up Mode queries, uses the Lifetime Data endpoint (daily aggregated).
// The result always covers complete days only (through yesterday for ongoing periods).
func (c *EnlightenCloudClient) GetEnergyImportForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	return c.fetchMetric(ctx, importMetric, testDate, queryMode)
}

// GetEnergyExportForDate gets the total Grid Export for a specific date/period.
// If testDate is zero, uses today. queryMode selects Day, Month, Year, or True-Up Mode.
// For Month, Year, and True-Up Mode queries, uses the Lifetime Data endpoint (daily aggregated).
// The result always covers complete days only (through yesterday for ongoing periods).
func (c *EnlightenCloudClient) GetEnergyExportForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	return c.fetchMetric(ctx, exportMetric, testDate, queryMode)
}

// GetProductionForDate gets the total Production for a specific date/period.
// If testDate is zero, uses today. queryMode selects Day, Month, Year, or True-Up Mode.
// For Month, Year, and True-Up Mode queries, uses the Lifetime Data endpoint (daily aggregated).
// The result always covers complete days only (through yesterday for ongoing periods).
// Returns the aggregated sum of all wh_del values from the API response.
func (c *EnlightenCloudClient) GetProductionForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	return c.fetchMetric(ctx, productionMetric, testDate, queryMode)
}

// GetConsumptionForDate gets the total Consumption for a specific date/period.
// If testDate is zero, uses today. queryMode selects Day, Month, Year, or True-Up Mode.
// For Month, Year, and True-Up Mode queries, uses the Lifetime Data endpoint (daily aggregated).
// The result always covers complete days only (through yesterday for ongoing periods).
func (c *EnlightenCloudClient) GetConsumptionForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	return c.fetchMetric(ctx, consumptionMetric, testDate, queryMode)
}

// GetBatteryDataForDate gets battery charge, discharge, and State of Charge (SOC) for today.
// Only called when testDate is zero (today's report). Returns charged kWh, discharged kWh,
// and SOC percentage from last_reported_aggregate_soc.
func (c *EnlightenCloudClient) GetBatteryDataForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (charged float64, discharged float64, soc int, err error) {
	// Interval Data endpoint for today's 15-minute battery telemetry.
	bodyBytes, err := c.fetchTelemetryData(ctx, "telemetry/battery", testDate, queryMode)
	if err != nil {
		return 0, 0, 0, err
	}

	var data parser.TelemetryResponse
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return 0, 0, 0, fmt.Errorf("parsing battery response JSON: %w", err)
	}

	// Extract SOC from last_reported_aggregate_soc field (format: "97%" as string)
	var socPercent int
	if data.LastReportedAggregateSOC != "" {
		// Parse string like "97%" to integer 97
		socStr := strings.TrimSuffix(data.LastReportedAggregateSOC, constants.BatterySOCPercentSuffix)
		if parsedSOC, err := strconv.Atoi(socStr); err == nil {
			socPercent = parsedSOC
		}
	}

	// Sum intervals that fall within the requested period (in configured timezone)
	// For battery telemetry:
	// - charge.enwh = Battery Charge (Wh) per interval
	// - discharge.enwh = Battery Discharge (Wh) per interval
	// These are incremental values per 15-minute interval, so we sum them.
	// Filter by configured timezone (periodStart to periodEnd)
	periodStart, periodEnd := timezone.GetBoundaries(testDate, queryMode, c.timezone)
	var chargeWh, dischargeWh float64

	for _, interval := range data.Intervals {
		// Check if this interval's end time falls within the requested time range
		// The interval.EndAt is a Unix timestamp - convert to report timezone
		intervalEndTime := time.Unix(interval.EndAt, 0).In(c.timezone)

		// Include interval if its end time is within [periodStart, periodEnd] (inclusive both ends)
		// We use end time because the interval represents energy during the period ending at EndAt
		if (intervalEndTime.Equal(periodStart) || intervalEndTime.After(periodStart)) && (intervalEndTime.Equal(periodEnd) || intervalEndTime.Before(periodEnd)) {
			chargeWh += interval.Charge.Enwh       // Battery Charge
			dischargeWh += interval.Discharge.Enwh // Battery Discharge
		}
	}

	return chargeWh / constants.WhToKWh, dischargeWh / constants.WhToKWh, socPercent, nil // Convert Wh to kWh
}

// GetMetricsFromCloud fetches all metrics from the Cloud API for the specified period.
// If testDate is provided, uses that date instead of today.
// queryMode selects Day, Month, Year, or True-Up Mode.
// Battery data (charged, discharged, SOC) is only fetched for today's live Day Mode
// query (QueryModeDay with testDate.IsZero()); Past Period Day Mode queries and all
// Month, Year, and True-Up Mode queries leave those fields as zero and skip the
// battery API call entirely.
// Returns metrics and a boolean indicating if any cache was used.
func (c *EnlightenCloudClient) GetMetricsFromCloud(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (*LocalMetrics, bool, error) {
	metrics := &LocalMetrics{
		Timestamp: time.Now(),
	}
	cacheUsed := false

	// Helper to handle optional metrics that may fail (grid import/export, battery)
	shouldLogError := func(err error) bool {
		if timezone.IsPastPeriod(testDate, queryMode, c.timezone) {
			return false // Silently use 0 for Past Periods
		}
		// For Current Period, log only non-rate-limit errors
		return !constants.IsRateLimitError(err)
	}

	sysPrefix := ""
	if c.systemName != "" {
		sysPrefix = "[" + c.systemName + "] "
	}

	// Helper to check context cancellation
	checkCancelled := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	}

	// Fetch all metrics - make grid import/export optional (they may fail with 500)
	var err error

	metrics.GridImportToday, err = c.GetEnergyImportForDate(ctx, testDate, queryMode)
	if err != nil {
		if err := checkCancelled(); err != nil {
			return nil, false, err
		}
		// Grid import may fail - continue with 0
		if shouldLogError(err) {
			fmt.Fprintf(os.Stderr, "WARNING: %sFailed to get grid import: %v\n", sysPrefix, err)
		}
		metrics.GridImportToday = 0
	}
	cacheUsed = cacheUsed || c.cacheUsed

	metrics.GridExportToday, err = c.GetEnergyExportForDate(ctx, testDate, queryMode)
	if err != nil {
		if err := checkCancelled(); err != nil {
			return nil, false, err
		}
		// Grid export may fail - continue with 0
		if shouldLogError(err) {
			fmt.Fprintf(os.Stderr, "WARNING: %sFailed to get grid export: %v\n", sysPrefix, err)
		}
		metrics.GridExportToday = 0
	}
	cacheUsed = cacheUsed || c.cacheUsed

	metrics.ProductionToday, err = c.GetProductionForDate(ctx, testDate, queryMode)
	if err != nil {
		if err := checkCancelled(); err != nil {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("failed to get production: %w", err)
	}
	cacheUsed = cacheUsed || c.cacheUsed

	// Battery data is only meaningful for today's report; skip for past dates
	// and all Month, Year, and True-Up Mode queries to save one of the 10 allowed requests/minute.
	if queryMode == constants.QueryModeDay && testDate.IsZero() {
		metrics.BatteryChargedToday, metrics.BatteryDischargedToday, metrics.BatterySOC, err = c.GetBatteryDataForDate(ctx, testDate, queryMode)
		if err != nil {
			if err := checkCancelled(); err != nil {
				return nil, false, err
			}
			fmt.Fprintf(os.Stderr, "WARNING: %sFailed to get battery data: %v\n", sysPrefix, err)
			metrics.BatteryChargedToday = 0
			metrics.BatteryDischargedToday = 0
			metrics.BatterySOC = 0
		}
		cacheUsed = cacheUsed || c.cacheUsed
	}

	// Get consumption from API (more accurate than calculation)
	metrics.ConsumptionToday, err = c.GetConsumptionForDate(ctx, testDate, queryMode)
	if err != nil {
		if err := checkCancelled(); err != nil {
			return nil, false, err
		}
		// Fallback to calculation if API fails
		metrics.ConsumptionToday = metrics.ProductionToday +
			metrics.GridImportToday -
			metrics.GridExportToday -
			metrics.BatteryChargedToday +
			metrics.BatteryDischargedToday
	}
	cacheUsed = cacheUsed || c.cacheUsed

	return metrics, cacheUsed, nil
}

// isImmutablePeriod reports whether the data for targetDate and queryMode can no
// longer change, so a cached response may be served without a live call:
//   - Past Periods (already-ended Day / Month / Year / True-Up) -> true
//   - today's Day Mode query, Current Period MTD / YTD / True-Up -> false (live-first)
//
// Note: timezone.IsPastPeriod always returns false for QueryModeTrueUp by
// design (so its Lifetime Data endpoints are refreshed each run). For cache
// purposes we override that here: a True-Up Period whose start + 1 year is
// in the past is treated as past, since its totals are immutable.
func (c *EnlightenCloudClient) isImmutablePeriod(targetDate time.Time, queryMode constants.QueryMode) bool {
	isPast := timezone.IsPastPeriod(targetDate, queryMode, c.timezone)
	if !isPast && queryMode == constants.QueryModeTrueUp && !targetDate.IsZero() {
		trueUpEnd := targetDate.AddDate(1, 0, 0).In(c.timezone)
		if trueUpEnd.Before(time.Now().In(c.timezone)) {
			isPast = true
		}
	}
	return isPast
}

// tryLoadPastDateCache attempts to load a cached response by matching URL endpoint,
// system ID, and target date when the primary cache lookup failed (e.g. URL normalization differs).
// Returns (cached, true) when a match is found, (nil, false) otherwise.
// Used only for past-date queries to avoid deep nesting in makeCachedAPIRequest.
func (c *EnlightenCloudClient) tryLoadPastDateCache(url string, targetDate time.Time) (*cache.CachedResponse, bool) {
	parsedURL, err := neturl.Parse(url)
	if err != nil {
		return nil, false
	}
	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	var endpoint string
	if len(pathParts) >= 4 && pathParts[len(pathParts)-2] == "telemetry" {
		endpoint = "telemetry/" + pathParts[len(pathParts)-1]
	}
	var systemID string
	if len(pathParts) >= 3 {
		systemID = pathParts[len(pathParts)-3]
	}
	// Guard: if we cannot identify endpoint and systemID the scan would match
	// old-format cache entries that predate those metadata fields. Bail out
	// rather than returning a false positive.
	if endpoint == "" || systemID == "" {
		return nil, false
	}
	targetDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, c.timezone)
	dateStr := targetDay.Format(constants.DateFormat)
	allEntries, err := cache.ListCacheEntries()
	if err != nil {
		return nil, false
	}
	for _, entry := range allEntries {
		if entry.SystemID != systemID || entry.Endpoint != endpoint || entry.Date != dateStr {
			continue
		}
		found, err := cache.LoadCachedResponseByPath(entry.Path)
		if err != nil {
			continue
		}
		return found, true
	}
	return nil, false
}

// makeCachedAPIRequest makes an API request with intelligent caching.
//
// This is a complex function that handles multiple concerns:
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │                    CACHING DECISION FLOWCHART                           │
// │                                                                         │
// │       START                                                             │
// │         │                                                               │
// │         ▼                                                               │
// │  ┌───────────────┐                                                      │
// │  │Cache Disabled?│──YES──► Live call; never reads the cache             │
// │  │ (--no-cache,  │         (429/503/network errors propagate)           │
// │  │  Backfill)    │                                                      │
// │  └──────┬────────┘                                                      │
// │         │ NO                                                            │
// │         ▼                                                               │
// │  ┌─────────────────────────────────┐                                    │
// │  │ Past Period with valid cache?   │──YES──► Return immutable cache     │
// │  │ (isImmutablePeriod)             │         (no API call, no budget)   │
// │  └──────┬──────────────────────────┘                                    │
// │         │ NO (Current Period, or no cache entry)                        │
// │         ▼                                                               │
// │  ┌─────────────┐                                                        │
// │  │ Live call   │──429/503/error──► Propagate (the aggregator fails over │
// │  │             │                   to a spare credential on a 429)      │
// │  │             │──OK─────────────► RecordAPICall, save to cache, archive│
// │  └─────────────┘                   to raw/enphase, return data          │
// └─────────────────────────────────────────────────────────────────────────┘
//
// Parameters:
//   - url: The full API URL to request
//   - targetDate: The date being queried (zero value means today)
//   - queryMode: The Query Mode (Day, Month, Year, or True-Up), used to determine if
//     the period is in the past (e.g., current month is not a Past Period even
//     though its start date is before today)
//
// Returns:
//   - *http.Response: The response (from cache or live)
//   - bool: true if cache was used, false if fresh API call
//   - error: Any error encountered
func (c *EnlightenCloudClient) makeCachedAPIRequest(ctx context.Context, url string, targetDate time.Time, queryMode constants.QueryMode) (*http.Response, bool, error) {
	// Load any existing cached response for this URL. It is served only for an
	// immutable Past Period; a live call's errors are never masked by it.
	// IsPastPeriod is granularity-aware, so a current-month or
	// current-year query is not treated as past even though its start date is
	// before today; that flag only gates the past-date cache rescan below, which
	// recovers a hit when URL normalization differs from the original query.
	isDateInPast := timezone.IsPastPeriod(targetDate, queryMode, c.timezone)
	cached, cacheErr := cache.LoadCachedResponse(url, c.timezone)
	if cacheErr != nil && isDateInPast && !targetDate.IsZero() {
		if found, ok := c.tryLoadPastDateCache(url, targetDate); ok {
			cached, cacheErr = found, nil
		}
	}
	fb := cacheLookup{url: url, cached: cached, cacheErr: cacheErr}

	switch {
	case cache.CacheDisabled():
		return c.requestNoCacheMode(ctx, fb)
	case fb.has() && c.isImmutablePeriod(targetDate, queryMode):
		// Past Period: the data is immutable, so serving cache avoids a live call
		// that would waste budget and return identical results.
		cache.Debugf("serving cache (Past Period, age %s): %s", time.Since(fb.cached.CachedAt).Round(time.Second), cache.RedactURLKey(url))
		return fb.serve()
	default:
		// Current period with budget available: prefer a live call — data changes
		// throughout the day — with cache as the failure fallback.
		return c.requestLiveMode(ctx, fb)
	}
}

// cacheLookup bundles the cache entry loaded for a request (if any) with the
// request URL, so the request helpers need not re-thread them. A loaded entry is
// served only for an immutable Past Period; it never masks a live call's error.
type cacheLookup struct {
	url      string
	cached   *cache.CachedResponse // valid only when cacheErr == nil
	cacheErr error
}

// has reports whether a usable cache entry was loaded for this request.
func (f cacheLookup) has() bool { return f.cacheErr == nil }

// serve returns the loaded cache entry as an HTTP response, flagged as a cache
// hit. Only valid when has() is true.
func (f cacheLookup) serve() (*http.Response, bool, error) {
	return f.cached.ToHTTPResponse(), true, nil
}

// newAPIRequest builds an authenticated GET request for url. A build failure is
// fatal.
func (c *EnlightenCloudClient) newAPIRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// finalizeAndCache reads resp's body, saves it to cache (best effort), and
// returns a fresh response carrying a readable copy of that body. It consumes
// and closes resp.Body. The returned bool is always false: this is the
// fresh-API-call path, so the cache was not used. (SaveCachedResponseFromBytes
// reads only the status code and headers from the temp response, not its body.)
func (c *EnlightenCloudClient) finalizeAndCache(url string, resp *http.Response) (*http.Response, bool, error) {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, false, fmt.Errorf("failed to read response: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		// Untouched copy for later integrity checks; best effort like the cache.
		sys, endpoint, date := rawMeta(url, c.timezone)
		_, _ = rawstore.Save("enphase", rawName(sys, endpoint, date), ".json", bodyBytes,
			fmt.Sprintf("system=%s endpoint=%s date=%s url=%s", sys, endpoint, date, rawstore.RedactURL(url)))
	}

	tempResp := &http.Response{StatusCode: resp.StatusCode, Header: resp.Header}
	// Best-effort: a failed cache write just means a future request re-fetches
	// live; it must not fail the response we already have in hand.
	_ = cache.SaveCachedResponseFromBytes(url, tempResp, bodyBytes, c.timezone)

	return &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
	}, false, nil
}

// rawMeta identifies an Enphase API request for the raw archive: the system id,
// the endpoint path after it (e.g. "telemetry/production_meter") and the calendar
// date queried in tz. Any part it cannot find is empty.
func rawMeta(rawURL string, tz *time.Location) (system, endpoint, date string) {
	path := rawURL
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	for i, p := range parts {
		if p == "systems" && i+2 < len(parts) {
			system, endpoint = parts[i+1], strings.Join(parts[i+2:], "/")
			break
		}
	}
	return system, endpoint, cache.QueriedDateFromURL(rawURL, tz)
}

// rawName is the raw-archive file name for a request: <system>_<endpoint>_<date>,
// with "/" in the endpoint replaced by "_". It matches the names ImportEnphaseCache
// gives imported cache entries.
func rawName(system, endpoint, date string) string {
	name := system + "_" + strings.ReplaceAll(endpoint, "/", "_")
	if date != "" {
		name += "_" + date
	}
	return strings.Trim(name, "_")
}

// DayKWh converts a Day Mode interval response for one of the four day endpoints
// ("energy_import_telemetry", "energy_export_telemetry", "telemetry/production_meter",
// "telemetry/consumption_meter") into kWh using exactly the parsers the live fetch
// uses. ok is false for any other endpoint. It lets offline verification re-derive
// a day's totals from archived raw bodies.
func DayKWh(endpoint string, body []byte) (kwh float64, ok bool, err error) {
	for _, m := range []intervalMetric{importMetric, exportMetric, productionMetric, consumptionMetric} {
		if m.dayEndpoint != endpoint {
			continue
		}
		intervals, err := m.parseDay(body)
		if err != nil {
			return 0, true, err
		}
		return parser.SumIntervalValues(intervals, m.field) / constants.WhToKWh, true, nil
	}
	return 0, false, nil
}

// errServiceUnavailable is returned when Enphase responds 503 and there is no
// cached data to fall back on.
var errServiceUnavailable = errors.New("API request failed with status 503: Enphase service temporarily unavailable and no cached data available")

// requestNoCacheMode handles --no-cache and Backfill Mode: always make a live
// call and never fall back to cached data. Transport errors, 429, and 503 are
// returned as errors so the caller (the aggregator) can fail over to a spare
// credential, or fail explicitly — essential for Backfill Mode, whose records
// must be authoritative live data rather than silently-served stale cache.
// Successful responses are still written to cache for later (non-no-cache) reads.
func (c *EnlightenCloudClient) requestNoCacheMode(ctx context.Context, fb cacheLookup) (*http.Response, bool, error) {
	req, err := c.newAPIRequest(ctx, fb.url)
	if err != nil {
		return nil, false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("API request failed: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		return nil, false, errors.New(constants.RateLimitError)
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		return nil, false, errServiceUnavailable
	}

	c.budgetTracker().RecordAPICall(c.credentialName)
	return c.finalizeAndCache(fb.url, resp)
}

// requestLiveMode handles the normal current-period path: make a live call.
// Errors (transport, 429, 503, other non-OK) are propagated rather than masked
// by serving stale cache — a 429 must reach the aggregator so it can fail over
// to a spare credential. On success the response is cached and returned fresh.
func (c *EnlightenCloudClient) requestLiveMode(ctx context.Context, fb cacheLookup) (*http.Response, bool, error) {
	cache.Debugf("live API call: %s", cache.RedactURLKey(fb.url))
	req, err := c.newAPIRequest(ctx, fb.url)
	if err != nil {
		return nil, false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("API request failed: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		cache.Debugf("server returned 429: %s", cache.RedactURLKey(fb.url))
		return nil, false, errors.New(constants.RateLimitError)
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		return nil, false, errServiceUnavailable
	}
	c.budgetTracker().RecordAPICall(c.credentialName)
	cache.Debugf("live API call succeeded: %s", cache.RedactURLKey(fb.url))

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, false, fmt.Errorf("API request failed with status %d: failed to read body: %w", resp.StatusCode, err)
		}
		return nil, false, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// API call succeeded
	return c.finalizeAndCache(fb.url, resp)
}
