// Package aggregator - aggregator.go
//
// This file implements the core aggregation logic for combining metrics from
// multiple Enphase systems. For detailed metric derivation and calculation
// documentation, see the original aggregator.go file in the main package.
package aggregator

import (
	"context"
	"fmt"
	"os"
	"time"

	"enphase-monitor/internal/api"
	"enphase-monitor/internal/constants"
)

// SystemConfig represents configuration for a single Enphase system.
// This is a mirror of the main package's SystemConfig to avoid circular dependencies.
type SystemConfig struct {
	Name string
	ID   string
}

// APIConfig represents API configuration.
// This is a mirror of the main package's APIConfig to avoid circular dependencies.
type APIConfig struct {
	Key              string
	ClientID         string
	ClientSecret     string
	AuthorizationURL string
	RedirectURI      string
	RefreshToken     string
	Username         string
	Password         string
}

// OAuthTokenGetter is a function type for getting OAuth access tokens.
// This allows the aggregator to work with any OAuth implementation.
type OAuthTokenGetter func(ctx context.Context, apiConfig *APIConfig) (string, error)

// DataAggregator handles combining data from multiple systems
//
// GO PATTERN: Stateless Service Object (empty struct with methods)
// The empty struct stores no data, but this pattern provides:
//   - Method grouping: organizes related functions under one namespace
//   - Interface readiness: can implement interfaces for mocking/testing
//   - Future extensibility: fields can be added later without changing call sites
//
// Alternative: plain package-level functions (simpler but harder to mock/extend)
type DataAggregator struct {
	getAccessToken OAuthTokenGetter
}

// NewDataAggregator creates a new aggregator instance with the specified OAuth token getter
func NewDataAggregator(getAccessToken OAuthTokenGetter) *DataAggregator {
	return &DataAggregator{
		getAccessToken: getAccessToken,
	}
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
		cloudClient := api.NewEnlightenCloudClient(sys.ID, apiConfig.Key, accessToken, reportTimezone)

		var cacheUsed bool
		localMetrics, cacheUsed, err := cloudClient.GetMetricsFromCloud(ctx, testDate)
		if err != nil {
			if constants.IsRateLimitError(err) {
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

	// Calculate net import (positive = net import, negative = net export)
	metrics.NetImportToday = metrics.GridImportToday - metrics.GridExportToday

	metrics.CacheUsed = anyCacheUsed

	// If we collected any 429 errors that could not be resolved with cache, print them once and exit
	if len(rateLimitErrors) > 0 {
		// Enphase API rate limit window is 60 seconds (10 requests/minute for free tier)
		fmt.Fprintf(os.Stderr, "ERROR: API rate limit exceeded (429)\n")
		fmt.Fprintf(os.Stderr, "Please wait %d seconds before rerunning the program.\n", constants.APIRateLimitWaitSeconds)
		return nil, fmt.Errorf("rate limit exceeded (429): %d system(s) affected", len(rateLimitErrors))
	}

	return metrics, nil
}
