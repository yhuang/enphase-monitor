// Package app provides application setup and execution modes (once or continuous) for the enphase-monitor application.
//
// EXECUTION MODES
// ---------------
// The application supports two primary execution modes:
//   - RunOnce: Single query and exit (--once flag)
//   - RunContinuous: Continuous monitoring with refresh interval
//
// Both modes use fetchAndDisplay for the actual metric retrieval and display logic.
// Setup functions (CreateOAuthAdapter, SetupDisplay, ConfigureModes, ParseTestDate, etc.) live in setup.go.
package app

import (
	"context"
	"fmt"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/display"
	"enphase-monitor/internal/validation"
)

// RunOnce executes a single query, displays results, and returns an error on failure.
// The caller (main) is responsible for exiting with a non-zero code.
func RunOnce(ctx context.Context, agg *aggregator.DataAggregator, disp *display.Display, cfg *config.Config, testDate time.Time, testMode bool, reportTZ *time.Location) error {
	aggSystems, aggAPIConfig := GetAggregatorTypes(cfg)

	metrics, err := agg.GetAggregatedMetrics(ctx, aggSystems, aggAPIConfig, testDate, reportTZ)
	if err != nil {
		return err
	}

	disp.ShowMetrics(metrics)

	// If in test mode and test date is provided, validate against expected values
	if testMode {
		if testDate.IsZero() {
			return fmt.Errorf("--test flag requires --date flag to specify which date to validate")
		}
		testDateStr := testDate.Format(constants.DateFormat)
		if err := validation.ValidateMetrics(metrics, testDateStr); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}
	return nil
}

// RunContinuous executes continuous monitoring with periodic refresh.
// It returns an error only on fatal failures (e.g. rate limit); normal shutdown returns nil.
func RunContinuous(ctx context.Context, agg *aggregator.DataAggregator, disp *display.Display, cfg *config.Config, testDate time.Time, reportTZ *time.Location) error {
	disp.ShowInfo(fmt.Sprintf("Starting continuous monitoring (refresh every %d seconds)", cfg.RefreshIntervalSeconds))
	disp.ShowInfo("Press Ctrl+C to stop")

	ticker := time.NewTicker(time.Duration(cfg.RefreshIntervalSeconds) * time.Second)
	defer ticker.Stop()

	// Run immediately on start
	if err := fetchAndDisplay(ctx, agg, disp, cfg, testDate, reportTZ); err != nil {
		return err
	}

	for {
		select {
		case <-ticker.C:
			if err := fetchAndDisplay(ctx, agg, disp, cfg, testDate, reportTZ); err != nil {
				return err
			}

		case <-ctx.Done():
			// Context is cancelled when SIGINT (Ctrl+C) or SIGTERM is received.
			// In-flight HTTP requests are also cancelled via the shared context.
			disp.ShowInfo("Shutting down gracefully...")
			return nil
		}
	}
}

// fetchAndDisplay fetches metrics and displays them to the terminal.
// Returns a non-nil error only on fatal failures (e.g. rate limit); caller may exit.
// On non-fatal errors it shows the error and returns nil so the loop can continue.
func fetchAndDisplay(ctx context.Context, agg *aggregator.DataAggregator, disp *display.Display, cfg *config.Config, testDate time.Time, reportTZ *time.Location) error {
	aggSystems, aggAPIConfig := GetAggregatorTypes(cfg)

	metrics, err := agg.GetAggregatedMetrics(ctx, aggSystems, aggAPIConfig, testDate, reportTZ)
	if err != nil {
		// If context was cancelled (shutdown in progress), exit silently
		if ctx.Err() != nil {
			return nil
		}
		// Rate limit is fatal: return so caller can exit
		if constants.IsRateLimitError(err) {
			return err
		}
		disp.ShowError(err)
		return nil
	}

	// Clear screen for cleaner output
	fmt.Print("\033[H\033[2J")

	disp.ShowMetrics(metrics)
	return nil
}
