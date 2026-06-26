// Package app provides application setup and execution for the enphase-monitor application.
//
// EXECUTION MODES
// ---------------
// The application supports a single execution mode:
//   - RunOnce: Single query and exit (default behavior)
//
// Setup functions (CreateOAuthAdapter, SetupDisplay, ConfigureModes, ParseTestDate, etc.) live in setup.go.
package app

import (
	"context"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/credentials"
	"enphase-monitor/internal/display"
)

// RunConfig groups the parameters for RunOnce and related helpers.
type RunConfig struct {
	Agg       *aggregator.DataAggregator
	Pool      *credentials.Pool
	Disp      *display.Display
	Cfg       *config.Config
	TestDate  time.Time
	QueryMode constants.QueryMode
	ReportTZ  *time.Location
	Debug     bool

	// Location and Weather drive best-effort weather enrichment for Day-Mode
	// reports. Both nil (e.g. true-up path) disables it. See weather.go.
	Location CoordinateProvider
	Weather  WeatherProvider
}

// RunOnce executes a single query, displays results, and returns an error on failure.
// The caller (main) is responsible for exiting with a non-zero code.
func RunOnce(ctx context.Context, rc RunConfig) error {
	metrics, err := rc.Agg.GetAggregatedMetrics(ctx, GetSystems(rc.Cfg), rc.Pool, rc.TestDate, rc.QueryMode, rc.ReportTZ)
	if err != nil {
		return err
	}

	enrichWithTemperature(ctx, rc, metrics)
	rc.Disp.ShowMetrics(metrics)
	return nil
}
