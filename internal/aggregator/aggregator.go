// Package aggregator implements core aggregation logic for combining metrics
// from multiple Enphase systems into a single report.
//
// DataAggregator uses dependency injection (OAuth token getter, cloud client
// factory) for testability. Types such as AggregatedMetrics and SystemMetrics
// are defined in types.go.
package aggregator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"enphase-monitor/internal/api"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/types"
)

// SystemConfig is an alias for types.SystemConfig.
// This maintains backward compatibility while using the shared type definition.
type SystemConfig = types.SystemConfig

// APIConfig is an alias for types.APIConfig.
// This maintains backward compatibility while using the shared type definition.
type APIConfig = types.APIConfig

// OAuthTokenGetter is a function type for getting OAuth access tokens.
// This allows the aggregator to work with any OAuth implementation.
type OAuthTokenGetter func(ctx context.Context, apiConfig *APIConfig) (string, error)

// CloudClientFactory is a function type for creating cloud clients.
// This allows dependency injection for testing purposes.
type CloudClientFactory func(systemID, systemName, apiKey, accessToken string, timezone *time.Location) api.CloudClient

// defaultCloudClientFactory creates the default production cloud client.
func defaultCloudClientFactory(systemID, systemName, apiKey, accessToken string, tz *time.Location) api.CloudClient {
	return api.NewEnlightenCloudClient(systemID, apiKey, accessToken, tz).WithSystemName(systemName)
}

// DataAggregator handles combining data from multiple systems.
//
// GO PATTERN: Service Object with Dependency Injection
// This struct uses dependency injection for:
//   - getAccessToken: OAuth token retrieval (injected at construction)
//   - createCloudClient: Cloud client creation (injectable for testing)
//
// Benefits:
//   - Method grouping: organizes related functions under one namespace
//   - Interface readiness: can implement interfaces for mocking/testing
//   - Testability: dependencies can be mocked via factory functions
//   - Future extensibility: fields can be added later without changing call sites
type DataAggregator struct {
	getAccessToken    OAuthTokenGetter
	createCloudClient CloudClientFactory
}

// NewDataAggregator creates a new aggregator instance with the specified OAuth token getter.
// Uses the default cloud client factory for production use.
func NewDataAggregator(getAccessToken OAuthTokenGetter) *DataAggregator {
	return &DataAggregator{
		getAccessToken:    getAccessToken,
		createCloudClient: defaultCloudClientFactory,
	}
}

// NewDataAggregatorWithFactory creates an aggregator with a custom cloud client factory.
// This is primarily used for testing to inject mock cloud clients.
func NewDataAggregatorWithFactory(getAccessToken OAuthTokenGetter, factory CloudClientFactory) *DataAggregator {
	return &DataAggregator{
		getAccessToken:    getAccessToken,
		createCloudClient: factory,
	}
}

// GetAggregatedMetrics retrieves and combines data from all systems
// If testDate is provided, uses that date instead of today
// queryType specifies the query granularity (day/month/year)
// reportTimezone is the timezone to use for all systems' data queries (from config, system, or US/Pacific fallback)
func (a *DataAggregator) GetAggregatedMetrics(ctx context.Context, systems []SystemConfig, apiConfig *APIConfig, testDate time.Time, queryType constants.QueryType, reportTimezone *time.Location) (*AggregatedMetrics, error) {
	metrics := &AggregatedMetrics{
		Timestamp: time.Now(),
		QueryDate: testDate,    // time.Time zero value means "today"
		QueryType: queryType,   // Query granularity
		Systems:   make([]SystemMetrics, 0, len(systems)),
	}
	anyCacheUsed := false
	allFromCache := len(systems) > 0 // optimistic; flipped false if any live call is made
	var rateLimitErrors []string      // Collect 429 errors to report at the end

	for _, sys := range systems {
		// Use Cloud API
		if apiConfig == nil {
			return nil, fmt.Errorf("%s for system %s", constants.ErrAPIConfigRequired, sys.Name)
		}
		if apiConfig.Key == "" {
			return nil, fmt.Errorf("api.key required for system %s", sys.Name)
		}

		// Get OAuth access token using client credentials
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		accessToken, err := a.getAccessToken(ctx, apiConfig)
		if err != nil {
			return nil, fmt.Errorf("%s for system %s: %w", constants.ErrTokenRefreshFailed, sys.Name, err)
		}

		// Use the report timezone for all systems (from config, system, or US/Pacific fallback)
		// Create Cloud API client with API key, access token, and report timezone
		cloudClient := a.createCloudClient(sys.ID, sys.Name, apiConfig.Key, accessToken, reportTimezone)

		var cacheUsed bool
		localMetrics, cacheUsed, err := cloudClient.GetMetricsFromCloud(ctx, testDate, queryType)
		if err != nil && constants.IsRateLimitError(err) {
			rateLimitErrors = append(rateLimitErrors, fmt.Sprintf("System %s: %v", sys.Name, err))
			allFromCache = false
			continue
		}
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, fmt.Errorf("failed to get metrics from Cloud API for system %s: %w", sys.Name, err)
			}
			fmt.Printf("WARNING: [%s] Failed to get metrics, skipping: %v\n", sys.Name, err)
			allFromCache = false
			continue
		}
		// Track cache status
		if cacheUsed {
			anyCacheUsed = true
		} else {
			allFromCache = false
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

	// Calculate net import (positive = net import, negative = net export)
	metrics.NetImportToday = metrics.GridImportToday - metrics.GridExportToday

	metrics.CacheUsed = anyCacheUsed
	metrics.AllFromCache = allFromCache

	// If we collected any 429 errors that could not be resolved with cache, return error
	if len(rateLimitErrors) > 0 {
		return nil, fmt.Errorf("API rate limit exceeded (429): %d system(s) affected", len(rateLimitErrors))
	}

	return metrics, nil
}
