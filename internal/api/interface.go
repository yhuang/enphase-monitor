// Package api provides abstractions and implementations for interacting with
// the Enphase Enlighten Cloud API v4.
//
// PURPOSE
// -------
// This package defines the contract for fetching energy metrics from Enphase
// systems and provides the implementation for the Enphase Cloud API v4.
//
// ARCHITECTURE
// ------------
// - CloudClient interface: Defines the public API contract
// - EnlightenCloudClient: Concrete implementation for Enphase Cloud API v4
// - LocalMetrics: Return type containing all energy metrics
//
// USAGE
// -----
// The CloudClient interface enables dependency injection and testing:
//
//	var client api.CloudClient
//	client = api.NewEnlightenCloudClient(systemID, apiKey, token, tz)
//	metrics, cacheUsed, err := client.GetMetricsFromCloud(ctx, testDate)
//
// For testing, implement CloudClient with a mock:
//
//	type MockClient struct{}
//	func (m *MockClient) GetMetricsFromCloud(...) (*api.LocalMetrics, bool, error) {
//	    return &api.LocalMetrics{...}, false, nil
//	}
package api

import (
	"context"
	"time"

	"enphase-monitor/internal/constants"
)

// CloudClient defines the interface for fetching energy metrics from a cloud API.
//
// This interface abstracts the details of API communication, allowing for:
//   - Dependency injection in production code
//   - Easy mocking for unit tests
//   - Future support for alternative API providers
//
// All methods accept a context for cancellation/timeout control and an optional
// testDate parameter (zero value means "today").
type CloudClient interface {
	// GetMetricsFromCloud fetches all energy metrics for a system.
	// This is the primary method - it aggregates all individual metrics into one call.
	//
	// Parameters:
	//   - ctx: Context for request cancellation/timeout
	//   - testDate: Date to query (zero value = today)
	//   - queryMode: Query Mode (Day, Month, Year, or True-Up)
	//
	// Returns:
	//   - *LocalMetrics: All energy metrics for the period
	//   - bool: Whether cached data was used (true = cache, false = live API)
	//   - error: Any error encountered during fetching
	GetMetricsFromCloud(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (*LocalMetrics, bool, error)

	// GetEnergyImportForDate gets the total Grid Import for a specific date/period.
	//
	// Parameters:
	//   - ctx: Context for request cancellation/timeout
	//   - testDate: Date to query (zero value = today)
	//   - queryMode: Query Mode (Day, Month, Year, or True-Up)
	//
	// Returns:
	//   - float64: Grid Import in kWh
	//   - error: Any error encountered
	GetEnergyImportForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error)

	// GetEnergyExportForDate gets the total Grid Export for a specific date/period.
	//
	// Parameters:
	//   - ctx: Context for request cancellation/timeout
	//   - testDate: Date to query (zero value = today)
	//   - queryMode: Query Mode (Day, Month, Year, or True-Up)
	//
	// Returns:
	//   - float64: Grid Export in kWh
	//   - error: Any error encountered
	GetEnergyExportForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error)

	// GetProductionForDate gets the total Production for a specific date/period.
	//
	// Parameters:
	//   - ctx: Context for request cancellation/timeout
	//   - testDate: Date to query (zero value = today)
	//   - queryMode: Query Mode (Day, Month, Year, or True-Up)
	//
	// Returns:
	//   - float64: Production in kWh
	//   - error: Any error encountered
	GetProductionForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error)

	// GetConsumptionForDate gets the total Consumption for a specific date/period.
	//
	// Parameters:
	//   - ctx: Context for request cancellation/timeout
	//   - testDate: Date to query (zero value = today)
	//   - queryMode: Query Mode (Day, Month, Year, or True-Up)
	//
	// Returns:
	//   - float64: Consumption in kWh
	//   - error: Any error encountered
	GetConsumptionForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error)

	// GetBatteryDataForDate gets Battery Charge, Battery Discharge, and State of Charge (SOC) for a specific date/period.
	//
	// Parameters:
	//   - ctx: Context for request cancellation/timeout
	//   - testDate: Date to query (zero value = today)
	//   - queryMode: Query Mode (Day, Month, Year, or True-Up)
	//
	// Returns:
	//   - charged: Battery Charge in kWh
	//   - discharged: Battery Discharge in kWh
	//   - soc: Battery State of Charge (SOC), 0–100 (percent)
	//   - error: Any error encountered
	GetBatteryDataForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (charged float64, discharged float64, soc int, err error)
}

// Compile-time check that *EnlightenCloudClient implements CloudClient.
// If the interface or the implementation changes, this file fails to build.
var _ CloudClient = (*EnlightenCloudClient)(nil)
