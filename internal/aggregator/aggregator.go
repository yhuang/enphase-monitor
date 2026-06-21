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
	"enphase-monitor/internal/credentials"
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
//
// The credential pool is used two ways: each system is assigned a credential
// round-robin (pool.ForSystem) to spread the per-key rate limit, and when a
// system's credential is rate-limited (429) the call fails over to a spare
// credential (pool.Failover). A system only contributes a rate-limit error once
// every credential has been exhausted for it.
func (a *DataAggregator) GetAggregatedMetrics(
	ctx context.Context,
	systems []SystemConfig,
	pool *credentials.Pool,
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

	for i, sys := range systems {
		if pool == nil || pool.Len() == 0 {
			return nil, fmt.Errorf("%s for system %s", constants.ErrAPIConfigRequired, sys.Name)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Assign a credential (spread) and retry across spares on 429 (failover).
		cred := pool.ForSystem(i)
		tried := make(map[string]bool, pool.Len())
		var localMetrics *api.LocalMetrics
		var cacheUsed bool
		rateLimited := false

		for {
			tried[cred.Name] = true

			if cred.Key == "" {
				return nil, fmt.Errorf("api.key required for system %s", sys.Name)
			}

			accessToken, err := a.getAccessToken(ctx, cred)
			if err != nil {
				// Token acquisition failed for this credential (e.g. an expired/
				// revoked refresh token or an Enphase 5xx). Context errors are not
				// credential-specific, so they stay fatal; otherwise cool this
				// credential down and fail over to a spare, only erroring once every
				// credential has been exhausted.
				if isContextError(ctx, err) {
					return nil, fmt.Errorf("%s for system %s: %w", constants.ErrTokenRefreshFailed, sys.Name, err)
				}
				pool.MarkUnavailable(cred)
				if next, ok := pool.Failover(tried); ok {
					fmt.Fprintf(os.Stderr, "WARNING: [%s] token request failed for credential %q, trying %q: %v\n", sys.Name, cred.Name, next.Name, err)
					cred = next
					continue
				}
				return nil, fmt.Errorf("%s for system %s: %w", constants.ErrTokenRefreshFailed, sys.Name, err)
			}

			cloudClient := a.createCloudClient(sys.ID, sys.Name, cred.Key, accessToken, reportTimezone)

			lm, cu, err := cloudClient.GetMetricsFromCloud(ctx, testDate, queryMode)
			if err != nil && constants.IsRateLimitError(err) {
				// This credential is throttled; cool it down and fail over to a spare.
				pool.MarkUnavailable(cred)
				if next, ok := pool.Failover(tried); ok {
					cred = next
					continue
				}
				rateLimited = true
				break
			}
			if err != nil {
				if isContextError(ctx, err) {
					return nil, fmt.Errorf("failed to get metrics from Cloud API for system %s: %w", sys.Name, err)
				}
				fmt.Fprintf(os.Stderr, "WARNING: [%s] Failed to get metrics, skipping: %v\n", sys.Name, err)
				allFromCache = false
				break // localMetrics stays nil → skipped below
			}
			localMetrics = lm
			cacheUsed = cu
			break
		}

		if rateLimited {
			rateLimitErrors = append(rateLimitErrors, sys.Name)
			allFromCache = false
			continue
		}
		if localMetrics == nil {
			continue // non-429 failure already warned/skipped
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

// isContextError reports whether err is (or wraps) a context cancellation or
// deadline, or whether ctx itself is done. Such errors are not credential- or
// system-specific, so callers treat them as fatal rather than failing over.
func isContextError(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}
