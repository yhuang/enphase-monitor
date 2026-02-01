// Package app - runner.go
//
// PURPOSE
// -------
// This file contains the application execution logic for different run modes.
// Handles single execution (--once), continuous monitoring, and metric fetching/display.
//
// EXECUTION MODES
// ---------------
// The application supports two primary execution modes:
//   - RunOnce: Single query and exit (--once flag)
//   - RunContinuous: Continuous monitoring with refresh interval
//
// Both modes use fetchAndDisplay for the actual metric retrieval and display logic.
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

// RunOnce executes a single query, displays results, and exits
func RunOnce(ctx context.Context, agg *aggregator.DataAggregator, disp *display.Display, cfg *config.Config, testDate time.Time, testMode bool, reportTZ *time.Location) {
	aggSystems, aggAPIConfig := GetAggregatorTypes(cfg)

	metrics, err := agg.GetAggregatedMetrics(ctx, aggSystems, aggAPIConfig, testDate, reportTZ)
	if err != nil {
		// Check if it is a 429 rate limit error - if so, the error message already contains wait time info
		if constants.IsRateLimitError(err) {
			// Error message already printed in aggregator.go, just exit
			os.Exit(1)
		}
		disp.ShowError(err)
		os.Exit(1)
	}

	disp.ShowMetrics(metrics)

	// If in test mode and test date is provided, validate against expected values
	if testMode {
		if testDate.IsZero() {
			fmt.Fprintf(os.Stderr, "ERROR: --test flag requires --date flag to specify which date to validate\n")
			os.Exit(1)
		}
		testDateStr := testDate.Format(constants.DateFormat)
		if err := validation.ValidateMetrics(metrics, testDateStr); err != nil {
			fmt.Fprintf(os.Stderr, "\nValidation failed: %v\n", err)
			os.Exit(1)
		}
	}
}

// RunContinuous executes continuous monitoring with periodic refresh
func RunContinuous(ctx context.Context, agg *aggregator.DataAggregator, disp *display.Display, cfg *config.Config, testDate time.Time, reportTZ *time.Location) {
	disp.ShowInfo(fmt.Sprintf("Starting continuous monitoring (refresh every %d seconds)", cfg.RefreshInterval))
	disp.ShowInfo("Press Ctrl+C to stop")

	ticker := time.NewTicker(time.Duration(cfg.RefreshInterval) * time.Second)
	defer ticker.Stop()

	// Run immediately on start
	fetchAndDisplay(ctx, agg, disp, cfg, testDate, reportTZ)

	for {
		select {
		case <-ticker.C:
			fetchAndDisplay(ctx, agg, disp, cfg, testDate, reportTZ)

		case <-ctx.Done():
			// Context is cancelled when SIGINT (Ctrl+C) or SIGTERM is received.
			// In-flight HTTP requests are also cancelled via the shared context.
			disp.ShowInfo("Shutting down gracefully...")
			return
		}
	}
}

// fetchAndDisplay fetches metrics and displays them to the terminal
func fetchAndDisplay(ctx context.Context, agg *aggregator.DataAggregator, disp *display.Display, cfg *config.Config, testDate time.Time, reportTZ *time.Location) {
	aggSystems, aggAPIConfig := GetAggregatorTypes(cfg)

	metrics, err := agg.GetAggregatedMetrics(ctx, aggSystems, aggAPIConfig, testDate, reportTZ)
	if err != nil {
		// If context was cancelled (shutdown in progress), exit silently
		if ctx.Err() != nil {
			return
		}
		// Check if it's a 429 rate limit error - if so, exit instead of continuing
		if constants.IsRateLimitError(err) {
			// Error message already printed in aggregator.go, just exit
			os.Exit(1)
		}
		disp.ShowError(err)
		return
	}

	// Clear screen for cleaner output
	fmt.Print("\033[H\033[2J")

	disp.ShowMetrics(metrics)
}
