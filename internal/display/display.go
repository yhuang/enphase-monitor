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

	// Calculate date range based on query type (day/month/year)
	periodStart, periodEnd := timezone.GetBoundaries(queryDate, queryType, d.timezone)

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

	d.printMetric("Energy Produced", metrics.ProductionToday, d.colors.Production, "     ", 19, false)
	d.printMetric("Energy Consumed", metrics.ConsumptionToday, d.colors.TotalConsumed, "     ", 19, false)

	d.printNetFlow("  Net Energy Flow", metrics.NetImportToday, "   ", 21, false)
}

func (d *Display) printIndividualSystems(metrics *aggregator.AggregatedMetrics) {
	if len(metrics.Systems) <= 1 {
		return
	}

	fmt.Fprintf(d.writer, "\n   %s%sINDIVIDUAL SYSTEMS REPORT%s\n", constants.Bold, d.colors.PrimaryText, constants.Reset)
	fmt.Fprintln(d.writer, "  "+d.colors.SecondaryText+d.subSeparator+constants.Reset)

	for i, sys := range metrics.Systems {
		displayName := sys.Name
		identifier := sys.ID
		fmt.Fprintf(d.writer, "\n    %s[%d]%s %s%s%s %s(%s)%s\n",
			d.colors.Headers, i+1, constants.Reset,
			constants.Bold, displayName, constants.Reset,
			d.colors.SecondaryText, identifier, constants.Reset)
		d.printMetric("Imported from the Grid", sys.GridImportToday, d.colors.Import, "        ", 27, true)
		d.printMetric("Exported to the Grid", sys.GridExportToday, d.colors.Export, "        ", 27, true)
		d.printMetric("Captured from the Sun", sys.ProductionToday, d.colors.Production, "        ", 27, true)
		d.printNetFlow("Net Energy Flow", sys.NetImportedToday, "        ", 27, true)
		d.printMetric("Charged to Battery", sys.BatteryChargedToday, d.colors.Charge, "        ", 27, true)
		d.printMetric("Discharged from Battery", sys.BatteryDischargedToday, d.colors.Discharge, "        ", 27, true)
		// Only show battery charge percentage for day queries
		// For month/year queries, the SOC is just a snapshot and not meaningful
		if metrics.QueryType == constants.QueryTypeDay {
			fmt.Fprintf(d.writer, "        %sBattery Charge Percentage:%s %s%10d%%%s\n",
				d.colors.SecondaryText, constants.Reset, d.colors.Charge, sys.BatterySOC, constants.Reset)
		}
		d.printMetric("Total Energy Consumed", sys.ConsumptionToday, d.colors.TotalConsumed, "        ", 27, true)
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

func (d *Display) printNetFlow(label string, netValue float64, indent string, labelWidth int, rightAlign bool) {
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
	fmt.Fprintf(d.writer, "%s%s%s%s%s%s"+valueFormat+" %s%s %s(%s)%s\n",
		indent, d.colors.SecondaryText, labelWithColon, constants.Reset,
		padding, color, displayValue, unit, constants.Reset,
		color, direction, constants.Reset)
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
