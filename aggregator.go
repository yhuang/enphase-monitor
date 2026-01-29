// Package main - aggregator.go
//
// PURPOSE
// -------
// This file implements the DataAggregator, which orchestrates data collection
// from multiple Enphase systems and combines their metrics into a combined report.
//
// For detailed architecture documentation, see ARCHITECTURE.md sections:
//   - "Execution Flow" for the overall data collection process
//   - "Key Go Patterns Used" for the Facade pattern explanation
//
// ARCHITECTURE PATTERN: Facade
// ----------------------------
// The aggregator acts as a Facade pattern, providing a simple interface
// (GetAggregatedMetrics) that hides the complexity of:
//   - OAuth token management (delegates to oauth.go)
//   - API client creation (creates EnlightenCloudClient per system)
//   - Rate limit error handling (collects and reports at end)
//   - Metric aggregation (applies rules below)
//
// HOW IT WORKS
// ------------
//  1. Receives list of systems from config and optional test date
//  2. For each system:
//     a. Gets OAuth access token (cached or refreshed via oauth.go)
//     b. Creates EnlightenCloudClient for that system's ID
//     c. Calls GetMetricsFromCloud() to fetch all metrics
//     d. Stores individual system metrics
//     e. Adds to aggregated totals
//  3. Aggregates battery metrics across systems
//  4. Returns AggregatedMetrics with combined data
//
// METRIC DERIVATION AND CALCULATION
// ----------------------------------
// This section documents how each metric in the report is derived, including
// the API endpoints, raw data fields, and formulas used for calculation.
//
// ┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
// │  Metric                      │  API Endpoint                                   │  Raw Data Field              │  Calculation             │
// ├──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
// │  Production                  │  GET /api/v4/systems/{id}/                      │  intervals[].wh_del          │  Sum all wh_del          │
// │  (Solar Generation)          │    telemetry/production_meter                   │  (watt-hours)                │  values, convert         │
// │                              │                                                 │                              │  Wh → kWh (÷1000)        │
// │                              │                                                 │                              │                          │
// │                              │  Data Source: 15-minute interval                │                              │  Formula:                │
// │                              │  telemetry from production meter                │                              │  Σ(wh_del) / 1000        │
// │                              │                                                 │                              │                          │
// ├──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
// │  Consumption                 │  GET /api/v4/systems/{id}/                      │  intervals[].enwh            │  Sum all enwh            │
// │  (Home Usage)                │    telemetry/consumption_meter                  │  (watt-hours)                │  values, convert         │
// │                              │                                                 │                              │  Wh → kWh (÷1000)        │
// │                              │                                                 │                              │                          │
// │                              │  Data Source: 15-minute interval                │                              │  Formula:                │
// │                              │  telemetry from consumption meter               │                              │  Σ(enwh) / 1000          │
// │                              │                                                 │                              │                          │
// │                              │  Fallback (if API fails):                       │                              │  Alternative:            │
// │                              │  Production + Grid Import -                     │                              │  Production +            │
// │                              │  Grid Export - Battery Charged +                │                              │  Grid Import -           │
// │                              │  Battery Discharged                             │                              │  Grid Export -           │
// │                              │                                                 │                              │  Battery Charged +       │
// │                              │                                                 │                              │  Battery Discharged      │
// │                              │                                                 │                              │                          │
// ├──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
// │  Grid Import                 │  GET /api/v4/systems/{id}/                      │  intervals[][].              │  Sum all                 │
// │  (Energy from Grid)          │    energy_import_telemetry                      │    wh_imported               │  wh_imported values,     │
// │                              │                                                 │  (watt-hours)                │  convert Wh → kWh        │
// │                              │                                                 │                              │                          │
// │                              │  Data Source: Nested array of                   │                              │  Formula:                │
// │                              │  15-minute interval telemetry                   │                              │  Σ(wh_imported) / 1000   │
// │                              │  from grid import meter                         │                              │                          │
// │                              │                                                 │                              │                          │
// ├──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
// │  Grid Export                 │  GET /api/v4/systems/{id}/                      │  intervals[][].              │  Sum all                 │
// │  (Energy to Grid)            │    energy_export_telemetry                      │    wh_exported               │  wh_exported values,     │
// │                              │                                                 │  (watt-hours)                │  convert Wh → kWh        │
// │                              │                                                 │                              │                          │
// │                              │  Data Source: Nested array of                   │                              │  Formula:                │
// │                              │  15-minute interval telemetry                   │                              │  Σ(wh_exported) / 1000   │
// │                              │  from grid export meter                         │                              │                          │
// │                              │                                                 │                              │                          │
// ├──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
// │  Net Energy Flow             │  Calculated from Grid Import/Export             │  N/A (calculated)            │  Grid Import -           │
// │  (Net Import/Export)         │                                                 │                              │  Grid Export             │
// │                              │                                                 │                              │                          │
// │                              │  Positive = Net Import (more energy             │                              │  Formula:                │
// │                              │  imported than exported)                        │                              │  Grid Import -           │
// │                              │  Negative = Net Export (more energy             │                              │  Grid Export             │
// │                              │  exported than imported)                        │                              │                          │
// │                              │                                                 │                              │                          │
// ├──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
// │  Battery Charged             │  GET /api/v4/systems/{id}/                      │  intervals[].charge.         │  Sum all charge.enwh     │
// │  (Energy Stored)             │    telemetry/battery                            │    enwh                      │  values, convert         │
// │                              │                                                 │  (watt-hours)                │  Wh → kWh (÷1000)        │
// │                              │                                                 │                              │                          │
// │                              │  Data Source: 15-minute interval                │                              │  Formula:                │
// │                              │  telemetry from battery, filtered by            │                              │  Σ(charge.enwh) / 1000   │
// │                              │  configured timezone day boundaries             │                              │  (filtered by date)      │
// │                              │                                                 │                              │                          │
// ├──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
// │  Battery Discharged          │  GET /api/v4/systems/{id}/                      │  intervals[].                │  Sum all                 │
// │  (Energy Used)               │    telemetry/battery                            │    discharge.enwh            │  discharge.enwh          │
// │                              │                                                 │  (watt-hours)                │  values, convert         │
// │                              │                                                 │                              │  Wh → kWh (÷1000)        │
// │                              │                                                 │                              │                          │
// │                              │  Data Source: 15-minute interval                │                              │  Formula:                │
// │                              │  telemetry from battery, filtered by            │                              │  Σ(discharge.enwh) /     │
// │                              │  configured timezone day boundaries             │                              │  1000 (filtered)         │
// │                              │                                                 │                              │                          │
// ├──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
// │  Battery SOC                 │  GET /api/v4/systems/{id}/                      │  last_reported_              │  Parse percentage        │
// │  (State of Charge)           │    telemetry/battery                            │    aggregate_soc             │  string (e.g., "97%")    │
// │                              │                                                 │  (string: "97%")             │  to integer for each     │
// │                              │  Data Source: String field from                 │                              │  system                  │
// │                              │  battery telemetry response                     │                              │                          │
// │                              │                                                 │                              │  Formula:                │
// │                              │  Note: Battery State of Charge (SOC) is         │                              │  Parse "97%" → 97        │
// │                              │  shown per-system only in individual reports.   │                              │                          │
// │                              │                                                 │                              │                          │
// └──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
//
// AGGREGATION RULES
// -----------------
// Different metrics require different aggregation strategies when combining data from multiple systems:
//
// ┌──────────────────────────────────────────────────────────────────────────────────┐
// │  Metric              │  Method          │  Rationale                             │
// ├──────────────────────────────────────────────────────────────────────────────────┤
// │  Production          │  SUM             │  Each system has independent solar     │
// │                      │                  │  panels - total production is sum      │
// ├──────────────────────────────────────────────────────────────────────────────────┤
// │  Consumption         │  SUM             │  Each system monitors separate         │
// │                      │                  │  consumption - total is sum            │
// ├──────────────────────────────────────────────────────────────────────────────────┤
// │  Grid Import         │  SUM             │  Each system has separate grid         │
// │                      │                  │  connection - total import is sum      │
// ├──────────────────────────────────────────────────────────────────────────────────┤
// │  Grid Export         │  SUM             │  Each system exports independently     │
// │                      │                  │  - total export is sum                 │
// ├──────────────────────────────────────────────────────────────────────────────────┤
// │  Battery Charged     │  NOT AGGREGATED  │  Battery charge/discharge shown        │
// │  Battery Discharged  │                  │  per-system only in individual         │
// │                      │                  │  reports. Not aggregated.              │
// ├──────────────────────────────────────────────────────────────────────────────────┤
// │  Battery SOC         │  NOT AGGREGATED  │  State of charge is shown per-system   │
// │                      │                  │  only. Combined SOC is not calculated  │
// │                      │                  │  as it is not meaningful across        │
// │                      │                  │  systems with different capacities.    │
// └──────────────────────────────────────────────────────────────────────────────────┘
//
// DATA PROCESSING DETAILS
// -----------------------
//
//  1. **Interval Filtering**: All telemetry data uses 15-minute intervals. Intervals are filtered
//     to include only those whose end time falls within the requested date range (configured timezone).
//
//  2. **Unit Conversion**: All raw API values are in watt-hours (Wh). The application converts to
//     kilowatt-hours (kWh) by dividing by 1000.
//
//  3. **Date Boundaries**: Day boundaries are calculated in the report timezone
//     to match Enphase's reporting. The day starts at 00:00:00 and ends at 23:59:59 in the report timezone.
//
//  4. **Incremental Values**: All interval values (wh_del, enwh, wh_imported, wh_exported, etc.)
//     represent energy during that 15-minute period, not cumulative totals. Daily totals are
//     calculated by summing all intervals for the day.
//
//  5. **Multi-System Aggregation**: When multiple systems are configured, individual system metrics
//     are summed. The combined report shows totals across all systems, while individual system reports
//     show per-system breakdowns.
//
// ERROR HANDLING STRATEGY
// -----------------------
// The aggregator uses a tiered error handling approach:
//
//  1. Rate Limit Errors (429):
//     - Collected but do not fail immediately
//     - Allows other systems to be queried
//     - All 429 errors reported together at the end
//     - Provides wait time guidance to user
//
//  2. Optional Data Failures:
//     - Grid import/export may fail (some systems do not have meters)
//     - Battery data may be unavailable (no battery installed)
//     - These are logged as warnings but do not stop aggregation
//
//  3. Required Data Failures:
//     - Production and consumption are required
//     - API configuration errors are required
//     - These cause immediate return with error
//
//  4. Partial Success:
//     - If some systems succeed and others fail, we continue
//     - Failed systems are skipped (no data in final report)
//     - User sees which systems had errors
//
// DATA STRUCTURES
// ---------------
// AggregatedMetrics: Contains combined metrics from all systems plus
//
//	individual system breakdowns
//
// SystemMetrics: Contains metrics for a single system, used both for
//
//	individual display and aggregation
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// AggregatedMetrics represents combined data from all Enphase systems
type AggregatedMetrics struct {
	Timestamp time.Time
	QueryDate time.Time // The date being queried (zero value means today)

	// Today's Energy (kWh)
	ProductionToday  float64
	ConsumptionToday float64
	GridImportToday  float64
	GridExportToday  float64

	// Individual System Data
	Systems []SystemMetrics

	// Cache status
	CacheUsed bool // Indicates if any cached data was used
}

