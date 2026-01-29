// Package main - cloud_client.go
//
// PURPOSE
// -------
// This file implements the HTTP client for the Enphase Enlighten Cloud API v4.
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
// See response_parser.go for parsing logic that handles both formats.
//
// API ENDPOINTS
// -------------
// This client uses the following Enlighten Cloud API v4 endpoints:
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
// All endpoints accept query parameters:
//   - start_at: Unix timestamp (start of date range)
//   - end_at: Unix timestamp (end of date range)
//   - key: API key (required)
//
// RATE LIMITING
// -------------
// The API enforces rate limits (typically 10 requests/minute for free tier).
// This client relies on api_cache.go for caching responses to reduce API calls.
// On 429 errors, cached responses are used as fallback when available.
//
// ERROR HANDLING
// --------------
// - 401 Unauthorized: Invalid or expired access token (oauth.go handles refresh)
// - 429 Too Many Requests: Rate limit exceeded (api_cache.go handles caching)
// - 500 Server Error: Returned to caller (cache fallback if available)
// - Network errors: Returned to caller for handling
package main

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
)

// EnlightenCloudClient handles communication with Enphase Enlighten Cloud API v4.
// It manages authentication, request formatting, and response parsing for
// a specific system ID.
type EnlightenCloudClient struct {
	systemID    string
	apiKey      string
	accessToken string
	timezone    *time.Location // Timezone for reporting/queries
	httpClient  *http.Client
	cacheUsed   bool // Tracks if cache was used for the last request
}

