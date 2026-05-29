// Package aggregator implements core aggregation logic for combining metrics
// from multiple Systems at a Site into a single report.
//
// DataAggregator uses dependency injection (OAuth token getter, cloud client
// factory) for testability. Types such as AggregatedMetrics and SystemMetrics
// are defined in types.go.
package aggregator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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

// CloudClient is the subset of the cloud API that the aggregator depends on.
// It is defined here, on the consumer side, so the aggregator owns the abstraction
// it actually needs (only GetMetricsFromCloud); api.NewEnlightenCloudClient returns a
// concrete *api.EnlightenCloudClient that satisfies it.
type CloudClient interface {
	GetMetricsFromCloud(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (*api.LocalMetrics, bool, error)
}

// Compile-time check that *api.EnlightenCloudClient satisfies CloudClient.
var _ CloudClient = (*api.EnlightenCloudClient)(nil)

// CloudClientFactory is a function type for creating cloud clients.
// This allows dependency injection for testing purposes.
type CloudClientFactory func(systemID, systemName, apiKey, accessToken string, timezone *time.Location) CloudClient

// defaultCloudClientFactory creates the default production cloud client.
func defaultCloudClientFactory(systemID, systemName, apiKey, accessToken string, tz *time.Location) CloudClient {
	return api.NewEnlightenCloudClient(systemID, apiKey, accessToken, tz).WithSystemName(systemName)
}

// DataAggregator handles combining data from multiple systems.
// Dependencies are injected at construction to support testing.
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

// GetAggregatedMetrics retrieves and combines data from all systems.
// If testDate is provided, uses that date instead of today.
// queryMode specifies the Query Mode (Day, Month, Year, or True-Up).
// reportTimezone is the timezone to use for all systems' data queries (from config, system, or US/Pacific fallback).
func (a *DataAggregator) GetAggregatedMetrics(
	ctx context.Context,
	systems []SystemConfig,
	apiConfig *APIConfig,
	testDate time.Time,
	queryMode constants.QueryMode,
	reportTimezone *time.Location,
) (*AggregatedMetrics, error) {
	metrics := &AggregatedMetrics{
		Timestamp: time.Now(),
		QueryDate: testDate,
		QueryMode: queryMode,
		Systems:   make([]SystemMetrics, 0, len(systems)),
	}
	anyCacheUsed := false
	allFromCache := len(systems) > 0 // optimistic; flipped false if any live call is made
	var rateLimitErrors []string     // system names that hit the rate limit

	for _, sys := range systems {
		if apiConfig == nil {
			return nil, fmt.Errorf("%s for system %s", constants.ErrAPIConfigRequired, sys.Name)
		}
		if apiConfig.Key == "" {
			return nil, fmt.Errorf("api.key required for system %s", sys.Name)
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		accessToken, err := a.getAccessToken(ctx, apiConfig)
		if err != nil {
			return nil, fmt.Errorf("%s for system %s: %w", constants.ErrTokenRefreshFailed, sys.Name, err)
		}

		cloudClient := a.createCloudClient(sys.ID, sys.Name, apiConfig.Key, accessToken, reportTimezone)

		localMetrics, cacheUsed, err := cloudClient.GetMetricsFromCloud(ctx, testDate, queryMode)
		if err != nil && constants.IsRateLimitError(err) {
			rateLimitErrors = append(rateLimitErrors, sys.Name)
			allFromCache = false
			continue
		}
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, fmt.Errorf("failed to get metrics from Cloud API for system %s: %w", sys.Name, err)
			}
			fmt.Fprintf(os.Stderr, "WARNING: [%s] Failed to get metrics, skipping: %v\n", sys.Name, err)
			allFromCache = false
			continue
		}
		if cacheUsed {
			anyCacheUsed = true
		} else {
			allFromCache = false
		}

		netFlowToday := localMetrics.GridImportToday - localMetrics.GridExportToday

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
			NetFlowToday:           netFlowToday,
		}

		metrics.Systems = append(metrics.Systems, systemMetrics)
		metrics.ProductionToday += localMetrics.ProductionToday
		metrics.ConsumptionToday += localMetrics.ConsumptionToday
		metrics.GridImportToday += localMetrics.GridImportToday
		metrics.GridExportToday += localMetrics.GridExportToday
	}

	metrics.NetFlowToday = metrics.GridImportToday - metrics.GridExportToday

	metrics.CacheUsed = anyCacheUsed
	metrics.AllFromCache = allFromCache

	// If we collected any 429 errors that could not be resolved with cache, return error
	if len(rateLimitErrors) > 0 {
		return nil, fmt.Errorf("%s: affected systems: %s", constants.RateLimitError, strings.Join(rateLimitErrors, ", "))
	}

	return metrics, nil
}
