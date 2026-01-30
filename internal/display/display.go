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
	d.printHeader(metrics.Timestamp, metrics.CacheUsed, metrics.QueryDate)
	d.printTodayEnergy(metrics)
	d.printIndividualSystems(metrics)
	d.printSeparator()
}

// getDateRange calculates the display date range for a query.
// Returns the start time (midnight) and end time for display purposes.
func (d *Display) getDateRange(queryDate, nowLocal time.Time) (start, end time.Time) {
	// Determine which date to use
	targetDate := nowLocal
	if !queryDate.IsZero() {
		targetDate = queryDate
	}

	// Calculate midnight of the target date
	start = time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, d.timezone)

	// Calculate end time: if querying today, use current time; otherwise use end of day
	todayMidnight := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, d.timezone)
	if start.Equal(todayMidnight) {
		end = nowLocal
	} else {
		end = time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 23, 59, 59, 0, d.timezone)
	}

	return start, end
}

func (d *Display) printHeader(timestamp time.Time, cacheUsed bool, queryDate time.Time) {
	fmt.Fprintln(d.writer, "\n"+d.colors.Headers+d.separatorLine+constants.Reset)
	fmt.Fprintf(d.writer, "  %s%sENPHASE MULTI-SYSTEM MONITOR%s\n", constants.Bold, d.colors.Headers, constants.Reset)
	fmt.Fprintln(d.writer, d.colors.Headers+d.separatorLine+constants.Reset)

	nowLocal := time.Now().In(d.timezone)

	// Use helper to calculate date range (eliminates duplicate logic)
	dayStart, dayEnd := d.getDateRange(queryDate, nowLocal)

	fmt.Fprintf(d.writer, "  %s%-16s%s%s 12:00 AM\n                          to\n                  %s%s\n\n",
		d.colors.SecondaryText, "Query Range:   ", d.colors.PrimaryText,
		dayStart.Format("Mon Jan 2, 2006"),
		dayEnd.Format("Mon Jan 2, 2006 03:04 PM"), constants.Reset)

	timestampLocal := timestamp.In(d.timezone)
	fmt.Fprintf(d.writer, "  %s%-16s%s%s", d.colors.SecondaryText, "Last Updated:   ", d.colors.PrimaryText, timestampLocal.Format("Mon Jan 2, 2006 03:04:05 PM"))
	if cacheUsed {
		fmt.Fprintf(d.writer, " %s(cached)%s", d.colors.SecondaryText, constants.Reset)
	} else {
		fmt.Fprintf(d.writer, " %s(live)%s", d.colors.Discharge, constants.Reset)
	}
	fmt.Fprintln(d.writer)
	fmt.Fprintln(d.writer, d.colors.Headers+d.separatorLine+constants.Reset)
}

func (d *Display) printTodayEnergy(metrics *aggregator.AggregatedMetrics) {
	fmt.Fprintf(d.writer, "\n %s%sCOMBINED ENERGY REPORT (kWh)%s\n", constants.Bold, d.colors.PrimaryText, constants.Reset)
	fmt.Fprintln(d.writer, d.colors.SecondaryText+d.subSeparator+constants.Reset)

	d.printMetric("Produced", metrics.ProductionToday, d.colors.Production, "  ")
	d.printMetric("Consumed", metrics.ConsumptionToday, d.colors.TotalConsumed, "  ")

	d.printNetFlow("Net Energy Flow", metrics.NetImportToday, "  ")
}

func (d *Display) printIndividualSystems(metrics *aggregator.AggregatedMetrics) {
	if len(metrics.Systems) <= 1 {
		return
	}

	fmt.Fprintf(d.writer, "\n %s%sINDIVIDUAL SYSTEMS REPORT%s\n", constants.Bold, d.colors.PrimaryText, constants.Reset)
	fmt.Fprintln(d.writer, d.colors.SecondaryText+d.subSeparator+constants.Reset)

	for i, sys := range metrics.Systems {
		displayName := sys.Name
		identifier := sys.ID
		fmt.Fprintf(d.writer, "\n  %s[%d]%s %s%s%s %s(%s)%s\n",
			d.colors.Headers, i+1, constants.Reset,
			constants.Bold, displayName, constants.Reset,
			d.colors.SecondaryText, identifier, constants.Reset)
		d.printMetric("Imported from the Grid", sys.GridImportToday, d.colors.Import, "      ")
		d.printMetric("Exported to the Grid", sys.GridExportToday, d.colors.Export, "      ")
		d.printMetric("Captured from the Sun", sys.ProductionToday, d.colors.Production, "      ")
		d.printNetFlow("Net Energy Flow", sys.NetImportedToday, "      ")
		d.printMetric("Charged to Battery", sys.BatteryChargedToday, d.colors.Charge, "      ")
		d.printMetric("Discharged from Battery", sys.BatteryDischargedToday, d.colors.Discharge, "      ")
		fmt.Fprintf(d.writer, "      %sBattery Charge Percentage:%s      %s%7d%%%s\n",
			d.colors.SecondaryText, constants.Reset, d.colors.Charge, sys.BatterySOC, constants.Reset)
		d.printMetric("Total Consumed", sys.ConsumptionToday, d.colors.TotalConsumed, "      ")
	}
}

func (d *Display) printSeparator() {
	fmt.Fprintln(d.writer, "\n"+d.colors.Headers+d.separatorLine+constants.Reset+"\n")
}

func (d *Display) printMetric(label string, value float64, valueColor string, indent string) {
	fmt.Fprintf(d.writer, "%s%s%s:%s%s%8.1f kWh%s\n",
		indent, d.colors.SecondaryText, label, constants.Reset,
		valueColor, value, constants.Reset)
}

func (d *Display) printNetFlow(label string, netValue float64, indent string) {
	// Consolidate duplicate format logic using direction variable
	var color, direction string
	var displayValue float64

	if netValue < 0 {
		color = d.colors.NetExport
		direction = "export"
		displayValue = -netValue
	} else {
		color = d.colors.NetImport
		direction = "import"
		displayValue = netValue
	}

	fmt.Fprintf(d.writer, "%s%s%s:%s%s%8.1f kWh%s %s(%s)%s\n",
		indent, d.colors.SecondaryText, label, constants.Reset,
		color, displayValue, constants.Reset,
		color, direction, constants.Reset)
}

// ShowError displays an error message.
func (d *Display) ShowError(err error) {
	fmt.Fprintf(d.writer, "\n%s%sERROR:%s\n", constants.Bold, d.colors.Error, constants.Reset)
	fmt.Fprintf(d.writer, "   %s%v%s\n\n", d.colors.Error, err, constants.Reset)
}

// ShowInfo displays an informational message.
func (d *Display) ShowInfo(message string) {
	fmt.Fprintf(d.writer, "\n%s%s%s\n", d.colors.SecondaryText, message, constants.Reset)
}
