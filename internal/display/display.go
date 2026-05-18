// Package display provides terminal output formatting and presentation of
// aggregated energy metrics with customizable colors and timezone-aware dates.
//
// It uses dependency injection (io.Writer) so production code writes to
// os.Stdout and tests can capture output with a bytes.Buffer.
package display

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/timezone"
)

// GetDefaultColors returns the default color configuration.
func GetDefaultColors() config.ColorConfig {
	return config.ColorConfig{
		Production:    "\033[38;5;208m",
		Discharge:     "\033[38;5;34m",
		Import:        "\033[38;5;39m",
		Export:        "\033[38;5;220m",
		NetImport:     "\033[38;5;39m",
		NetExport:     "\033[38;5;220m",
		Headers:       "\033[38;5;51m",
		Charge:        "\033[38;5;205m",
		TotalConsumed: "\033[38;5;39m",
		SecondaryText: "\033[38;5;245m",
		PrimaryText:   "\033[38;5;255m",
		Error:         "\033[38;5;196m",
	}
}

// Display handles formatting and presenting metrics to the user.
//
// GO PATTERN: Dependency Injection via io.Writer
// By accepting an io.Writer, the Display struct becomes testable:
//   - Production: pass os.Stdout for terminal output
//   - Testing: pass bytes.Buffer to capture and verify output
type Display struct {
	colors        config.ColorConfig
	timezone      *time.Location
	separatorLine string
	subSeparator  string
	writer        io.Writer // Output destination (os.Stdout for production, bytes.Buffer for testing)
}

// NewDisplayWithColorsAndTimezone creates a new display instance with custom colors and timezone.
// Uses os.Stdout as the default output writer for production use.
func NewDisplayWithColorsAndTimezone(colors config.ColorConfig, tz *time.Location) *Display {
	return NewDisplayWithWriter(colors, tz, os.Stdout)
}

// NewDisplayWithWriter creates a display instance with a custom writer.
// This is primarily used for testing to capture output.
func NewDisplayWithWriter(colors config.ColorConfig, tz *time.Location, w io.Writer) *Display {
	defaultColors := GetDefaultColors()
	colors.MergeWithDefaults(defaultColors)

	return &Display{
		colors:        colors,
		timezone:      tz,
		separatorLine: strings.Repeat("=", constants.SeparatorWidth),
		subSeparator:  strings.Repeat("-", constants.SeparatorWidth),
		writer:        w,
	}
}

// ShowMetrics displays the aggregated metrics in a formatted output.
func (d *Display) ShowMetrics(metrics *aggregator.AggregatedMetrics) {
	d.printHeader(metrics.Timestamp, metrics.CacheUsed, metrics.QueryDate, metrics.QueryType)
	d.printTodayEnergy(metrics)
	d.printIndividualSystems(metrics)
	d.printSeparator()
}

func (d *Display) printHeader(timestamp time.Time, cacheUsed bool, queryDate time.Time, queryType constants.QueryType) {
	fmt.Fprintln(d.writer, "\n  "+d.colors.Headers+d.separatorLine+constants.Reset)
	fmt.Fprintf(d.writer, "    %s%sENPHASE MULTI-SYSTEM MONITOR%s\n", constants.Bold, d.colors.Headers, constants.Reset)
	fmt.Fprintln(d.writer, "  "+d.colors.Headers+d.separatorLine+constants.Reset)

	// Calculate date range based on query type (day/month/year).
	// For ongoing month/year periods, cap the display end to yesterday (last complete day)
	// to match the lifetime endpoint data coverage — today's partial day is not included.
	periodStart, periodEnd := timezone.GetBoundaries(queryDate, queryType, d.timezone)
	if (queryType == constants.QueryTypeMonth || queryType == constants.QueryTypeYear) &&
		!timezone.IsPastPeriod(queryDate, queryType, d.timezone) {
		yesterday := time.Now().In(d.timezone).AddDate(0, 0, -1)
		periodEnd = time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 0, d.timezone)
	}

	fmt.Fprintf(d.writer, "     %s%-11s%s%s 12:00 AM\n                            to\n                    %s%s\n\n",
		d.colors.SecondaryText, "Query Range:   ", d.colors.PrimaryText,
		periodStart.Format("Mon Jan 2, 2006"),
		periodEnd.Format("Mon Jan 2, 2006 03:04 PM"), constants.Reset)

	timestampLocal := timestamp.In(d.timezone)
	fmt.Fprintf(d.writer, "    %s%-12s%s%s", d.colors.SecondaryText, "Last Updated:   ", d.colors.PrimaryText, timestampLocal.Format("Mon Jan 2, 2006 03:04:05 PM"))
	sourceLabel := d.colors.Discharge + "(live)" + constants.Reset
	if cacheUsed {
		sourceLabel = d.colors.SecondaryText + "(cached)" + constants.Reset
	}
	fmt.Fprintf(d.writer, " %s", sourceLabel)
	fmt.Fprintln(d.writer)
	fmt.Fprintln(d.writer, "  "+d.colors.Headers+d.separatorLine+constants.Reset)
}

