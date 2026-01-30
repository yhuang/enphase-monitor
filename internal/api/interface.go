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
	//
	// Returns:
	//   - *LocalMetrics: All energy metrics for the day
	//   - bool: Whether cached data was used (true = cache, false = live API)
	//   - error: Any error encountered during fetching
	GetMetricsFromCloud(ctx context.Context, testDate time.Time) (*LocalMetrics, bool, error)

	// GetEnergyImportForDate gets total energy imported from the grid for a specific date.
	//
	// Parameters:
	//   - ctx: Context for request cancellation/timeout
	//   - testDate: Date to query (zero value = today)
	//
	// Returns:
	//   - float64: Energy imported in kWh
	//   - error: Any error encountered
	GetEnergyImportForDate(ctx context.Context, testDate time.Time) (float64, error)

	// GetEnergyExportForDate gets total energy exported to the grid for a specific date.
	//
	// Parameters:
	//   - ctx: Context for request cancellation/timeout
	//   - testDate: Date to query (zero value = today)
	//
	// Returns:
	//   - float64: Energy exported in kWh
	//   - error: Any error encountered
	GetEnergyExportForDate(ctx context.Context, testDate time.Time) (float64, error)

	// GetProductionForDate gets total solar energy production for a specific date.
	//
	// Parameters:
	//   - ctx: Context for request cancellation/timeout
	//   - testDate: Date to query (zero value = today)
	//
	// Returns:
	//   - float64: Energy produced in kWh
	//   - error: Any error encountered
	GetProductionForDate(ctx context.Context, testDate time.Time) (float64, error)

	// GetConsumptionForDate gets total energy consumption for a specific date.
	//
	// Parameters:
	//   - ctx: Context for request cancellation/timeout
	//   - testDate: Date to query (zero value = today)
	//
	// Returns:
	//   - float64: Energy consumed in kWh
	//   - error: Any error encountered
	GetConsumptionForDate(ctx context.Context, testDate time.Time) (float64, error)

	// GetBatteryDataForDate gets battery charge, discharge, and state of charge for a specific date.
	//
	// Parameters:
	//   - ctx: Context for request cancellation/timeout
	//   - testDate: Date to query (zero value = today)
	//
	// Returns:
	//   - charged: Energy charged to battery in kWh
	//   - discharged: Energy discharged from battery in kWh
	//   - soc: State of charge percentage (0-100)
	//   - error: Any error encountered
	GetBatteryDataForDate(ctx context.Context, testDate time.Time) (charged float64, discharged float64, soc int, err error)
}
