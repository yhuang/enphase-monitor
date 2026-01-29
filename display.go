// Package display handles formatting and presentation of metrics to the user.
//
// PURPOSE
// -------
// This file provides the Display struct and methods for formatting energy metrics
// into a readable terminal report with color customization.
//
// OUTPUT STRUCTURE
// ----------------
// The display output consists of three main sections:
//
//  1. Header Section:
//     - Application title
//     - Query date range (for historical queries) or "Today" indicator
//     - Last updated timestamp (with cache indicator if applicable)
//
//  2. Combined Energy Report:
//     - Aggregated totals across all systems
//     - Production, consumption, net energy flow
//     - Battery charge/discharge totals
//
//  3. Individual Systems Report:
//     - Per-system breakdown with all metrics
//     - Grid import/export, production, consumption
//     - Battery charge/discharge and SOC
//     - Net energy flow per system
//
// COLOR CUSTOMIZATION
// -------------------
// All colors are customizable via config.yaml. The Display struct stores
// a ColorConfig that contains ANSI escape codes for each metric type.
// Colors can be specified as:
//   - Hex codes: "#FF5733" (automatically converted to ANSI)
//   - ANSI codes: "\033[38;5;208m" (used directly)
//
// Default colors are provided in getDefaultColors() if not specified in config.
//
// FORMATTING DETAILS
// ------------------
// - All numeric values are right-aligned for readability
// - Time formats use 12-hour format with AM/PM (fixed-width with leading zeros)
// - Battery State of Charge (SOC) displayed as percentage with right alignment (per-system only)
// - Cache status indicated in timestamp line when cached data is used
package main

import (
	"fmt"
	"strings"
	"time"
)

// getDefaultColors returns the default color configuration
// These defaults match the values in config.yaml
// Note: Reset and Bold are constants defined in constants.go
func getDefaultColors() ColorConfig {
	return ColorConfig{
		Production:    "\033[38;5;208m", // Solar production (Enphase orange)
		Discharge:     "\033[38;5;34m",  // Battery discharge, positive metrics
		Import:        "\033[38;5;39m",  // Consumption, grid import
		Export:        "\033[38;5;220m", // Grid export
		NetImport:     "\033[38;5;39m",  // Net energy flow (import) - defaults to same as Import
		NetExport:     "\033[38;5;220m", // Net energy flow (export) - defaults to same as Export
		Headers:       "\033[38;5;51m",  // Headers, separators
		Charge:        "\033[38;5;205m", // Battery charge
		TotalConsumed: "\033[38;5;39m",  // Total consumed (defaults to same as Import)
		SecondaryText: "\033[38;5;245m", // Secondary info
		PrimaryText:   "\033[38;5;255m", // Primary text
		Error:         "\033[38;5;196m", // Error messages
	}
}

// Display handles formatting and presenting metrics to the user
type Display struct {
	colors   ColorConfig
	timezone *time.Location // Timezone for reporting/display
}

// NewDisplayWithColorsAndTimezone creates a new display instance with custom colors and timezone
func NewDisplayWithColorsAndTimezone(colors ColorConfig, tz *time.Location) *Display {
	defaultColors := getDefaultColors()
	// Use defaults for any colors not specified
	// Note: Reset and Bold are constants defined in constants.go
	colors.mergeWithDefaults(defaultColors)
	return &Display{
		colors:   colors,
		timezone: tz,
	}
}

// ShowMetrics displays the aggregated metrics in a formatted output
func (d *Display) ShowMetrics(metrics *AggregatedMetrics) {
	d.printHeader(metrics.Timestamp, metrics.CacheUsed, metrics.QueryDate)
	d.printTodayEnergy(metrics)
	d.printIndividualSystems(metrics)
	d.printSeparator()
}

func (d *Display) printHeader(timestamp time.Time, cacheUsed bool, queryDate time.Time) {
	fmt.Println("\n" + d.colors.Headers + strings.Repeat("=", 57) + Reset)
	fmt.Printf("  %s%sENPHASE MULTI-SYSTEM MONITOR%s\n", Bold, d.colors.Headers, Reset)
	fmt.Println(d.colors.Headers + strings.Repeat("=", 57) + Reset)

	// Show query range
	// Use the configured timezone for display
	nowLocal := time.Now().In(d.timezone)

	if !queryDate.IsZero() {
		// For a specific query date, show the date range in the configured timezone
		queryDayLocal := time.Date(queryDate.Year(), queryDate.Month(), queryDate.Day(), 0, 0, 0, 0, d.timezone)
		todayLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, d.timezone)

		var dayEndLocal time.Time
		if queryDayLocal.Equal(todayLocal) {
			// For today, show current time as end
			dayEndLocal = nowLocal
		} else {
			// For past dates, show end of day
			dayEndLocal = time.Date(queryDate.Year(), queryDate.Month(), queryDate.Day(), 23, 59, 59, 0, d.timezone)
		}

		dayStartLocal := time.Date(queryDate.Year(), queryDate.Month(), queryDate.Day(), 0, 0, 0, 0, d.timezone)

		fmt.Printf("  %s%-16s%s%s 12:00 AM\n                          to\n                  %s%s\n\n",
			d.colors.SecondaryText, "Query Range:   ", d.colors.PrimaryText,
			dayStartLocal.Format("Mon Jan 2, 2006"),
			dayEndLocal.Format("Mon Jan 2, 2006 03:04 PM"), Reset)
	} else {
		// Default to today in the configured timezone
		todayStartLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, d.timezone)

		fmt.Printf("  %s%-16s%s%s 12:00 AM\n                          to\n                  %s%s\n\n",
			d.colors.SecondaryText, "Query Range:   ", d.colors.PrimaryText,
			todayStartLocal.Format("Mon Jan 2, 2006"),
			nowLocal.Format("Mon Jan 2, 2006 03:04 PM"), Reset)
	}

	// Convert timestamp to the configured timezone for display
	timestampLocal := timestamp.In(d.timezone)
	fmt.Printf("  %s%-16s%s%s", d.colors.SecondaryText, "Last Updated:   ", d.colors.PrimaryText, timestampLocal.Format("Mon Jan 2, 2006 03:04:05 PM"))
	if cacheUsed {
		fmt.Printf(" %s(cached)%s", d.colors.SecondaryText, Reset)
	} else {
		fmt.Printf(" %s(live)%s", d.colors.Discharge, Reset)
	}
	fmt.Println()
	fmt.Println(d.colors.Headers + strings.Repeat("=", 57) + Reset)
}

