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
// The Enphase Cloud API v4 enforces a per-API-key budget of
// cache.MaxRequestsPerWindow requests per cache.MinRequestInterval (10 / 60s).
// This client relies on internal/cache for caching responses and on a
// sliding-window counter to stay under the limit:
//  1. Past Period serving: Past Period responses are always served from cache
//     without consuming any API Budget — the data is immutable so a live call
//     would return identical results. Detected via cacheMaxAge returning 0.
//     Current Period queries (today / MTD / YTD / Current Period True-Up) are
//     live-first: a real API call is made whenever budget allows, because data
//     changes throughout the day. Cache is the fallback only when budget is
//     exhausted.
//  2. Sliding-window counter: every live response appends a timestamp to
//     cache/api_calls. cache.RemainingBudget() reports how many requests
//     are still available in the current window. When budget is exhausted
//     the client tries an exact-URL cache (any age) then a cross-endpoint
//     match (same endpoint + system, any date) instead of a live call.
//     If no match exists, the request short-circuits to constants.RateLimitError
//     so the user sees the 429 wait message instead of us burning a
//     guaranteed-failed live call.
//  3. 429/503 fallback: when the API responds with a rate-limit or
//     service-unavailable status, cached responses are served as a best-
//     effort fallback regardless of age.
//
// ERROR HANDLING
// --------------
// - 401 Unauthorized: Invalid or expired access token (oauth.go handles refresh)
// - 429 Too Many Requests: Rate limit exceeded (internal/cache handles caching)
// - 500 Server Error: Returned to caller (cache fallback if available)
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
	"strconv"
	"strings"
	"time"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/parser"
	"enphase-monitor/internal/timezone"
	"enphase-monitor/internal/urlbuilder"
)

// EnlightenCloudClient handles communication with Enphase Enlighten Cloud API v4.
// It manages authentication, request formatting, and response parsing for
// a specific system ID.
//
// TESTABILITY: The baseURL field allows dependency injection for testing.
// Production code uses EnphaseAPIv4SystemsURL constant, tests can inject mock server URL.
type EnlightenCloudClient struct {
	baseURL     string // Base URL for API requests (injectable for testing)
	systemID    string
	systemName  string
	apiKey      string
	accessToken string
	timezone    *time.Location // Timezone for reporting/queries
	httpClient  *http.Client
	cacheUsed   bool // Tracks if cache was used for the last request
}

// WithSystemName sets the display name used in warning messages and returns the client.
func (c *EnlightenCloudClient) WithSystemName(name string) *EnlightenCloudClient {
	c.systemName = name
	return c
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

// GetEnergyImportForDate gets the total Grid Import for a specific date/period.
// If testDate is zero, uses today. queryMode selects Day, Month, Year, or True-Up Mode.
// For Month, Year, and True-Up Mode queries, uses the Lifetime Data endpoint (daily aggregated).
// The result always covers complete days only (through yesterday for ongoing periods).
func (c *EnlightenCloudClient) GetEnergyImportForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	if queryMode == constants.QueryModeMonth || queryMode == constants.QueryModeYear || queryMode == constants.QueryModeTrueUp {
		return c.getEnergyImportLifetime(ctx, testDate, queryMode)
	}

	// Single-day query: use Interval Data endpoint (better granularity, 96 data points)
	bodyBytes, err := c.fetchTelemetryData(ctx, "energy_import_telemetry", testDate, queryMode)
	if err != nil {
		return 0, err
	}
	allIntervals, err := parser.ParseNestedTelemetryResponse(bodyBytes)
	if err != nil {
		return 0, err
	}
	importWh := parser.SumIntervalValues(allIntervals, constants.FieldWhImported)
	return importWh / constants.WhToKWh, nil
}

