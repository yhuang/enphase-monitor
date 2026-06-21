package aggregator

import (
	"time"

	"enphase-monitor/internal/constants"
)

// AggregatedMetrics represents combined data from all Systems for a single query.
type AggregatedMetrics struct {
	Timestamp time.Time
	QueryDate time.Time           // The date being queried (zero value means today)
	QueryMode constants.QueryMode // The Query Mode (Day, Month, Year, or True-Up)

	// Energy totals for the queried period (kWh)
	// Field names retain the `Today` suffix for backwards compatibility; the
	// values reflect whatever Query Mode produced them.
	ProductionToday  float64
	ConsumptionToday float64
	GridImportToday  float64
	GridExportToday  float64
	NetFlowToday     float64 // Net Flow = Grid Import − Grid Export (positive = net import direction, negative = net export direction)

	// Per-System breakdown
	Systems []SystemMetrics

	// Cache status
	CacheUsed    bool // True if any cached data was used
	AllFromCache bool // True only when every System was served entirely from Cache

	// Weather for the queried day, populated best-effort by the app layer for
	// Day Mode only (never for month, year, true-up, or cache-only reports).
	// Nil when the query mode is not Day or the weather lookup failed; consumers
	// must nil-check before reading.
	Weather *DailyWeather
}

// DailyWeather is the day's weather summary attached to a Day-Mode report.
// It mirrors weather.DailyWeather but is defined here so the aggregator and
// display packages do not depend on the weather package (the app layer maps
// between them).
type DailyWeather struct {
	TempHigh        float64
	TempLow         float64
	TempUnit        string  // display symbol, e.g. "°F"
	WeatherCode     int     // WMO weather interpretation code (precise; Condition is its collapsed label)
	Condition       string  // human-readable label (from WMO weather code)
	CloudCoverPct   float64 // mean cloud cover, %
	PrecipitationMM float64 // total precipitation, mm
	SolarRadiation  float64 // daily shortwave radiation, kWh/m²

	// Current snapshot ("now"), populated only for a live today report. When
	// HasCurrent, the display shows these instantaneous values for condition and
	// temperature alongside the day's high/low range. Display-only — the dataset
	// and export use the daily aggregates above.
	HasCurrent             bool
	CurrentTemp            float64
	CurrentCondition       string
	CurrentCloudCoverPct   float64
	CurrentPrecipitationMM float64
}

// TrueUpReport holds energy metrics for a True-Up Period.
type TrueUpReport struct {
	StartDate   time.Time // user-provided True-Up Start Date (display only; data starts from the 1st of this month)
	EndDate     time.Time // last day with complete data (yesterday)
	Timestamp   time.Time // when the data was fetched
	CacheUsed   bool      // true if any cached data was used
	GridImport  float64
	GridExport  float64
	Production  float64
	Consumption float64
	NetFlow     float64 // Net Flow = Grid Import − Grid Export (positive = net import direction, negative = net export direction)
	Systems     []TrueUpSystemReport
}

// TrueUpSystemReport holds per-System metrics for a True-Up Period.
type TrueUpSystemReport struct {
	Name        string
	ID          string
	GridImport  float64
	GridExport  float64
	Production  float64
	Consumption float64
	NetFlow     float64
}

// SystemMetrics represents metrics for a single System.
type SystemMetrics struct {
	Name             string
	ID               string // System ID (Enphase system ID in Enlighten)
	ProductionToday  float64
	ConsumptionToday float64
	BatterySOC       int // Battery State of Charge (SOC), 0–100 (percent)

	// Per-period energy values for this System (field names retain the
	// `Today` suffix for backwards compatibility; values reflect the active
	// Query Mode).
	GridImportToday        float64
	GridExportToday        float64
	BatteryChargedToday    float64
	BatteryDischargedToday float64
	NetFlowToday           float64
}