func (d *Display) printTodayEnergy(metrics *AggregatedMetrics) {
	fmt.Printf("\n %s%sCOMBINED ENERGY REPORT (kWh)%s\n", Bold, d.colors.PrimaryText, Reset)
	fmt.Println(d.colors.SecondaryText + strings.Repeat("-", 57) + Reset)

	fmt.Printf("  %sProduced:%s               %s%8.1f kWh%s\n", d.colors.PrimaryText, Reset, d.colors.Production, metrics.ProductionToday, Reset)
	fmt.Printf("  %sConsumed:%s               %s%8.1f kWh%s\n", d.colors.PrimaryText, Reset, d.colors.TotalConsumed, metrics.ConsumptionToday, Reset)

	// Show net energy flow (sum of net import across all systems)
	netEnergyFlow := metrics.GridImportToday - metrics.GridExportToday
	if netEnergyFlow < 0 {
		fmt.Printf("  %sNet Energy Flow:%s        %s%8.1f kWh%s %s(export)%s\n",
			d.colors.PrimaryText, Reset, d.colors.NetExport, -netEnergyFlow, Reset,
			d.colors.NetExport, Reset)
	} else {
		fmt.Printf("  %sNet Energy Flow:%s        %s%8.1f kWh%s %s(import)%s\n",
			d.colors.PrimaryText, Reset, d.colors.NetImport, netEnergyFlow, Reset,
			d.colors.NetImport, Reset)
	}

}

func (d *Display) printIndividualSystems(metrics *AggregatedMetrics) {
	if len(metrics.Systems) <= 1 {
		return
	}

	fmt.Printf("\n %s%sINDIVIDUAL SYSTEMS REPORT%s\n", Bold, d.colors.PrimaryText, Reset)
	fmt.Println(d.colors.SecondaryText + strings.Repeat("-", 57) + Reset)

	for i, sys := range metrics.Systems {
		displayName := sys.Name
		// Display System ID
		identifier := sys.ID
		fmt.Printf("\n  %s[%d]%s %s%s%s %s(%s)%s\n",
			d.colors.Headers, i+1, Reset,
			Bold, displayName, Reset,
			d.colors.SecondaryText, identifier, Reset)
		fmt.Printf("      %sImported from the Grid:%s     %s%8.1f kWh%s\n",
			d.colors.SecondaryText, Reset, d.colors.Import, sys.GridImportToday, Reset)
		fmt.Printf("      %sExported to the Grid:%s       %s%8.1f kWh%s\n",
			d.colors.SecondaryText, Reset, d.colors.Export, sys.GridExportToday, Reset)
		fmt.Printf("      %sCaptured from the Sun:%s      %s%8.1f kWh%s\n",
			d.colors.SecondaryText, Reset, d.colors.Production, sys.ProductionToday, Reset)
		if sys.NetImportedToday < 0 {
			fmt.Printf("      %sNet Energy Flow:%s            %s%8.1f kWh%s %s(export)%s\n",
				d.colors.SecondaryText, Reset, d.colors.NetExport, -sys.NetImportedToday, Reset,
				d.colors.NetExport, Reset)
		} else {
			fmt.Printf("      %sNet Energy Flow:%s            %s%8.1f kWh%s %s(import)%s\n",
				d.colors.SecondaryText, Reset, d.colors.NetImport, sys.NetImportedToday, Reset,
				d.colors.NetImport, Reset)
		}
		fmt.Printf("      %sCharged to Battery:%s         %s%8.1f kWh%s\n",
			d.colors.SecondaryText, Reset, d.colors.Charge, sys.BatteryChargedToday, Reset)
		fmt.Printf("      %sDischarged from Battery:%s    %s%8.1f kWh%s\n",
			d.colors.SecondaryText, Reset, d.colors.Discharge, sys.BatteryDischargedToday, Reset)
		fmt.Printf("      %sBattery Charge Percentage:%s      %s%7d%%%s\n",
			d.colors.SecondaryText, Reset, d.colors.Charge, sys.BatterySOC, Reset)
		fmt.Printf("      %sTotal Consumed:%s             %s%8.1f kWh%s\n",
			d.colors.SecondaryText, Reset, d.colors.TotalConsumed, sys.ConsumptionToday, Reset)
	}
}

func (d *Display) printSeparator() {
	fmt.Println("\n" + d.colors.Headers + strings.Repeat("=", 57) + Reset + "\n")
}

// ShowError displays an error message
func (d *Display) ShowError(err error) {
	fmt.Printf("\n%s%sERROR:%s\n", Bold, d.colors.Error, Reset)
	fmt.Printf("   %s%v%s\n\n", d.colors.Error, err, Reset)
}

// ShowInfo displays an informational message
func (d *Display) ShowInfo(message string) {
	fmt.Printf("\n%s%s%s\n", d.colors.SecondaryText, message, Reset)
}