// GetEnergyExportForDate gets the total Grid Export for a specific date/period.
// If testDate is zero, uses today. queryMode selects Day, Month, Year, or True-Up Mode.
// For Month, Year, and True-Up Mode queries, uses the Lifetime Data endpoint (daily aggregated).
// The result always covers complete days only (through yesterday for ongoing periods).
func (c *EnlightenCloudClient) GetEnergyExportForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	if queryMode == constants.QueryModeMonth || queryMode == constants.QueryModeYear || queryMode == constants.QueryModeTrueUp {
		return c.getEnergyExportLifetime(ctx, testDate, queryMode)
	}

	// Single-day query: use Interval Data endpoint
	bodyBytes, err := c.fetchTelemetryData(ctx, "energy_export_telemetry", testDate, queryMode)
	if err != nil {
		return 0, err
	}
	allIntervals, err := parser.ParseNestedTelemetryResponse(bodyBytes)
	if err != nil {
		return 0, err
	}
	exportWh := parser.SumIntervalValues(allIntervals, constants.FieldWhExported)
	return exportWh / constants.WhToKWh, nil
}

// GetProductionForDate gets the total Production for a specific date/period.
// If testDate is zero, uses today. queryMode selects Day, Month, Year, or True-Up Mode.
// For Month, Year, and True-Up Mode queries, uses the Lifetime Data endpoint (daily aggregated).
// The result always covers complete days only (through yesterday for ongoing periods).
// Returns the aggregated sum of all wh_del values from the API response.
func (c *EnlightenCloudClient) GetProductionForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	if queryMode == constants.QueryModeMonth || queryMode == constants.QueryModeYear || queryMode == constants.QueryModeTrueUp {
		return c.getEnergyLifetime(ctx, testDate, queryMode)
	}

	// Single-day query: use Interval Data endpoint
	bodyBytes, err := c.fetchTelemetryData(ctx, "telemetry/production_meter", testDate, queryMode)
	if err != nil {
		return 0, err
	}
	allIntervals, err := parser.ParseNestedTelemetryResponse(bodyBytes)
	if err != nil {
		allIntervals, err = parser.ParseTelemetryResponse(bodyBytes)
		if err != nil {
			return 0, err
		}
	}
	productionWh := parser.SumIntervalValues(allIntervals, constants.FieldWhDel)
	return productionWh / constants.WhToKWh, nil
}

// GetConsumptionForDate gets the total Consumption for a specific date/period.
// If testDate is zero, uses today. queryMode selects Day, Month, Year, or True-Up Mode.
// For Month, Year, and True-Up Mode queries, uses the Lifetime Data endpoint (daily aggregated).
// The result always covers complete days only (through yesterday for ongoing periods).
func (c *EnlightenCloudClient) GetConsumptionForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	if queryMode == constants.QueryModeMonth || queryMode == constants.QueryModeYear || queryMode == constants.QueryModeTrueUp {
		return c.getConsumptionLifetime(ctx, testDate, queryMode)
	}

	// Single-day query: use Interval Data endpoint
	bodyBytes, err := c.fetchTelemetryData(ctx, "telemetry/consumption_meter", testDate, queryMode)
	if err != nil {
		return 0, err
	}
	intervals, err := parser.ParseTelemetryResponse(bodyBytes)
	if err != nil {
		return 0, err
	}
	consumptionWh := parser.SumIntervalValues(intervals, constants.FieldEnwh)
	return consumptionWh / constants.WhToKWh, nil
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
	socPercent := 0
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