func (d *Display) printTodayEnergy(metrics *aggregator.AggregatedMetrics) {
	fmt.Fprintf(d.writer, "\n   %s%sCOMBINED ENERGY REPORT%s\n", constants.Bold, d.colors.PrimaryText, constants.Reset)
	fmt.Fprintln(d.writer, "  "+d.colors.SecondaryText+d.subSeparator+constants.Reset)

	d.printNetFlow("  Net Energy Flow", metrics.NetImportToday, "  ", 24, false, true)
	fmt.Fprintln(d.writer)
	d.printMetric("Energy Produced", metrics.ProductionToday, d.colors.Production, "    ", 22, false)
	d.printMetric("Energy Consumed", metrics.ConsumptionToday, d.colors.TotalConsumed, "    ", 22, false)
	d.printMetric("Energy Imported", metrics.GridImportToday, d.colors.Import, "    ", 22, false)
	d.printMetric("Energy Exported", metrics.GridExportToday, d.colors.Export, "    ", 22, false)
}

func (d *Display) printIndividualSystems(metrics *aggregator.AggregatedMetrics) {
	if len(metrics.Systems) <= 1 {
		return
	}

	fmt.Fprintf(d.writer, "\n\n   %s%sINDIVIDUAL SYSTEMS REPORT%s\n", constants.Bold, d.colors.PrimaryText, constants.Reset)
	fmt.Fprintln(d.writer, "  "+d.colors.SecondaryText+d.subSeparator+constants.Reset)

	for i, sys := range metrics.Systems {
		displayName := sys.Name
		identifier := sys.ID
		prefix := "\n"
		if i == 0 {
			prefix = ""
		}
		fmt.Fprintf(d.writer, "%s    %s[%d]%s %s%s%s %s(%s)%s\n",
			prefix, d.colors.Headers, i+1, constants.Reset,
			constants.Bold, displayName, constants.Reset,
			d.colors.SecondaryText, identifier, constants.Reset)
		d.printNetFlow("Net Energy Flow", sys.NetImportedToday, "        ", 27, true, false)
		d.printMetric("Energy Produced", sys.ProductionToday, d.colors.Production, "        ", 27, true)
		d.printMetric("Energy Consumed", sys.ConsumptionToday, d.colors.TotalConsumed, "        ", 27, true)
		d.printMetric("Energy Imported", sys.GridImportToday, d.colors.Import, "        ", 27, true)
		d.printMetric("Energy Exported", sys.GridExportToday, d.colors.Export, "        ", 27, true)
		// Battery metrics are only relevant for single-day reports
		if metrics.QueryType == constants.QueryTypeDay {
			d.printMetric("Charged to Battery", sys.BatteryChargedToday, d.colors.Charge, "        ", 27, true)
			d.printMetric("Discharged from Battery", sys.BatteryDischargedToday, d.colors.Discharge, "        ", 27, true)
			fmt.Fprintf(d.writer, "        %sBattery Charge Percentage:%s %s%10d%%%s\n",
				d.colors.SecondaryText, constants.Reset, d.colors.Charge, sys.BatterySOC, constants.Reset)
		}
	}
}

