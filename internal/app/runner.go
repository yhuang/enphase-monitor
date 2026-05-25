// Package app provides application setup and execution modes (once or continuous) for the enphase-monitor application.
//
// EXECUTION MODES
// ---------------
// The application supports two primary execution modes:
//   - RunOnce: Single query and exit (default behavior)
//   - RunContinuous: Continuous monitoring with refresh interval
//
// Both modes use fetchAndDisplay for the actual metric retrieval and display logic.
// Setup functions (CreateOAuthAdapter, SetupDisplay, ConfigureModes, ParseTestDate, etc.) live in setup.go.
package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/display"
	"enphase-monitor/internal/validation"
)

// RunConfig groups the parameters shared across RunOnce, RunContinuous, and fetchAndDisplay.
type RunConfig struct {
	Agg       *aggregator.DataAggregator
	Disp      *display.Display
	Cfg       *config.Config
	TestDate  time.Time
	QueryMode constants.QueryMode
	ReportTZ  *time.Location
	Debug     bool
}

// RunOnce executes a single query, displays results, and returns an error on failure.
// The caller (main) is responsible for exiting with a non-zero code.
func RunOnce(ctx context.Context, rc RunConfig, validationMode bool) error {
	aggSystems, aggAPIConfig := GetAggregatorTypes(rc.Cfg)

	metrics, err := rc.Agg.GetAggregatedMetrics(ctx, aggSystems, aggAPIConfig, rc.TestDate, rc.QueryMode, rc.ReportTZ)
	if err != nil {
		return err
	}

	rc.Disp.ShowMetrics(metrics)

	// If in Validation Mode and test date is provided, validate against expected values
	if validationMode {
		if rc.TestDate.IsZero() {
			return fmt.Errorf("--test flag requires --date flag to specify which date to validate")
		}
		testDateStr := FormatDateForQueryMode(rc.TestDate, rc.QueryMode)
		if err := validation.ValidateMetrics(os.Stdout, metrics, testDateStr); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}
	return nil
}

// RunContinuous executes continuous monitoring with periodic refresh.
// It returns an error only on fatal failures (e.g. 429); normal shutdown returns nil.
func RunContinuous(ctx context.Context, rc RunConfig) error {
	rc.Disp.ShowInfo(fmt.Sprintf("Starting continuous monitoring (refresh every %d seconds)", rc.Cfg.RefreshIntervalSeconds))
	rc.Disp.ShowInfo("Press Ctrl+C to stop")

	ticker := time.NewTicker(time.Duration(rc.Cfg.RefreshIntervalSeconds) * time.Second)
	defer ticker.Stop()

	// Run immediately on start
	if err := fetchAndDisplay(ctx, rc); err != nil {
		return err
	}

	for {
		select {
		case <-ticker.C:
			if err := fetchAndDisplay(ctx, rc); err != nil {
				return err
			}

		case <-ctx.Done():
			rc.Disp.ShowInfo("Shutting down gracefully...")
			return nil
		}
	}
}

// fetchAndDisplay fetches metrics and displays them to the terminal.
// Returns a non-nil error only on fatal failures (e.g. 429); caller may exit.
// On non-fatal errors it shows the error and returns nil so the loop can continue.
func fetchAndDisplay(ctx context.Context, rc RunConfig) error {
	aggSystems, aggAPIConfig := GetAggregatorTypes(rc.Cfg)

	metrics, err := rc.Agg.GetAggregatedMetrics(ctx, aggSystems, aggAPIConfig, rc.TestDate, rc.QueryMode, rc.ReportTZ)
	if err != nil {
		// If context was cancelled (shutdown in progress), exit silently
		if ctx.Err() != nil {
			return nil
		}
		// 429 is fatal: return so caller can exit
		if constants.IsRateLimitError(err) {
			return err
		}
		rc.Disp.ShowError(err)
		return nil
	}

	// Clear screen for cleaner output (skipped in debug mode to preserve debug output)
	if !rc.Debug {
		rc.Disp.ClearScreen()
	}

	rc.Disp.ShowMetrics(metrics)
	return nil
}