// QueryCost returns the number of live API calls GetMetricsFromCloud will make
// for a single system with the given query mode. Callers can compare this value
// against cache.RemainingBudget() before starting a fetch to decide whether a
// fully live run is possible or whether cached data will be needed.
//
// hasBattery should be true when the system is known to have a battery device.
// For QueryModeDay with hasBattery=true this returns 5 (worst-case estimate):
// GetMetricsFromCloud only calls the battery endpoint when testDate is zero
// (today's live report), but QueryCost is used as a conservative preflight
// check before we know the target date.
// For QueryModeMonth / QueryModeYear / QueryModeTrueUp the battery endpoint is
// never called regardless of hasBattery.
func QueryCost(queryMode constants.QueryMode, hasBattery bool) int {
	// Fixed set per system: grid import, grid export, production, consumption.
	const baseCalls = 4
	if queryMode == constants.QueryModeDay && hasBattery {
		return baseCalls + 1
	}
	return baseCalls
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

	// Preflight: warn when the remaining budget is smaller than what this query
	// needs. For Past Periods every endpoint is served from immutable cache so
	// budget consumption is zero — skip the check. For Current Periods the client
	// will still make whatever live calls it can and fall back to cache for the
	// rest, but alerting early helps the caller understand why results may be stale.
	// Day queries always attempt battery (hasBattery=true for the conservative count).
	if cache.DebugMode() && !timezone.IsPastPeriod(testDate, queryMode, c.timezone) {
		needed := QueryCost(queryMode, queryMode == constants.QueryModeDay)
		remaining := cache.RemainingBudget()
		if remaining < needed {
			fmt.Printf("WARNING: %sInsufficient API budget: need %d call(s), %d/%d remaining — results may use cached data\n",
				sysPrefix, needed, remaining, cache.MaxRequestsPerWindow)
		}
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
			fmt.Printf("WARNING: %sFailed to get grid import: %v\n", sysPrefix, err)
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
			fmt.Printf("WARNING: %sFailed to get grid export: %v\n", sysPrefix, err)
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
			fmt.Printf("WARNING: %sFailed to get battery data: %v\n", sysPrefix, err)
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

// maybeShowNoCacheFallbackWarning prints a one-time warning when returning cached
// data despite --no-cache (e.g. on API error or 429). reason is used in the message.
func maybeShowNoCacheFallbackWarning(reason string) {
	if cache.BudgetWarningShown() {
		return
	}
	fmt.Printf("WARNING: %s - returning cached data despite --no-cache flag\n", reason)
	cache.SetBudgetWarningShown(true)
}

// budgetExhausted reports whether the per-API-key API Budget for the
// current MinRequestInterval window has been used up. When true, the client
// prefers cache over a live API call for any URL that has one.
func budgetExhausted() bool {
	return cache.RemainingBudget() <= 0
}

// cacheMaxAge returns the maximum age at which a cached response for a given
// Query Mode and target date is still considered valid:
//   - Past Periods (already-ended Day / Month / Year / True-Up) -> 0
//     ("never expires" — the data won't change)
//   - today's Day Mode query                                    -> 1 hour
//   - Current Period MTD / YTD / True-Up                        -> 24 hours
//
// Note: timezone.IsPastPeriod always returns false for QueryModeTrueUp by
// design (so its Lifetime Data endpoints are refreshed each run). For cache
// purposes we override that here: a True-Up Period whose start + 1 year is
// in the past is treated as past, since its totals are immutable.
func (c *EnlightenCloudClient) cacheMaxAge(targetDate time.Time, queryMode constants.QueryMode) time.Duration {
	isPast := timezone.IsPastPeriod(targetDate, queryMode, c.timezone)
	if !isPast && queryMode == constants.QueryModeTrueUp && !targetDate.IsZero() {
		trueUpEnd := targetDate.AddDate(1, 0, 0).In(c.timezone)
		if trueUpEnd.Before(time.Now().In(c.timezone)) {
			isPast = true
		}
	}
	if isPast {
		return 0
	}
	if queryMode == constants.QueryModeDay {
		return cache.MaxCurrentDayCacheAge
	}
	return cache.MaxCurrentPeriodCacheAge
}

// serveRecentEndpointCache returns the most recent cache entry for the same
// endpoint+systemID as url, filtered by maxAge (0 = no upper bound). Returns
// nil and false when the URL can't be parsed or no eligible entry exists.
//
// Caller is responsible for gating this on budgetExhausted(); the lookup is
// shaped specifically for the budget-depleted fallback path so that a query
// against a new --date can still surface the most recent same-endpoint cache
// the user has on disk.
func serveRecentEndpointCache(url string, maxAge time.Duration) (*http.Response, bool) {
	endpoint, systemID := cache.ExtractEndpointAndSystemID(url)
	if endpoint == "" || systemID == "" {
		return nil, false
	}
	recent, err := cache.FindMostRecentByEndpoint(endpoint, systemID, maxAge)
	if err != nil {
		return nil, false
	}
	return recent.ToHTTPResponse(), true
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
// │  ┌─────────────┐                                                        │
// │  │ Validation  │──YES──► Return cache only (fail if no cache)           │
// │  │    Mode?    │                                                        |
// │  └──────┬──────┘                                                        │
// │         │ NO                                                            │
// │         ▼                                                               │
// │  ┌───────────────┐                                                      │
// │  │Cache Disabled?│──YES──► Make API call (fall back to cache on error)  │
// │  └──────┬────────┘                                                      │
// │         │ NO                                                            │
// │         ▼                                                               │
// │  ┌─────────────────────────────────┐                                    │
// │  │ Past Period with valid cache?   │──YES──► Return immutable cache     │
// │  │ (cacheMaxAge == 0)              │         (no API call, no budget)   │
// │  └──────┬──────────────────────────┘                                    │
// │         │ NO (Current Period)                                           │
// │         ▼                                                               │
// │  ┌────────────────────┐                                                 │
// │  │ Budget exhausted?  │──YES──► Exact-URL cache (any age);              │
// │  │ (>=10 calls in 60s)│         cross-endpoint cache; RateLimitError    │
// │  └──────┬─────────────┘                                                 │
// │         │ NO (budget available)                                         │
// │         ▼                                                               │
// │  ┌─────────────┐                                                        │
// │  │ Make API    │──429──► Serve any-age cache if available               │
// │  │   Request   │──OK───► RecordAPICall, save to cache, return data      │
// │  └─────────────┘                                                        │
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
	// ─────────────────────────────────────────────────────────────────────────
	// SECTION 1: INITIALIZATION
	// Determine if we are querying a Past Period (affects caching strategy).
	// IsPastPeriod is granularity-aware (not a naive "date before today" check) so that
	// Month and Year Mode queries for the Current Period are NOT treated as past — e.g. a
	// month query with testDate=2026-03-01 is the current month, not a Past Period, even
	// though March 1 is before today.
	// ─────────────────────────────────────────────────────────────────────────
	isDateInPast := timezone.IsPastPeriod(targetDate, queryMode, c.timezone)

	// ─────────────────────────────────────────────────────────────────────────
	// SECTION 2: CACHE LOOKUP
	// First, try to load any existing cached response for this URL.
	// We will use this either directly (if valid) or as fallback (if API fails).
	// ─────────────────────────────────────────────────────────────────────────
	cached, cacheErr := cache.LoadCachedResponse(url, c.timezone)
	if cacheErr != nil && isDateInPast && !targetDate.IsZero() {
		if found, ok := c.tryLoadPastDateCache(url, targetDate); ok {
			cached = found
			cacheErr = nil
		}
	}

	// ─────────────────────────────────────────────────────────────────────────
	// SECTION 3: VALIDATION MODE HANDLING
	// In Validation Mode, ONLY use cache - never make live API calls.
	// This allows validating behavior without hitting the real API.
	// ─────────────────────────────────────────────────────────────────────────
	if cache.ValidationMode() {
		if cacheErr == nil {
			resp := cached.ToHTTPResponse()
			return resp, true, nil // Cache was used (Validation Mode)
		}
		// In Validation Mode, provide more detailed error about missing cache
		cachePath := cache.GetCachePath(url, c.timezone)
		normalizedURL := cache.NormalizeURLForCache(url, c.timezone)
		return nil, false, fmt.Errorf("validation mode: no cached response available; cache path: %s, normalized URL: %s", cachePath, cache.RedactURLKey(normalizedURL))
	}

	// ─────────────────────────────────────────────────────────────────────────
	// SECTION 4: LIVE CACHE MODE HANDLING (--no-cache)
	// When cache is disabled, skip cache lookup and always make live API calls.
	// Note: We still fall back to cache on 429 errors as a safety measure.
	// ─────────────────────────────────────────────────────────────────────────
	if cache.CacheDisabled() {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, false, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil && cacheErr != nil {
			return nil, false, fmt.Errorf("API request failed: %w", err)
		}
		if err != nil {
			maybeShowNoCacheFallbackWarning("API error")
			return cached.ToHTTPResponse(), true, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests && cacheErr != nil {
			resp.Body.Close()
			return nil, false, errors.New(constants.RateLimitError)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			maybeShowNoCacheFallbackWarning("Rate limited (429)")
			return cached.ToHTTPResponse(), true, nil
		}
		if resp.StatusCode == http.StatusServiceUnavailable && cacheErr != nil {
			resp.Body.Close()
			return nil, false, fmt.Errorf("API request failed with status 503: Enphase service temporarily unavailable and no cached data available")
		}
		if resp.StatusCode == http.StatusServiceUnavailable {
			resp.Body.Close()
			maybeShowNoCacheFallbackWarning("Service unavailable (503)")
			return cached.ToHTTPResponse(), true, nil
		}
		cache.RecordAPICall()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			resp.Body.Close()
			return nil, false, fmt.Errorf("failed to read response: %w", err)
		}
		resp.Body.Close()
		tempResp := &http.Response{StatusCode: resp.StatusCode, Header: resp.Header}
		// Save to cache (ignore errors - caching is best effort)
		_ = cache.SaveCachedResponseFromBytes(url, tempResp, bodyBytes, c.timezone)
		return &http.Response{
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
			Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		}, false, nil
	}

	// isPast is true for Past Periods whose data is immutable (closed Day, Month, Year, or True-Up).
	// maxAge == 0 is the sentinel for "never expires" returned by cacheMaxAge for Past Periods.
	isPast := c.cacheMaxAge(targetDate, queryMode) == 0

	// Past periods: always serve from cache — the data will never change, so a
	// live call would waste budget and return identical results.
	if cacheErr == nil && isPast {
		cache.Debugf("serving cache (Past Period, age %s): %s", time.Since(cached.CachedAt).Round(time.Second), cache.RedactURLKey(url))
		return cached.ToHTTPResponse(), true, nil
	}

	// Current periods (today / current month / current year / current true-up year):
	// prefer a live API call when budget allows — data changes throughout the day.
	// Cache is the fallback when budget is exhausted, not the default.
	if budgetExhausted() {
		cache.Debugf("budget exhausted (%d/%d), falling back to cache: %s", cache.RemainingBudget(), cache.MaxRequestsPerWindow, cache.RedactURLKey(url))
		if cacheErr == nil {
			cache.Debugf("serving cache (budget exhausted, age %s)", time.Since(cached.CachedAt).Round(time.Second))
			return cached.ToHTTPResponse(), true, nil
		}
		if recent, ok := serveRecentEndpointCache(url, 0); ok {
			cache.Debugf("cross-date cache hit (budget exhausted, any age)")
			return recent, true, nil
		}
		cache.Debugf("budget exhausted, no cache available — returning RateLimitError")
		return nil, false, errors.New(constants.RateLimitError)
	}

	// Make the API request
	cache.Debugf("live API call (budget %d/%d): %s", cache.RemainingBudget(), cache.MaxRequestsPerWindow, cache.RedactURLKey(url))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil && cacheErr != nil {
		return nil, false, fmt.Errorf("API request failed: %w", err)
	}
	if err != nil {
		return cached.ToHTTPResponse(), true, nil // Cache was used (error fallback)
	}

	// Handle rate limit (429)
	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		cache.Debugf("server returned 429: %s", cache.RedactURLKey(url))
		if cacheErr == nil {
			cache.Debugf("429 fallback: serving existing cache (age %s)", time.Since(cached.CachedAt).Round(time.Second))
			return cached.ToHTTPResponse(), true, nil // Cache was used (429 fallback)
		}
		// Try one more time to load cache
		if retryCached, retryErr := cache.LoadCachedResponse(url, c.timezone); retryErr == nil {
			cache.Debugf("429 fallback: serving reloaded cache (age %s)", time.Since(retryCached.CachedAt).Round(time.Second))
			return retryCached.ToHTTPResponse(), true, nil // Cache was used (429 retry)
		}
		// Last resort: find any cached response for the same endpoint+system, regardless of age
		if recent, ok := serveRecentEndpointCache(url, 0); ok {
			cache.Debugf("429 fallback: serving cross-date cache (any age)")
			return recent, true, nil // Cache was used (429 cross-date fallback)
		}
		cache.Debugf("429 fallback: no cache available — returning RateLimitError")
		return nil, false, errors.New(constants.RateLimitError)
	}

	// Handle 503 Service Unavailable - Enphase server temporarily down, use cache if available (even stale)
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		if cacheErr == nil {
			return cached.ToHTTPResponse(), true, nil // Cache was used (503 fallback)
		}
		if retryCached, retryErr := cache.LoadCachedResponse(url, c.timezone); retryErr == nil {
			return retryCached.ToHTTPResponse(), true, nil // Cache was used (503 retry)
		}
		return nil, false, fmt.Errorf("API request failed with status 503: Enphase service temporarily unavailable and no cached data available")
	}
	cache.RecordAPICall()
	cache.Debugf("live API call succeeded (budget now %d/%d)", cache.RemainingBudget(), cache.MaxRequestsPerWindow)

	// Handle other non-OK status codes: serve cache as a best-effort fallback
	// when available, otherwise surface the error. Age is not gated here —
	// when the API itself is failing, any cache we have is better than
	// nothing, and the per-query-mode freshness policy was already applied
	// at the top of this function before we attempted the live call.
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, false, fmt.Errorf("API request failed with status %d: failed to read body: %w", resp.StatusCode, err)
		}
		if cacheErr == nil {
			return cached.ToHTTPResponse(), true, nil
		}
		return nil, false, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// API call succeeded

	// Read the response body before caching
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, false, fmt.Errorf("failed to read response: %w", err)
	}
	resp.Body.Close()

	// Save to cache (ignore errors - caching is best effort)
	tempResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
	}
	_ = cache.SaveCachedResponseFromBytes(url, tempResp, bodyBytes, c.timezone)

	// Return response with readable body
	return &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
	}, false, nil // Cache was NOT used (fresh API call)
}

