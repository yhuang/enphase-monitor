package display

import (
	"fmt"
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
type Display struct {
	colors        config.ColorConfig
	timezone      *time.Location
	separatorLine string
	subSeparator  string
}

// NewDisplayWithColorsAndTimezone creates a new display instance with custom colors and timezone.
func NewDisplayWithColorsAndTimezone(colors config.ColorConfig, tz *time.Location) *Display {
	defaultColors := GetDefaultColors()
	colors.MergeWithDefaults(defaultColors)

	return &Display{
		colors:        colors,
		timezone:      tz,
		separatorLine: strings.Repeat("=", constants.SeparatorWidth),
		subSeparator:  strings.Repeat("-", constants.SeparatorWidth),
	}
}

// ShowMetrics displays the aggregated metrics in a formatted output.
func (d *Display) ShowMetrics(metrics *aggregator.AggregatedMetrics) {
	d.printHeader(metrics.Timestamp, metrics.CacheUsed, metrics.QueryDate)
	d.printTodayEnergy(metrics)
	d.printIndividualSystems(metrics)
	d.printSeparator()
}

func (d *Display) printHeader(timestamp time.Time, cacheUsed bool, queryDate time.Time) {
	fmt.Println("\n" + d.colors.Headers + d.separatorLine + constants.Reset)
	fmt.Printf("  %s%sENPHASE MULTI-SYSTEM MONITOR%s\n", constants.Bold, d.colors.Headers, constants.Reset)
	fmt.Println(d.colors.Headers + d.separatorLine + constants.Reset)

	nowLocal := time.Now().In(d.timezone)

	if !queryDate.IsZero() {
		queryDayLocal := time.Date(queryDate.Year(), queryDate.Month(), queryDate.Day(), 0, 0, 0, 0, d.timezone)
		todayLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, d.timezone)

		var dayEndLocal time.Time
		if queryDayLocal.Equal(todayLocal) {
			dayEndLocal = nowLocal
		} else {
			dayEndLocal = time.Date(queryDate.Year(), queryDate.Month(), queryDate.Day(), 23, 59, 59, 0, d.timezone)
		}

		dayStartLocal := time.Date(queryDate.Year(), queryDate.Month(), queryDate.Day(), 0, 0, 0, 0, d.timezone)

		fmt.Printf("  %s%-16s%s%s 12:00 AM\n                          to\n                  %s%s\n\n",
			d.colors.SecondaryText, "Query Range:   ", d.colors.PrimaryText,
			dayStartLocal.Format("Mon Jan 2, 2006"),
			dayEndLocal.Format("Mon Jan 2, 2006 03:04 PM"), constants.Reset)
	} else {
		todayStartLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, d.timezone)

		fmt.Printf("  %s%-16s%s%s 12:00 AM\n                          to\n                  %s%s\n\n",
			d.colors.SecondaryText, "Query Range:   ", d.colors.PrimaryText,
			todayStartLocal.Format("Mon Jan 2, 2006"),
			nowLocal.Format("Mon Jan 2, 2006 03:04 PM"), constants.Reset)
	}

	timestampLocal := timestamp.In(d.timezone)
	fmt.Printf("  %s%-16s%s%s", d.colors.SecondaryText, "Last Updated:   ", d.colors.PrimaryText, timestampLocal.Format("Mon Jan 2, 2006 03:04:05 PM"))
	if cacheUsed {
		fmt.Printf(" %s(cached)%s", d.colors.SecondaryText, constants.Reset)
	} else {
		fmt.Printf(" %s(live)%s", d.colors.Discharge, constants.Reset)
	}
	fmt.Println()
	fmt.Println(d.colors.Headers + d.separatorLine + constants.Reset)
}

func (d *Display) printTodayEnergy(metrics *aggregator.AggregatedMetrics) {
	fmt.Printf("\n %s%sCOMBINED ENERGY REPORT (kWh)%s\n", constants.Bold, d.colors.PrimaryText, constants.Reset)
	fmt.Println(d.colors.SecondaryText + d.subSeparator + constants.Reset)

	d.printMetric("Produced", metrics.ProductionToday, d.colors.Production, "  ")
	d.printMetric("Consumed", metrics.ConsumptionToday, d.colors.TotalConsumed, "  ")

	d.printNetFlow("Net Energy Flow", metrics.NetImportToday, "  ")
}

func (d *Display) printIndividualSystems(metrics *aggregator.AggregatedMetrics) {
	if len(metrics.Systems) <= 1 {
		return
	}

	fmt.Printf("\n %s%sINDIVIDUAL SYSTEMS REPORT%s\n", constants.Bold, d.colors.PrimaryText, constants.Reset)
	fmt.Println(d.colors.SecondaryText + d.subSeparator + constants.Reset)

	for i, sys := range metrics.Systems {
		displayName := sys.Name
		identifier := sys.ID
		fmt.Printf("\n  %s[%d]%s %s%s%s %s(%s)%s\n",
			d.colors.Headers, i+1, constants.Reset,
			constants.Bold, displayName, constants.Reset,
			d.colors.SecondaryText, identifier, constants.Reset)
		d.printMetric("Imported from the Grid", sys.GridImportToday, d.colors.Import, "      ")
		d.printMetric("Exported to the Grid", sys.GridExportToday, d.colors.Export, "      ")
		d.printMetric("Captured from the Sun", sys.ProductionToday, d.colors.Production, "      ")
		d.printNetFlow("Net Energy Flow", sys.NetImportedToday, "      ")
		d.printMetric("Charged to Battery", sys.BatteryChargedToday, d.colors.Charge, "      ")
		d.printMetric("Discharged from Battery", sys.BatteryDischargedToday, d.colors.Discharge, "      ")
		fmt.Printf("      %sBattery Charge Percentage:%s      %s%7d%%%s\n",
			d.colors.SecondaryText, constants.Reset, d.colors.Charge, sys.BatterySOC, constants.Reset)
		d.printMetric("Total Consumed", sys.ConsumptionToday, d.colors.TotalConsumed, "      ")
	}
}

func (d *Display) printSeparator() {
	fmt.Println("\n" + d.colors.Headers + d.separatorLine + constants.Reset + "\n")
}

func (d *Display) printMetric(label string, value float64, valueColor string, indent string) {
	fmt.Printf("%s%s%s:%s%s%8.1f kWh%s\n",
		indent, d.colors.SecondaryText, label, constants.Reset,
		valueColor, value, constants.Reset)
}

func (d *Display) printNetFlow(label string, netValue float64, indent string) {
	if netValue < 0 {
		fmt.Printf("%s%s%s:%s%s%8.1f kWh%s %s(export)%s\n",
			indent, d.colors.SecondaryText, label, constants.Reset,
			d.colors.NetExport, -netValue, constants.Reset,
			d.colors.NetExport, constants.Reset)
	} else {
		fmt.Printf("%s%s%s:%s%s%8.1f kWh%s %s(import)%s\n",
			indent, d.colors.SecondaryText, label, constants.Reset,
			d.colors.NetImport, netValue, constants.Reset,
			d.colors.NetImport, constants.Reset)
	}
}

// ShowError displays an error message.
func (d *Display) ShowError(err error) {
	fmt.Printf("\n%s%sERROR:%s\n", constants.Bold, d.colors.Error, constants.Reset)
	fmt.Printf("   %s%v%s\n\n", d.colors.Error, err, constants.Reset)
}

// ShowInfo displays an informational message.
func (d *Display) ShowInfo(message string) {
	fmt.Printf("\n%s%s%s\n", d.colors.SecondaryText, message, constants.Reset)
}