func (d *Display) printSeparator() {
	fmt.Fprintln(d.writer, "\n  "+d.colors.Headers+d.separatorLine+constants.Reset+"\n")
}

func (d *Display) printMetric(label string, value float64, valueColor string, indent string, labelWidth int, rightAlign bool) {
	labelWithColon := label + ":"
	padding := ""
	if labelWidth > 0 && len(labelWithColon) < labelWidth {
		padding = strings.Repeat(" ", labelWidth-len(labelWithColon))
	}

	// Use MWh for values >= 1000 kWh
	displayValue := value
	unit := "kWh"
	if value >= constants.KWhToMWh {
		displayValue = value / constants.KWhToMWh
		unit = "MWh"
	}

	valueFormat := "%.1f"
	if rightAlign {
		valueFormat = "%7.1f"
	}
	fmt.Fprintf(d.writer, "%s%s%s%s%s%s"+valueFormat+" %s%s\n",
		indent, d.colors.SecondaryText, labelWithColon, constants.Reset,
		padding, valueColor, displayValue, unit, constants.Reset)
}

func (d *Display) printNetFlow(label string, netValue float64, indent string, labelWidth int, rightAlign bool, highlight bool) {
	// Default to import (positive), override for export (negative)
	color := d.colors.NetImport
	direction := "import"
	displayValue := netValue

	if netValue < 0 {
		color = d.colors.NetExport
		direction = "export"
		displayValue = -netValue
	}

	labelWithColon := label + ":"
	padding := ""
	if labelWidth > 0 && len(labelWithColon) < labelWidth {
		padding = strings.Repeat(" ", labelWidth-len(labelWithColon))
	}

	// Use MWh for values >= 1000 kWh
	unit := "kWh"
	if displayValue >= constants.KWhToMWh {
		displayValue = displayValue / constants.KWhToMWh
		unit = "MWh"
	}

	valueFormat := "%.1f"
	if rightAlign {
		valueFormat = "%7.1f"
	}

	if highlight {
		const bg = "\033[48;5;17m" // dark navy — complements the cyan/yellow value colors
		r := constants.Reset + bg  // reset fg then re-apply bg so it persists across color changes

		// Separate leading spaces from visible label text so the background starts
		// exactly one space before the first character and ends one space after the last.
		trimmed := strings.TrimLeft(labelWithColon, " ")
		allLeading := indent + labelWithColon[:len(labelWithColon)-len(trimmed)]
		outerIndent := allLeading[:len(allLeading)-1] // one space stays inside bg as left pad

		fmt.Fprintf(d.writer, "%s%s%s%s%s"+valueFormat+" %s%s %s(%s)%s%s\n",
			outerIndent+bg+" "+d.colors.SecondaryText, trimmed, r,
			padding, color, displayValue, unit, r,
			color, direction, bg+" ", constants.Reset)
	} else {
		fmt.Fprintf(d.writer, "%s%s%s%s%s%s"+valueFormat+" %s%s %s(%s)%s\n",
			indent, d.colors.SecondaryText, labelWithColon, constants.Reset,
			padding, color, displayValue, unit, constants.Reset,
			color, direction, constants.Reset)
	}
}

// ShowTrueUpReport displays the true-up year energy report.
func (d *Display) ShowTrueUpReport(report *aggregator.TrueUpReport) {
	d.printTrueUpHeader(report)
	d.printTrueUpCombined(report)
	if len(report.Systems) > 1 {
		d.printTrueUpSystems(report)
	}
	d.printSeparator()
}

