// Package api provides the HTTP client and types for the Enphase Enlighten Cloud API v4.
//
// PURPOSE
// -------
// This package implements the HTTP client for the Enphase Enlighten Cloud API v4.
// It handles all communication with Enphase's cloud servers to fetch energy metrics
// for a specific solar system.
//
// WHY CLOUD API?
// --------------
// The Enlighten Cloud API v4 provides:
//   - Remote access: Works from anywhere with internet (no local network required)
//   - Historical data: Query any past date (not just today)
//   - Reliable data: Aggregated and validated by Enphase servers
//   - Standardized format: Consistent JSON responses across all systems
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
// The API returns telemetry data in 15-minute intervals. Each interval contains:
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
// INTERVAL-BASED ENDPOINTS (15-minute data, 7-day limit):
//
//	Production (Solar Generation):
//	GET /api/v4/systems/{system_id}/telemetry/production_meter
//	Returns: Array of intervals with enwh (energy produced)
//
//	Consumption (Home Usage):
//	GET /api/v4/systems/{system_id}/telemetry/consumption_meter
//	Returns: Array of intervals with enwh (energy consumed)
//
//	Grid Import (Energy from Grid):
//	GET /api/v4/systems/{system_id}/energy_import_telemetry
//	Returns: Nested array of intervals with wh_imported
//
//	Grid Export (Energy to Grid):
//	GET /api/v4/systems/{system_id}/energy_export_telemetry
//	Returns: Nested array of intervals with wh_exported
//
//	Battery (Charge/Discharge):
//	GET /api/v4/systems/{system_id}/telemetry/battery
//	Returns: Array of intervals with charge/discharge/soc data
//
// LIFETIME ENDPOINTS (daily aggregated data, no 7-day limit):
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
//	Battery Lifetime:
//	GET /api/v4/systems/{system_id}/battery_lifetime
//	Returns: {"charge": [...], "discharge": [...]} - arrays of daily Wh values
//
// ENDPOINT SELECTION STRATEGY:
//   - Single-day queries: Use interval endpoints (better granularity, 96 data points).
//   - Month/year/true-up queries: Use lifetime endpoints (daily aggregated, no 7-day limit).
//     NOTE: The interval endpoints only return one calendar day per call regardless of
//     the end_at parameter (API returns granularity=day and ignores wider ranges).
//   - Ongoing periods (current month/year/true-up): Data is capped to yesterday (the last
//     complete day). Today's partial data is excluded — the lifetime endpoints only contain
//     completed days. Use lifetimeEndDate() to get the correct end date.
//
// All interval endpoints accept query parameters:
//   - start_at: Unix timestamp (start of date range)
//   - end_at: Unix timestamp (end of date range)
//   - key: API key (required)
//
// All lifetime endpoints accept query parameters:
//   - start_date: Date string in YYYY-MM-DD format
//   - key: API key (required)
//
// RATE LIMITING
// -------------
// The Enphase Cloud API v4 enforces a per-API-key budget of
// cache.MaxRequestsPerWindow requests per cache.MinRequestInterval (10 / 60s).
// This client relies on internal/cache for caching responses and on a
// sliding-window counter to stay under the limit:
//  1. Per-query-type cache expiry (see EnlightenCloudClient.cacheMaxAge):
//       - past day / month / year / past true-up year: never expires
//       - today's day query:                           1 hour
//       - MTD / YTD / current true-up year:            24 hours
//     If a cache entry for the exact URL exists and is within its age bound,
//     it is returned without an API call. Past periods (immutable totals)
//     are served regardless of age.
//  2. Sliding-window counter: every live response appends a timestamp to
//     cache/api_calls. cache.RemainingBudget() reports how many requests
//     are still available in the current window. When budget is exhausted
//     the client tries a cross-endpoint match (same endpoint + system, any
//     date, gated by the same maxAge) instead of a live call. If no match
//     passes, the request short-circuits to constants.RateLimitError so
//     the user sees the 429 wait message instead of us burning a
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
// the lifetime endpoint does not contain today's partial data.
// For past periods the period's own end date is used (all days are already complete).
func (c *EnlightenCloudClient) lifetimeEndDate(testDate time.Time, queryType constants.QueryType) string {
	if timezone.IsPastPeriod(testDate, queryType, c.timezone) {
		_, periodEnd := timezone.GetBoundaries(testDate, queryType, c.timezone)
		return periodEnd.Format(constants.DateFormat)
	}
	yesterday := time.Now().In(c.timezone).AddDate(0, 0, -1)
	return yesterday.Format(constants.DateFormat)
}