// getEnergyLifetime fetches production data using the Lifetime Data endpoint (energy_lifetime, daily aggregated).
// This Lifetime Data endpoint has no 7-day limit and returns daily totals instead of 15-minute intervals.
func (c *EnlightenCloudClient) getEnergyLifetime(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	periodStart, _ := timezone.GetBoundaries(testDate, queryMode, c.timezone)
	startDateStr := periodStart.Format(constants.DateFormat)
	// Cap to yesterday for ongoing periods so the total covers only complete days.
	// For Past Periods, the period end is already a complete day so we use it directly.
	endDateStr := c.lifetimeEndDate(testDate, queryMode)

	// Build URL for Lifetime Data endpoint
	reqURL := urlbuilder.BuildLifetimeURL(c.baseURL, c.systemID, "energy_lifetime", c.apiKey, startDateStr)

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

	// Sum daily values within the period
	totalWh := parser.SumDailyIntervals(dailyIntervals, startDateStr, endDateStr)
	return totalWh / constants.WhToKWh, nil
}

// getConsumptionLifetime fetches consumption data using the Lifetime Data endpoint (consumption_lifetime).
func (c *EnlightenCloudClient) getConsumptionLifetime(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	periodStart, _ := timezone.GetBoundaries(testDate, queryMode, c.timezone)
	startDateStr := periodStart.Format(constants.DateFormat)
	endDateStr := c.lifetimeEndDate(testDate, queryMode)

	reqURL := urlbuilder.BuildLifetimeURL(c.baseURL, c.systemID, "consumption_lifetime", c.apiKey, startDateStr)

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

	totalWh := parser.SumDailyIntervals(dailyIntervals, startDateStr, endDateStr)
	return totalWh / constants.WhToKWh, nil
}

