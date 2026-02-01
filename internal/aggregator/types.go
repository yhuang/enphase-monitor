package aggregator

import (
	"time"
)

// AggregatedMetrics represents combined data from all Enphase systems
type AggregatedMetrics struct {
	Timestamp time.Time
	QueryDate time.Time // The date being queried (zero value means today)

	// Today's Energy (kWh)
	ProductionToday  float64
	ConsumptionToday float64
	GridImportToday  float64
	GridExportToday  float64
	NetImportToday   float64 // Grid Import - Grid Export (positive = import, negative = export)

	// Individual System Data
	Systems []SystemMetrics

	// Cache status
	CacheUsed bool // Indicates if any cached data was used
}

// SystemMetrics represents metrics for a single system
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
	NetImportedToday       float64
}