func (d *Display) printTrueUpHeader(report *aggregator.TrueUpReport) {
	// Data coverage starts from the first day of the start month (full months used)
	dataStart := time.Date(report.StartDate.Year(), report.StartDate.Month(), 1, 0, 0, 0, 0, d.timezone)

	fmt.Fprintln(d.writer, "\n  "+d.colors.Headers+d.separatorLine+constants.Reset)
	fmt.Fprintf(d.writer, "    %s%sENPHASE MULTI-SYSTEM MONITOR%s\n", constants.Bold, d.colors.Headers, constants.Reset)
	fmt.Fprintln(d.writer, "  "+d.colors.Headers+d.separatorLine+constants.Reset)

	fmt.Fprintf(d.writer, "     %s%-15s%s%s 12:00 AM\n                              to\n                    %s%s\n\n",
		d.colors.SecondaryText, "Data Range:", d.colors.PrimaryText,
		dataStart.Format("Mon Jan 2, 2006"),
		report.EndDate.Format("Mon Jan 2, 2006 03:04 PM"), constants.Reset)

	fmt.Fprintf(d.writer, "     %s%-15s%s%s %s(full months used)%s\n",
		d.colors.SecondaryText, "True-Up Start:", d.colors.PrimaryText,
		report.StartDate.Format("Mon Jan 2, 2006"),
		d.colors.SecondaryText, constants.Reset)

	fmt.Fprintln(d.writer, "  "+d.colors.Headers+d.separatorLine+constants.Reset)
}

func (d *Display) printTrueUpCombined(report *aggregator.TrueUpReport) {
	fmt.Fprintf(d.writer, "\n   %s%sTRUE-UP ENERGY REPORT%s\n", constants.Bold, d.colors.PrimaryText, constants.Reset)
	fmt.Fprintln(d.writer, "  "+d.colors.SecondaryText+d.subSeparator+constants.Reset)

	d.printNetFlow("  Net Energy Flow", report.NetFlow, "  ", 24, false, true)
	fmt.Fprintln(d.writer)
	d.printMetric("Energy Produced", report.Production, d.colors.Production, "    ", 22, false)
	d.printMetric("Energy Consumed", report.Consumption, d.colors.TotalConsumed, "    ", 22, false)
	d.printMetric("Energy Imported", report.GridImport, d.colors.Import, "    ", 22, false)
	d.printMetric("Energy Exported", report.GridExport, d.colors.Export, "    ", 22, false)
}

func (d *Display) printTrueUpSystems(report *aggregator.TrueUpReport) {
	fmt.Fprintf(d.writer, "\n\n   %s%sINDIVIDUAL SYSTEMS REPORT%s\n", constants.Bold, d.colors.PrimaryText, constants.Reset)
	fmt.Fprintln(d.writer, "  "+d.colors.SecondaryText+d.subSeparator+constants.Reset)

	for i, sys := range report.Systems {
		fmt.Fprintf(d.writer, "\n    %s[%d]%s %s%s%s %s(%s)%s\n",
			d.colors.Headers, i+1, constants.Reset,
			constants.Bold, sys.Name, constants.Reset,
			d.colors.SecondaryText, sys.ID, constants.Reset)
		d.printNetFlow("Net Energy Flow", sys.NetFlow, "        ", 27, true, false)
		d.printMetric("Energy Produced", sys.Production, d.colors.Production, "        ", 27, true)
		d.printMetric("Energy Consumed", sys.Consumption, d.colors.TotalConsumed, "        ", 27, true)
		d.printMetric("Energy Imported", sys.GridImport, d.colors.Import, "        ", 27, true)
		d.printMetric("Energy Exported", sys.GridExport, d.colors.Export, "        ", 27, true)
	}
}

// ShowError displays an error message.
func (d *Display) ShowError(err error) {
	fmt.Fprintf(d.writer, "\n  %s%sERROR:%s\n", constants.Bold, d.colors.Error, constants.Reset)
	fmt.Fprintf(d.writer, "     %s%v%s\n\n", d.colors.Error, err, constants.Reset)
}

// ShowInfo displays an informational message.
func (d *Display) ShowInfo(message string) {
	fmt.Fprintf(d.writer, "\n  %s%s%s\n", d.colors.SecondaryText, message, constants.Reset)
}