// fetchTelemetryData is a helper method that reduces redundant code across Get*ForDate methods.
// It handles the common pattern of: make request, track cache usage, read body, close response.
// This helper eliminates ~15 lines of boilerplate per method (5 methods = ~75 lines saved).
// queryType determines the date boundaries (day/month/year).
func (c *EnlightenCloudClient) fetchTelemetryData(ctx context.Context, endpoint string, testDate time.Time, queryType constants.QueryType) ([]byte, error) {
	periodStart, periodEnd := timezone.GetBoundaries(testDate, queryType, c.timezone)
	// Use client's baseURL for dependency injection (testability)
	reqURL := c.buildTelemetryURL(endpoint, periodStart, periodEnd)

	resp, cacheUsed, err := c.makeCachedAPIRequest(ctx, reqURL, testDate, queryType)
	c.cacheUsed = cacheUsed
	if err != nil {
		return nil, fmt.Errorf("%s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	return parser.ReadResponseBody(resp.Body)
}

// buildTelemetryURL constructs a URL using the client's base URL and parameters
// This method uses the injected baseURL for testability
func (c *EnlightenCloudClient) buildTelemetryURL(endpoint string, dayStart, dayEnd time.Time) string {
	baseURL := fmt.Sprintf("%s/%s/%s", c.baseURL, c.systemID, endpoint)
	return baseURL + "?key=" + c.apiKey +
		"&start_at=" + strconv.FormatInt(dayStart.Unix(), 10) +
		"&end_at=" + strconv.FormatInt(dayEnd.Unix(), 10)
}

// LocalMetrics is exported in types.go
//
// GetEnergyImportForDate gets the total energy imported from the grid for a specific date/period.
// If testDate is zero, uses today. queryType determines the time range (day/month/year/true-up).
// For month/year/true-up queries, uses the _lifetime endpoint (daily aggregated, no 7-day API limit).
// The result always covers complete days only (through yesterday for ongoing periods).
func (c *EnlightenCloudClient) GetEnergyImportForDate(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	if queryType == constants.QueryTypeMonth || queryType == constants.QueryTypeYear || queryType == constants.QueryTypeTrueUp {
		return c.getEnergyImportLifetime(ctx, testDate, queryType)
	}

	// Single-day query: use interval endpoint (better granularity, 96 data points)
	bodyBytes, err := c.fetchTelemetryData(ctx, "energy_import_telemetry", testDate, queryType)
	if err != nil {
		return 0, err
	}
	allIntervals, err := parser.ParseNestedTelemetryResponse(bodyBytes)
	if err != nil {
		return 0, err
	}
	if len(allIntervals) == 0 {
		return 0, nil
	}
	importWh := parser.SumIntervalValues(allIntervals, constants.FieldWhImported)
	return importWh / constants.WhToKWh, nil
}

// GetEnergyExportForDate gets the total energy exported to the grid for a specific date/period.
// If testDate is zero, uses today. queryType determines the time range (day/month/year/true-up).
// For month/year/true-up queries, uses the _lifetime endpoint (daily aggregated, no 7-day API limit).
// The result always covers complete days only (through yesterday for ongoing periods).
func (c *EnlightenCloudClient) GetEnergyExportForDate(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	if queryType == constants.QueryTypeMonth || queryType == constants.QueryTypeYear || queryType == constants.QueryTypeTrueUp {
		return c.getEnergyExportLifetime(ctx, testDate, queryType)
	}

	// Single-day query: use interval endpoint
	bodyBytes, err := c.fetchTelemetryData(ctx, "energy_export_telemetry", testDate, queryType)
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

// GetProductionForDate gets the total energy production for a specific date/period.
// If testDate is zero, uses today. queryType determines the time range (day/month/year/true-up).
// For month/year/true-up queries, uses the _lifetime endpoint (daily aggregated, no 7-day API limit).
// The result always covers complete days only (through yesterday for ongoing periods).
// Returns the aggregated sum of all wh_del values from the API response.
func (c *EnlightenCloudClient) GetProductionForDate(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	if queryType == constants.QueryTypeMonth || queryType == constants.QueryTypeYear || queryType == constants.QueryTypeTrueUp {
		return c.getEnergyLifetime(ctx, testDate, queryType)
	}

	// Single-day query: use interval endpoint
	bodyBytes, err := c.fetchTelemetryData(ctx, "telemetry/production_meter", testDate, queryType)
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

// GetConsumptionForDate gets the total energy consumption for a specific date/period.
// If testDate is zero, uses today. queryType determines the time range (day/month/year/true-up).
// For month/year/true-up queries, uses the _lifetime endpoint (daily aggregated, no 7-day API limit).
// The result always covers complete days only (through yesterday for ongoing periods).
func (c *EnlightenCloudClient) GetConsumptionForDate(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	if queryType == constants.QueryTypeMonth || queryType == constants.QueryTypeYear || queryType == constants.QueryTypeTrueUp {
		return c.getConsumptionLifetime(ctx, testDate, queryType)
	}

	// Single-day query: use interval endpoint
	bodyBytes, err := c.fetchTelemetryData(ctx, "telemetry/consumption_meter", testDate, queryType)
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

// GetBatteryDataForDate gets battery charge, discharge, and State of Charge (SOC) for a specific date/period.
// If testDate is zero, uses today. queryType determines the time range (day/month/year/true-up).
// Returns charged kWh, discharged kWh, and SOC percentage from last_reported_aggregate_soc.
// For month/year/true-up queries, uses battery_lifetime endpoint (daily aggregated, no 7-day API limit).
// The result always covers complete days only (through yesterday for ongoing periods).
func (c *EnlightenCloudClient) GetBatteryDataForDate(ctx context.Context, testDate time.Time, queryType constants.QueryType) (charged float64, discharged float64, soc int, err error) {
	if queryType == constants.QueryTypeMonth || queryType == constants.QueryTypeYear || queryType == constants.QueryTypeTrueUp {
		return c.getBatteryLifetime(ctx, testDate, queryType)
	}

	// Single-day query: use interval endpoint.
	bodyBytes, err := c.fetchTelemetryData(ctx, "telemetry/battery", testDate, queryType)
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
	// - charge.enwh = energy charged to battery (Wh) per interval
	// - discharge.enwh = energy discharged from battery (Wh) per interval
	// These are incremental values per 15-minute interval, so we sum them
	// Filter by configured timezone (periodStart to periodEnd)
	periodStart, periodEnd := timezone.GetBoundaries(testDate, queryType, c.timezone)
	var chargeWh, dischargeWh float64

	for _, interval := range data.Intervals {
		// Check if this interval's end time falls within the requested time range
		// The interval.EndAt is a Unix timestamp - convert to report timezone
		intervalEndTime := time.Unix(interval.EndAt, 0).In(c.timezone)

		// Include interval if its end time is within [periodStart, periodEnd] (inclusive both ends)
		// We use end time because the interval represents energy during the period ending at EndAt
		if (intervalEndTime.Equal(periodStart) || intervalEndTime.After(periodStart)) && (intervalEndTime.Equal(periodEnd) || intervalEndTime.Before(periodEnd)) {
			chargeWh += interval.Charge.Enwh       // Energy charged to battery
			dischargeWh += interval.Discharge.Enwh // Energy discharged from battery
		}
	}

	return chargeWh / constants.WhToKWh, dischargeWh / constants.WhToKWh, socPercent, nil // Convert Wh to kWh
}

// getBatteryLifetime fetches battery data using the battery_lifetime endpoint (daily aggregated).
// This endpoint has no 7-day limit and returns daily charge/discharge totals.
func (c *EnlightenCloudClient) getBatteryLifetime(ctx context.Context, testDate time.Time, queryType constants.QueryType) (charged float64, discharged float64, soc int, err error) {
	periodStart, _ := timezone.GetBoundaries(testDate, queryType, c.timezone)
	startDateStr := periodStart.Format(constants.DateFormat)

	// Build URL for lifetime endpoint
	reqURL := fmt.Sprintf("%s/%s/battery_lifetime?key=%s&start_date=%s", c.baseURL, c.systemID, c.apiKey, startDateStr)

	resp, cacheUsed, err := c.makeCachedAPIRequest(ctx, reqURL, testDate, queryType)
	c.cacheUsed = cacheUsed
	if err != nil {
		// Battery data is optional - return zeros if it fails
		return 0, 0, 0, nil
	}
	defer resp.Body.Close()

	bodyBytes, err := parser.ReadResponseBody(resp.Body)
	if err != nil {
		return 0, 0, 0, nil
	}

	// Parse the lifetime response
	// Battery lifetime returns arrays like: {"charge": [...], "discharge": [...]}
	var data struct {
		SystemID  int64     `json:"system_id"`
		StartDate string    `json:"start_date"`
		Charge    []float64 `json:"charge"`
		Discharge []float64 `json:"discharge"`
	}

	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return 0, 0, 0, nil
	}

	// Calculate dates and sum values in range
	startDate, err := time.Parse(constants.DateFormat, data.StartDate)
	if err != nil {
		return 0, 0, 0, nil
	}

	endDateStr := c.lifetimeEndDate(testDate, queryType)
	var totalCharge, totalDischarge float64

	for i := range data.Charge {
		date := startDate.AddDate(0, 0, i)
		dateStr := date.Format(constants.DateFormat)

		// Filter by date range (inclusive)
		if dateStr >= startDateStr && dateStr <= endDateStr {
			totalCharge += data.Charge[i]
			if i < len(data.Discharge) {
				totalDischarge += data.Discharge[i]
			}
		}
	}

	return totalCharge / constants.WhToKWh, totalDischarge / constants.WhToKWh, 0, nil
}

// GetMetricsFromCloud fetches all metrics from the Cloud API for the specified period.
// If testDate is provided, uses that date instead of today.
// queryType determines the time range (day/month/year/true-up).
// Battery data (charged, discharged, SOC) is only fetched for QueryTypeDay; all other
// query types leave those fields as zero and skip the battery API call entirely.
// Returns metrics and a boolean indicating if any cache was used.
func (c *EnlightenCloudClient) GetMetricsFromCloud(ctx context.Context, testDate time.Time, queryType constants.QueryType) (*LocalMetrics, bool, error) {
	metrics := &LocalMetrics{
		Timestamp: time.Now(),
	}
	cacheUsed := false

	// Helper to handle optional metrics that may fail (grid import/export, battery)
	shouldLogError := func(err error) bool {
		if timezone.IsPastPeriod(testDate, queryType, c.timezone) {
			return false // Silently use 0 for past periods
		}
		// For current period, log only non-rate-limit errors
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

	metrics.GridImportToday, err = c.GetEnergyImportForDate(ctx, testDate, queryType)
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

	metrics.GridExportToday, err = c.GetEnergyExportForDate(ctx, testDate, queryType)
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

	metrics.ProductionToday, err = c.GetProductionForDate(ctx, testDate, queryType)
	if err != nil {
		if err := checkCancelled(); err != nil {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("failed to get production: %w", err)
	}
	cacheUsed = cacheUsed || c.cacheUsed

	// Battery data is only meaningful for single-day reports; skip the API call
	// for month/year/true-up queries to save one of the 10 allowed requests/minute.
	if queryType == constants.QueryTypeDay {
		metrics.BatteryChargedToday, metrics.BatteryDischargedToday, metrics.BatterySOC, err = c.GetBatteryDataForDate(ctx, testDate, queryType)
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
	metrics.ConsumptionToday, err = c.GetConsumptionForDate(ctx, testDate, queryType)
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
	if cache.RateLimitWarningShown() {
		return
	}
	fmt.Printf("WARNING: %s - returning cached data despite --no-cache flag\n", reason)
	cache.SetRateLimitWarningShown(true)
}

// budgetExhausted reports whether the per-API-key rate-limit budget for the
// current MinRequestInterval window has been used up. When true, the client
// prefers cache over a live API call for any URL that has one.
func budgetExhausted() bool {
	return cache.RemainingBudget() <= 0
}

// cacheMaxAge returns the maximum age at which a cached response for a given
// query type and target date is still considered valid:
//   - past periods (already-ended day / month / year / true-up year) -> 0
//     ("never expires" — the data won't change)
//   - today's day query                                              -> 1 hour
//   - current MTD / YTD / current true-up year                       -> 24 hours
//
// Note: timezone.IsPastPeriod always returns false for QueryTypeTrueUp by
// design (so its lifetime endpoints are refreshed each run). For cache
// purposes we override that here: a true-up year whose start + 1 year is
// in the past is treated as past, since its totals are immutable.
func (c *EnlightenCloudClient) cacheMaxAge(targetDate time.Time, queryType constants.QueryType) time.Duration {
	isPast := timezone.IsPastPeriod(targetDate, queryType, c.timezone)
	if !isPast && queryType == constants.QueryTypeTrueUp && !targetDate.IsZero() {
		trueUpEnd := targetDate.AddDate(1, 0, 0).In(c.timezone)
		if trueUpEnd.Before(time.Now().In(c.timezone)) {
			isPast = true
		}
	}
	if isPast {
		return 0
	}
	if queryType == constants.QueryTypeDay {
		return cache.MaxCurrentDayCacheAge
	}
	return cache.MaxCurrentPeriodCacheAge
}

// cacheServable reports whether the cached response is within the acceptable
// age window for this query type. maxAge of 0 means "no upper bound".
func cacheServable(cached *cache.CachedResponse, maxAge time.Duration) bool {
	if cached == nil {
		return false
	}
	if maxAge == 0 {
		return true
	}
	return time.Since(cached.CachedAt) <= maxAge
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
	targetDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, c.timezone)
	dateStr := targetDay.Format("2006-01-02")
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
// │  │ Test Mode?  │──YES──► Return cache only (fail if no cache)           │
// │  └──────┬──────┘                                                        │
// │         │ NO                                                            │
// │         ▼                                                               │
// │  ┌───────────────┐                                                      │
// │  │Cache Disabled?│──YES──► Make API call (fall back to cache on error)  │
// │  └──────┬────────┘                                                      │
// │         │ NO                                                            │
// │         ▼                                                               │
// │  ┌──────────────────────┐                                               │
// │  │ Cache within maxAge? │──YES──► Return cache (no API call)            │
// │  │ Past period:    ∞    │                                                │
// │  │ Today's day:    1h   │                                                │
// │  │ MTD/YTD/cur TU: 24h  │                                                │
// │  └──────┬───────────────┘                                                │
// │         │ NO (missing or expired)                                        │
// │         ▼                                                                │
// │  ┌────────────────────┐                                                  │
// │  │ Budget exhausted?  │──YES──► Cross-endpoint match (same maxAge);     │
// │  │ (>=10 calls in 60s)│         else short-circuit to RateLimitError    │
// │  └──────┬─────────────┘                                                  │
// │         │ NO (budget available)                                          │
// │         ▼                                                                │
// │  ┌─────────────┐                                                         │
// │  │ Make API    │──429──► Serve any-age cache if available                │
// │  │   Request   │──OK───► RecordAPICall, save to cache, return data      │
// │  └─────────────┘                                                         │
// └─────────────────────────────────────────────────────────────────────────┘
//
// Parameters:
//   - url: The full API URL to request
//   - targetDate: The date being queried (zero value means today)
//   - queryType: The query granularity (day/month/year), used to determine if
//     the period is in the past (e.g., current month is not a past period even
//     though its start date is before today)
//
// Returns:
//   - *http.Response: The response (from cache or live)
//   - bool: true if cache was used, false if fresh API call
//   - error: Any error encountered
func (c *EnlightenCloudClient) makeCachedAPIRequest(ctx context.Context, url string, targetDate time.Time, queryType constants.QueryType) (*http.Response, bool, error) {
	// ─────────────────────────────────────────────────────────────────────────
	// SECTION 1: INITIALIZATION
	// Determine if we are querying a past period (affects caching strategy).
	// Use IsPastPeriod (not IsPastDate) so that month/year queries for the
	// current period are NOT treated as past — e.g. a month query with
	// testDate=2026-03-01 is the current month, not a past period, even
	// though March 1 is before today.
	// ─────────────────────────────────────────────────────────────────────────
	isDateInPast := timezone.IsPastPeriod(targetDate, queryType, c.timezone)

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
	// SECTION 4: TEST MODE HANDLING
	// In test mode, ONLY use cache - never make live API calls.
	// This allows validating behavior without hitting the real API.
	// ─────────────────────────────────────────────────────────────────────────
	if cache.TestMode() {
		if cacheErr == nil {
			resp := cached.ToHTTPResponse()
			return resp, true, nil // Cache was used (test mode)
		}
		// In test mode, provide more detailed error about missing cache
		cachePath := cache.GetCachePath(url, c.timezone)
		normalizedURL := cache.NormalizeURLForCache(url, c.timezone)
		return nil, false, fmt.Errorf("test mode: no cached response available; cache path: %s, normalized URL: %s", cachePath, cache.RedactURLKey(normalizedURL))
	}

	// ─────────────────────────────────────────────────────────────────────────
	// SECTION 5: NO-CACHE MODE HANDLING
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

		if resp.StatusCode == 429 && cacheErr != nil {
			resp.Body.Close()
			return nil, false, fmt.Errorf(constants.RateLimitError)
		}
		if resp.StatusCode == 429 {
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

	// isPast is true for periods whose data is immutable (ended day/month/year/true-up).
	// maxAge == 0 is the sentinel for "never expires" returned by cacheMaxAge for past periods.
	isPast := c.cacheMaxAge(targetDate, queryType) == 0

	// Past periods: always serve from cache — the data will never change, so a
	// live call would waste budget and return identical results.
	if cacheErr == nil && isPast {
		cache.Debugf("serving cache (past period, age %s): %s", time.Since(cached.CachedAt).Round(time.Second), cache.RedactURLKey(url))
		return cached.ToHTTPResponse(), true, nil
	}

	// Current periods (today / current month / current year / active true-up):
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
		return nil, false, fmt.Errorf(constants.RateLimitError)
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
	if resp.StatusCode == 429 {
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
		return nil, false, fmt.Errorf(constants.RateLimitError)
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
	// nothing, and the per-query-type freshness policy was already applied
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

// getEnergyLifetime fetches production data using the energy_lifetime endpoint (daily aggregated).
// This endpoint has no 7-day limit and returns daily totals instead of 15-minute intervals.
func (c *EnlightenCloudClient) getEnergyLifetime(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	periodStart, _ := timezone.GetBoundaries(testDate, queryType, c.timezone)
	startDateStr := periodStart.Format(constants.DateFormat)
	// Cap to yesterday for ongoing periods so the total covers only complete days.
	// For past periods, the period end is already a complete day so we use it directly.
	endDateStr := c.lifetimeEndDate(testDate, queryType)

	// Build URL for lifetime endpoint
	reqURL := fmt.Sprintf("%s/%s/energy_lifetime?key=%s&start_date=%s", c.baseURL, c.systemID, c.apiKey, startDateStr)

	resp, cacheUsed, err := c.makeCachedAPIRequest(ctx, reqURL, testDate, queryType)
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

// getConsumptionLifetime fetches consumption data using the consumption_lifetime endpoint.
func (c *EnlightenCloudClient) getConsumptionLifetime(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	periodStart, _ := timezone.GetBoundaries(testDate, queryType, c.timezone)
	startDateStr := periodStart.Format(constants.DateFormat)
	endDateStr := c.lifetimeEndDate(testDate, queryType)

	reqURL := fmt.Sprintf("%s/%s/consumption_lifetime?key=%s&start_date=%s", c.baseURL, c.systemID, c.apiKey, startDateStr)

	resp, cacheUsed, err := c.makeCachedAPIRequest(ctx, reqURL, testDate, queryType)
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

// getEnergyImportLifetime fetches grid import data using the energy_import_lifetime endpoint.
func (c *EnlightenCloudClient) getEnergyImportLifetime(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	periodStart, _ := timezone.GetBoundaries(testDate, queryType, c.timezone)
	startDateStr := periodStart.Format(constants.DateFormat)
	endDateStr := c.lifetimeEndDate(testDate, queryType)

	reqURL := fmt.Sprintf("%s/%s/energy_import_lifetime?key=%s&start_date=%s", c.baseURL, c.systemID, c.apiKey, startDateStr)

	resp, cacheUsed, err := c.makeCachedAPIRequest(ctx, reqURL, testDate, queryType)
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

// getEnergyExportLifetime fetches grid export data using the energy_export_lifetime endpoint.
func (c *EnlightenCloudClient) getEnergyExportLifetime(ctx context.Context, testDate time.Time, queryType constants.QueryType) (float64, error) {
	periodStart, _ := timezone.GetBoundaries(testDate, queryType, c.timezone)
	startDateStr := periodStart.Format(constants.DateFormat)
	endDateStr := c.lifetimeEndDate(testDate, queryType)

	reqURL := fmt.Sprintf("%s/%s/energy_export_lifetime?key=%s&start_date=%s", c.baseURL, c.systemID, c.apiKey, startDateStr)

	resp, cacheUsed, err := c.makeCachedAPIRequest(ctx, reqURL, testDate, queryType)
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