// getEnergyImportLifetime fetches grid import data using the Lifetime Data endpoint (energy_import_lifetime).
func (c *EnlightenCloudClient) getEnergyImportLifetime(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	periodStart, _ := timezone.GetBoundaries(testDate, queryMode, c.timezone)
	startDateStr := periodStart.Format(constants.DateFormat)
	endDateStr := c.lifetimeEndDate(testDate, queryMode)

	reqURL := urlbuilder.BuildLifetimeURL(c.baseURL, c.systemID, "energy_import_lifetime", c.apiKey, startDateStr)

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

	totalWh := parser.SumDailyIntervals(dailyIntervals, startDateStr, endDateStr)
	return totalWh / constants.WhToKWh, nil
}

// getEnergyExportLifetime fetches grid export data using the Lifetime Data endpoint (energy_export_lifetime).
func (c *EnlightenCloudClient) getEnergyExportLifetime(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	periodStart, _ := timezone.GetBoundaries(testDate, queryMode, c.timezone)
	startDateStr := periodStart.Format(constants.DateFormat)
	endDateStr := c.lifetimeEndDate(testDate, queryMode)

	reqURL := urlbuilder.BuildLifetimeURL(c.baseURL, c.systemID, "energy_export_lifetime", c.apiKey, startDateStr)

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

	totalWh := parser.SumDailyIntervals(dailyIntervals, startDateStr, endDateStr)
	return totalWh / constants.WhToKWh, nil
}