// NewEnlightenCloudClient creates a new client for Enlighten Cloud API with API key and OAuth token
func NewEnlightenCloudClient(systemID, apiKey, accessToken string, timezone *time.Location) *EnlightenCloudClient {
	return &EnlightenCloudClient{
		systemID:    systemID,
		apiKey:      apiKey,
		accessToken: accessToken,
		timezone:    timezone,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// fetchTelemetryData is a helper method that reduces redundant code across Get*ForDate methods.
// It handles the common pattern of: make request, track cache usage, read body, close response.
// This helper eliminates ~15 lines of boilerplate per method (5 methods = ~75 lines saved).
func (c *EnlightenCloudClient) fetchTelemetryData(ctx context.Context, endpoint string, testDate time.Time) ([]byte, error) {
	dayStart, dayEnd := getDayBoundaries(testDate, c.timezone)
	reqURL := buildTelemetryURL(c.systemID, endpoint, c.apiKey, dayStart, dayEnd)

	resp, cacheUsed, err := c.makeCachedAPIRequest(ctx, reqURL, testDate)
	c.cacheUsed = cacheUsed
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return readResponseBody(resp.Body)
}

// TelemetryResponse represents the response from telemetry endpoints
// Note: production_meter, consumption_meter, battery return a single array
type TelemetryResponse struct {
	LastReportedAggregateSOC string              `json:"last_reported_aggregate_soc,omitempty"` // Battery state of charge percentage as string (e.g., "97%")
	Intervals                []TelemetryInterval `json:"intervals"`
}

// TelemetryResponseNested represents the response from energy_import_telemetry and energy_export_telemetry
// which return intervals as an array of arrays
type TelemetryResponseNested struct {
	Intervals [][]TelemetryInterval `json:"intervals"`
}

// TelemetryInterval represents a single 15-minute interval
type TelemetryInterval struct {
	EndAt      int64   `json:"end_at"`      // Unix timestamp
	WhDel      float64 `json:"wh_del"`      // Energy delivered (for production_meter)
	WhRcv      float64 `json:"wh_rcv"`      // Energy received (legacy - not used in current endpoints)
	WhImported float64 `json:"wh_imported"` // Energy imported (for energy_import_telemetry)
	WhExported float64 `json:"wh_exported"` // Energy exported (for energy_export_telemetry)
	Enwh       float64 `json:"enwh"`        // Energy in Wh (for production_meter, consumption_meter)
	Charge     struct {
		Enwh float64 `json:"enwh"` // Energy charged in Wh
	} `json:"charge"`
	Discharge struct {
		Enwh float64 `json:"enwh"` // Energy discharged in Wh
	} `json:"discharge"`
}

// LocalMetrics contains processed metrics from the Cloud API
// This struct is used to standardize the format of metrics returned from GetMetricsFromCloud
type LocalMetrics struct {
	Timestamp              time.Time // When these metrics were collected
	ProductionToday        float64   // kWh - Solar energy produced today
	ConsumptionToday       float64   // kWh - Energy consumed today
	GridImportToday        float64   // kWh - Energy imported from grid today
	GridExportToday        float64   // kWh - Energy exported to grid today
	BatteryChargedToday    float64   // kWh - Energy charged to battery today
	BatteryDischargedToday float64   // kWh - Energy discharged from battery today
	BatterySOC             int       // State of charge percentage (0-100)
}

// GetEnergyImportForDate gets the total energy imported from the grid for a specific date.
// If testDate is nil, uses today. Uses energy_import_telemetry endpoint which respects date filtering.
func (c *EnlightenCloudClient) GetEnergyImportForDate(ctx context.Context, testDate time.Time) (float64, error) {
	bodyBytes, err := c.fetchTelemetryData(ctx, "energy_import_telemetry", testDate)
	if err != nil {
		return 0, err
	}

	// For energy_import_telemetry, intervals is an array of arrays
	allIntervals, err := parseNestedTelemetryResponse(bodyBytes)
	if err != nil {
		return 0, err
	}

	// Sum intervals that fall within the requested day (in configured timezone)
	// For import telemetry: WhImported = energy imported per interval
	// These are incremental values per 15-minute interval, so we sum them
	// Note: The API returns intervals for the requested date range, so we can sum all intervals
	if len(allIntervals) == 0 {
		return 0, nil // No intervals means no import for this date
	}

	// Sum all wh_imported values
	importWh := sumIntervalValues(allIntervals, "wh_imported")
	return importWh / 1000.0, nil // Convert Wh to kWh
}

// GetEnergyExportForDate gets the total energy exported to the grid for a specific date.
// If testDate is nil, uses today. Uses energy_export_telemetry endpoint which respects date filtering.
func (c *EnlightenCloudClient) GetEnergyExportForDate(ctx context.Context, testDate time.Time) (float64, error) {
	bodyBytes, err := c.fetchTelemetryData(ctx, "energy_export_telemetry", testDate)
	if err != nil {
		return 0, err
	}

	// For energy_export_telemetry, intervals is an array of arrays
	allIntervals, err := parseNestedTelemetryResponse(bodyBytes)
	if err != nil {
		return 0, err
	}

	// Sum intervals that fall within the requested day (in configured timezone)
	// For export telemetry: WhExported = energy exported per interval
	// These are incremental values per 15-minute interval, so we sum them
	exportWh := sumIntervalValues(allIntervals, "wh_exported")
	return exportWh / 1000.0, nil // Convert Wh to kWh
}

// GetProductionForDate gets the total energy production for a specific date.
// If testDate is nil, uses today. Uses telemetry/production_meter endpoint which respects date filtering.
// Returns the aggregated sum of all wh_del values from the API response.
func (c *EnlightenCloudClient) GetProductionForDate(ctx context.Context, testDate time.Time) (float64, error) {
	bodyBytes, err := c.fetchTelemetryData(ctx, "telemetry/production_meter", testDate)
	if err != nil {
		return 0, err
	}

	// Try parsing as nested array format first (like import/export endpoints)
	// Some endpoints may return nested arrays, others return single arrays
	allIntervals, err := parseNestedTelemetryResponse(bodyBytes)
	if err != nil {
		// If nested parsing fails, try single array format
		allIntervals, err = parseTelemetryResponse(bodyBytes)
		if err != nil {
			return 0, err
		}
	}

	// Sum all wh_del values from intervals
	// For production telemetry: WhDel = energy produced per interval
	// These are incremental values per 15-minute interval
	productionWh := sumIntervalValues(allIntervals, "wh_del")
	return productionWh / 1000.0, nil // Convert Wh to kWh
}

// GetConsumptionForDate gets the total energy consumption for a specific date.
// If testDate is nil, uses today. Uses telemetry/consumption_meter endpoint which respects date filtering.
func (c *EnlightenCloudClient) GetConsumptionForDate(ctx context.Context, testDate time.Time) (float64, error) {
	bodyBytes, err := c.fetchTelemetryData(ctx, "telemetry/consumption_meter", testDate)
	if err != nil {
		return 0, err
	}

	// Parse telemetry response (single array format)
	intervals, err := parseTelemetryResponse(bodyBytes)
	if err != nil {
		return 0, err
	}

	// Sum intervals that fall within the requested day (in configured timezone)
	// For consumption telemetry: Enwh = energy consumed per interval
	// These are incremental values per 15-minute interval, so we sum them
	consumptionWh := sumIntervalValues(intervals, "enwh")
	return consumptionWh / 1000.0, nil // Convert Wh to kWh
}

// GetBatteryDataForDate gets battery charge, discharge, and State of Charge (SOC) for a specific date.
// If testDate is nil, uses today. Uses /telemetry/battery endpoint which returns both charge and discharge in one call.
// Returns charged kWh, discharged kWh, and SOC percentage from last_reported_aggregate_soc.
// Note: Charge and discharge calculations are unchanged - SOC is extracted separately from the response.
func (c *EnlightenCloudClient) GetBatteryDataForDate(ctx context.Context, testDate time.Time) (charged float64, discharged float64, soc int, err error) {
	bodyBytes, err := c.fetchTelemetryData(ctx, "telemetry/battery", testDate)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("battery API request failed: %w", err)
	}

	var data TelemetryResponse
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return 0, 0, 0, fmt.Errorf("parsing battery response JSON: %w", err)
	}

	// Extract SOC from last_reported_aggregate_soc field (format: "97%" as string)
	socPercent := 0
	if data.LastReportedAggregateSOC != "" {
		// Parse string like "97%" to integer 97
		socStr := strings.TrimSuffix(data.LastReportedAggregateSOC, "%")
		if parsedSOC, err := strconv.Atoi(socStr); err == nil {
			socPercent = parsedSOC
		}
	}

	// Sum intervals that fall within the requested day (in configured timezone)
	// For battery telemetry:
	// - charge.enwh = energy charged to battery (Wh) per interval
	// - discharge.enwh = energy discharged from battery (Wh) per interval
	// These are incremental values per 15-minute interval, so we sum them
	// Filter by configured timezone (dayStart to dayEnd)
	dayStart, dayEnd := getDayBoundaries(testDate, c.timezone)
	var chargeWh, dischargeWh float64

	for _, interval := range data.Intervals {
		// Check if this interval's end time falls within the requested time range
		// The interval.EndAt is a Unix timestamp - convert to report timezone
		intervalEndTime := time.Unix(interval.EndAt, 0).In(c.timezone)

		// Include interval if its end time is within [dayStart, dayEnd] (inclusive both ends)
		// We use end time because the interval represents energy during the period ending at EndAt
		if (intervalEndTime.Equal(dayStart) || intervalEndTime.After(dayStart)) && (intervalEndTime.Equal(dayEnd) || intervalEndTime.Before(dayEnd)) {
			chargeWh += interval.Charge.Enwh       // Energy charged to battery
			dischargeWh += interval.Discharge.Enwh // Energy discharged from battery
		}
	}

	return chargeWh / 1000.0, dischargeWh / 1000.0, socPercent, nil // Convert Wh to kWh
}

