package aggregator

import (
	"time"

	"enphase-monitor/internal/constants"
)

// AggregatedMetrics represents combined data from all Enphase systems.
type AggregatedMetrics struct {
	Timestamp time.Time
	QueryDate time.Time             // The date being queried (zero value means today)
	QueryMode constants.QueryMode   // The Query Mode (day/month/year/true-up)

	// Today's Energy (kWh)
	ProductionToday  float64
	ConsumptionToday float64
	GridImportToday  float64
	GridExportToday  float64
	NetFlowToday     float64 // Grid Import - Grid Export (positive = net import, negative = net export)

	// Individual System Data
	Systems []SystemMetrics

	// Cache status
	CacheUsed    bool // Indicates if any cached data was used
	AllFromCache bool // True only when every system was served entirely from cache
}

// TrueUpReport holds energy metrics for a true-up year period.
type TrueUpReport struct {
	StartDate   time.Time // user-provided true-up start date (display only; data starts from the 1st of this month)
	EndDate     time.Time // last day with complete data (yesterday)
	Timestamp   time.Time // when the data was fetched
	CacheUsed   bool      // true if any cached data was used
	GridImport  float64
	GridExport  float64
	Production  float64
	Consumption float64
	NetFlow     float64 // GridImport - GridExport (positive = net import, negative = net export)
	Systems     []TrueUpSystemReport
}

// TrueUpSystemReport holds per-system metrics for the true-up period.
type TrueUpSystemReport struct {
	Name        string
	ID          string
	GridImport  float64
	GridExport  float64
	Production  float64
	Consumption float64
	NetFlow     float64
}

// SystemMetrics represents metrics for a single system.
type SystemMetrics struct {
	Name             string
	ID               string // System ID (for Cloud API)
	ProductionToday  float64
	ConsumptionToday float64
	BatterySOC       int

	// Today's energy values for this system
	GridImportToday        float64
	GridExportToday        float64
	BatteryChargedToday    float64
	BatteryDischargedToday float64
	NetFlowToday           float64
}