// SystemMetrics represents metrics for a single system
type SystemMetrics struct {
	Name             string
	ID               string // System ID (for Cloud API)
	ProductionToday  float64
	ConsumptionToday float64
	BatterySOC       int

	// Today's energy values for this system
	GridImportToday        float64
	GridExportToday        float64
	BatteryChargedToday    float64
	BatteryDischargedToday float64
	NetImportedToday       float64
}

// DataAggregator handles combining data from multiple systems
//
// GO PATTERN: Stateless Service Object (empty struct with methods)
// The empty struct stores no data, but this pattern provides:
//   - Method grouping: organizes related functions under one namespace
//   - Interface readiness: can implement interfaces for mocking/testing
//   - Future extensibility: fields can be added later without changing call sites
//
// Alternative: plain package-level functions (simpler but harder to mock/extend)
type DataAggregator struct{}

// NewDataAggregator creates a new aggregator instance
func NewDataAggregator() *DataAggregator {
	return &DataAggregator{}
}

// GetAggregatedMetrics retrieves and combines data from all systems
// If testDate is provided, uses that date instead of today
// reportTimezone is the timezone to use for all systems' data queries (from config, system, or US/Pacific fallback)
func (a *DataAggregator) GetAggregatedMetrics(ctx context.Context, systems []SystemConfig, apiConfig *APIConfig, testDate time.Time, reportTimezone *time.Location) (*AggregatedMetrics, error) {
	metrics := &AggregatedMetrics{
		Timestamp: time.Now(),
		QueryDate: testDate, // time.Time zero value means "today"
		Systems:   make([]SystemMetrics, 0, len(systems)),
	}
	anyCacheUsed := false
	var rateLimitErrors []string // Collect 429 errors to print once at the end

	for _, sys := range systems {
		// Use Cloud API
		if apiConfig == nil {
			return nil, fmt.Errorf("api configuration required for system %s", sys.Name)
		}
		if apiConfig.Key == "" {
			return nil, fmt.Errorf("api.key required for system %s", sys.Name)
		}

		// Get OAuth access token using client credentials
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		accessToken, err := GetAccessToken(ctx, apiConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to get OAuth access token for system %s: %w", sys.Name, err)
		}

		// Use the report timezone for all systems (from config, system, or US/Pacific fallback)
		// Create Cloud API client with API key, access token, and report timezone
		cloudClient := NewEnlightenCloudClient(sys.ID, apiConfig.Key, accessToken, reportTimezone)

		var cacheUsed bool
		localMetrics, cacheUsed, err := cloudClient.GetMetricsFromCloud(ctx, testDate)
		if err != nil {
			if strings.Contains(err.Error(), RateLimitError) {
				// Collect the error but do not fail immediately
				rateLimitErrors = append(rateLimitErrors, fmt.Sprintf("System %s: %v", sys.Name, err))
				continue
			}
			// For other errors, return immediately (fail fast)
			return nil, fmt.Errorf("failed to get metrics from Cloud API for system %s: %w", sys.Name, err)
		}
		// Track if any system used cache
		if cacheUsed {
			anyCacheUsed = true
		}

		// Calculate net imported today
		netImportedToday := localMetrics.GridImportToday - localMetrics.GridExportToday

		systemMetrics := SystemMetrics{
			Name:                   sys.Name,
			ID:                     sys.ID,
			ProductionToday:        localMetrics.ProductionToday,
			ConsumptionToday:       localMetrics.ConsumptionToday,
			BatterySOC:             localMetrics.BatterySOC,
			GridImportToday:        localMetrics.GridImportToday,
			GridExportToday:        localMetrics.GridExportToday,
			BatteryChargedToday:    localMetrics.BatteryChargedToday,
			BatteryDischargedToday: localMetrics.BatteryDischargedToday,
			NetImportedToday:       netImportedToday,
		}

		metrics.Systems = append(metrics.Systems, systemMetrics)

		// Aggregate totals
		// Production: sum across systems (each system has its own solar panels)
		metrics.ProductionToday += localMetrics.ProductionToday

		// Consumption: sum across systems
		metrics.ConsumptionToday += localMetrics.ConsumptionToday

		// Grid import/export: sum across systems
		metrics.GridImportToday += localMetrics.GridImportToday
		metrics.GridExportToday += localMetrics.GridExportToday
	}

	metrics.CacheUsed = anyCacheUsed

	// If we collected any 429 errors that could not be resolved with cache, print them once and exit
	if len(rateLimitErrors) > 0 {
		// Enphase API rate limit window is 60 seconds (10 requests/minute for free tier)
		fmt.Fprintf(os.Stderr, "ERROR: API rate limit exceeded (429)\n")
		fmt.Fprintf(os.Stderr, "Please wait 60 seconds before rerunning the program.\n")
		return nil, fmt.Errorf("rate limit exceeded (429): %d system(s) affected", len(rateLimitErrors))
	}

	return metrics, nil
}