// GetMetricsFromCloud fetches all today's metrics from the Cloud API
// If testDate is provided, uses that date instead of today
// Returns metrics and a boolean indicating if any cache was used
func (c *EnlightenCloudClient) GetMetricsFromCloud(ctx context.Context, testDate time.Time) (*LocalMetrics, bool, error) {
	metrics := &LocalMetrics{
		Timestamp: time.Now(),
	}
	cacheUsed := false

	// Fetch all metrics - make grid import/export optional (they may fail with 500)
	var err error

	metrics.GridImportToday, err = c.GetEnergyImportForDate(ctx, testDate)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		// Grid import may fail - continue with 0 (silently for past dates if cache not found)
		if isPastDate(testDate, c.timezone) {
			// For past dates, silently use 0 if cache not found
			metrics.GridImportToday = 0
		} else {
			// For today, log the error (but suppress 429 rate limit warnings - they're handled by cache)
			if !strings.Contains(err.Error(), RateLimitError) {
				fmt.Printf("WARNING: Failed to get grid import: %v\n", err)
			}
			metrics.GridImportToday = 0
		}
	}
	cacheUsed = cacheUsed || c.cacheUsed

	metrics.GridExportToday, err = c.GetEnergyExportForDate(ctx, testDate)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		// Grid export may fail - continue with 0 (silently for past dates if cache not found)
		if isPastDate(testDate, c.timezone) {
			// For past dates, silently use 0 if cache not found
			metrics.GridExportToday = 0
		} else {
			// For today, log the error (but suppress 429 rate limit warnings - they're handled by cache)
			if !strings.Contains(err.Error(), RateLimitError) {
				fmt.Printf("WARNING: Failed to get grid export: %v\n", err)
			}
			metrics.GridExportToday = 0
		}
	}
	cacheUsed = cacheUsed || c.cacheUsed

	metrics.ProductionToday, err = c.GetProductionForDate(ctx, testDate)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, fmt.Errorf("failed to get production: %w", err)
	}
	cacheUsed = cacheUsed || c.cacheUsed

	metrics.BatteryChargedToday, metrics.BatteryDischargedToday, metrics.BatterySOC, err = c.GetBatteryDataForDate(ctx, testDate)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		// Log the error but continue without battery data
		// This helps understand why battery data might be missing
		fmt.Printf("WARNING: Failed to get battery data: %v\n", err)
		metrics.BatteryChargedToday = 0
		metrics.BatteryDischargedToday = 0
		metrics.BatterySOC = 0
	}
	cacheUsed = cacheUsed || c.cacheUsed

	// Get consumption from API (more accurate than calculation)
	metrics.ConsumptionToday, err = c.GetConsumptionForDate(ctx, testDate)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
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
// │  ┌─────────────┐                                                        │
// │  │ Past Date?  │──YES──► Prefer cache, but allow API if no cache        │
// │  └──────┬──────┘                                                        │
// │         │ NO (Today)                                                    │
// │         ▼                                                               │
// │  ┌─────────────┐                                                        │
// │  │Cache Fresh? │──YES──► Return cache (avoid unnecessary API call)      │
// │  └──────┬──────┘                                                        │
// │         │ NO (Stale or missing)                                         │
// │         ▼                                                               │
// │  ┌─────────────┐                                                        │
// │  │ Make API    │──429──► Return stale cache if available                │
// │  │   Request   │──OK───► Save to cache, return fresh data               │
// │  └─────────────┘                                                        │
// └─────────────────────────────────────────────────────────────────────────┘
//
// Parameters:
//   - url: The full API URL to request
//   - targetDate: The date being queried (zero value means today)
//
// Returns:
//   - *http.Response: The response (from cache or live)
//   - bool: true if cache was used, false if fresh API call
//   - error: Any error encountered
func (c *EnlightenCloudClient) makeCachedAPIRequest(ctx context.Context, url string, targetDate time.Time) (*http.Response, bool, error) {
	// ─────────────────────────────────────────────────────────────────────────
	// SECTION 1: INITIALIZATION
	// Determine if we are querying a past date (affects caching strategy)
	// Use the report timezone
	// ─────────────────────────────────────────────────────────────────────────
	isPastDate := isPastDate(targetDate, c.timezone)

	// ─────────────────────────────────────────────────────────────────────────
	// SECTION 2: CACHE LOOKUP
	// First, try to load any existing cached response for this URL.
	// We will use this either directly (if valid) or as fallback (if API fails).
	// ─────────────────────────────────────────────────────────────────────────
	cached, cacheErr := loadCachedResponse(url, c.timezone)

	// If cache lookup failed for past dates, try to find cache by endpoint and date
	if cacheErr != nil && isPastDate && !targetDate.IsZero() {
		// Extract endpoint from URL
		parsedURL, err := neturl.Parse(url)
		if err == nil {
			pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
			var endpoint string
			if len(pathParts) >= 4 && pathParts[len(pathParts)-2] == "telemetry" {
				endpoint = "telemetry/" + pathParts[len(pathParts)-1]
			}

			// Extract system ID from URL
			var systemID string
			if len(pathParts) >= 3 {
				systemID = pathParts[len(pathParts)-3]
			}

			// Find cache entries by date
			// Use the report timezone
			targetDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, c.timezone)
			dateStr := targetDay.Format("2006-01-02")
			allEntries, err := ListCacheEntries()
			if err == nil {
				for _, entry := range allEntries {
					// Match by endpoint, system ID, and date
					// SystemID might be stored as string in cache, so compare as strings
					entrySystemID := entry.SystemID
					if entrySystemID == systemID && entry.Endpoint == endpoint && entry.Date == dateStr {
						// Found a matching cache entry - use it
						foundCached, err := loadCachedResponseByPath(entry.Path)
						if err == nil {
							cached = foundCached
							cacheErr = nil // Clear the error since we found a match
							break
						}
					}
				}
			}
		}
	}

	// ─────────────────────────────────────────────────────────────────────────
	// SECTION 4: TEST MODE HANDLING
	// In test mode, ONLY use cache - never make live API calls.
	// This allows validating behavior without hitting the real API.
	// ─────────────────────────────────────────────────────────────────────────
	if testMode {
		if cacheErr == nil {
			resp := cached.toHTTPResponse()
			return resp, true, nil // Cache was used (test mode)
		}
		// In test mode, provide more detailed error about missing cache
		cachePath := getCachePath(url, c.timezone)
		normalizedURL := normalizeURLForCache(url, c.timezone)
		return nil, false, fmt.Errorf("test mode: no cached response available. Cache path: %s, Normalized URL: %s", cachePath, redactURLKey(normalizedURL))
	}

	// ─────────────────────────────────────────────────────────────────────────
	// SECTION 5: NO-CACHE MODE HANDLING
	// When cache is disabled, skip cache lookup and always make live API calls.
	// Note: We still fall back to cache on 429 errors as a safety measure.
	// ─────────────────────────────────────────────────────────────────────────
	if cacheDisabled {
		// Make direct API request
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, false, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Even in no-cache mode, fall back to cache on error
			if cacheErr == nil {
				if !rateLimitWarningShown {
					fmt.Printf("WARNING: API error - returning cached data despite --no-cache flag\n")
					rateLimitWarningShown = true
				}
				resp := cached.toHTTPResponse()
				return resp, true, nil // Cache was used (error fallback in no-cache mode)
			}
			return nil, false, fmt.Errorf("API request failed: %w", err)
		}

		// Handle rate limit (429) specially
		if resp.StatusCode == 429 {
			resp.Body.Close()
			if cacheErr == nil {
				if !rateLimitWarningShown {
					fmt.Printf("WARNING: Rate limited (429) - returning cached data despite --no-cache flag\n")
					rateLimitWarningShown = true
				}
				resp := cached.toHTTPResponse()
				return resp, true, nil // Cache was used (429 fallback in no-cache mode)
			}
			return nil, false, fmt.Errorf(RateLimitError)
		}
		// Save response to cache for future use (even in no-cache mode)
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			resp.Body.Close()
			return nil, false, fmt.Errorf("failed to read response: %w", err)
		}
		resp.Body.Close()
		tempResp := &http.Response{
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
		}
		_ = saveCachedResponseFromBytes(url, tempResp, bodyBytes, c.timezone)
		return &http.Response{
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
			Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		}, false, nil // Cache was NOT used (no-cache mode)
	}

	// Check cache age if it exists
	var isCacheStale bool
	if cacheErr == nil && !cached.CachedAt.IsZero() {
		cacheAge := time.Since(cached.CachedAt)
		isCacheStale = cacheAge >= minRequestInterval
	}

	// For past dates (yesterday or earlier), prefer cache but allow live API calls if cache does not exist
	if isPastDate && cacheErr == nil {
		// Cache exists for past date - use it
		return cached.toHTTPResponse(), true, nil // Cache was used (past date)
	}

	// Make the API request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// If API call failed, try cache as fallback
		if cacheErr == nil {
			resp := cached.toHTTPResponse()
			return resp, true, nil // Cache was used (error fallback)
		}
		return nil, false, fmt.Errorf("API request failed: %w", err)
	}

	// Handle rate limit (429)
	if resp.StatusCode == 429 {
		resp.Body.Close()
		if cacheErr == nil {
			// Cache exists - use it silently
			resp := cached.toHTTPResponse()
			return resp, true, nil // Cache was used (429 fallback)
		}
		// Try one more time to load cache
		if retryCached, retryErr := loadCachedResponse(url, c.timezone); retryErr == nil {
			resp := retryCached.toHTTPResponse()
			return resp, true, nil // Cache was used (429 retry)
		}
		// No cache available for 429 - return error
		return nil, false, fmt.Errorf(RateLimitError)
	}

	// Handle other non-OK status codes
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// For other errors, try cache as fallback
		// For past dates, always prefer cache over live API errors
		if isPastDate {
			// Re-try cache lookup in case it failed initially
			if cacheErr == nil && !isCacheStale {
				// Cache exists and is valid - use it
				resp := cached.toHTTPResponse()
				return resp, true, nil // Cache was used (fallback for past date)
			}
			// Try one more time to load cache - maybe the initial lookup failed due to timing
			if retryCached, retryErr := loadCachedResponse(url, c.timezone); retryErr == nil {
				resp := retryCached.toHTTPResponse()
				return resp, true, nil // Cache was used (retry for past date)
			}
		} else {
			// For non-past dates, try cache as fallback if it exists and is not stale
			if cacheErr == nil && !isCacheStale {
				resp := cached.toHTTPResponse()
				return resp, true, nil // Cache was used (fallback)
			}
		}
		// Cache is stale (from a different day) or does not exist - return the error
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
	_ = saveCachedResponseFromBytes(url, tempResp, bodyBytes, c.timezone)

	// Return response with readable body
	return &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
	}, false, nil // Cache was NOT used (fresh API call)
}
