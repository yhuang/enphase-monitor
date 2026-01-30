// Package api - types.go
//
// PURPOSE
// -------
// Defines types used by the CloudClient interface, particularly LocalMetrics
// which is the return type for GetMetricsFromCloud.
//
// These types are exported to allow consumers of the api package to work with
// the data structures without needing to import implementation details.
package api

import "time"

// LocalMetrics contains processed metrics from the Cloud API for a single system.
//
// This struct standardizes the format of metrics returned from GetMetricsFromCloud.
// All energy values are in kilowatt-hours (kWh). Battery SOC is a percentage (0-100).
//
// Fields:
//   - Timestamp: When these metrics were collected (time.Now())
//   - ProductionToday: Solar energy produced today (kWh)
//   - ConsumptionToday: Energy consumed today (kWh)
//   - GridImportToday: Energy imported from grid today (kWh)
//   - GridExportToday: Energy exported to grid today (kWh)
//   - BatteryChargedToday: Energy charged to battery today (kWh)
//   - BatteryDischargedToday: Energy discharged from battery today (kWh)
//   - BatterySOC: State of charge percentage (0-100)
type LocalMetrics struct {
	Timestamp              time.Time // When these metrics were collected
	ProductionToday        float64   // kWh - Solar energy produced today
	ConsumptionToday       float64   // kWh - Energy consumed today
	GridImportToday        float64   // kWh - Energy imported from grid today
	GridExportToday        float64   // kWh - Energy exported to grid today
	BatteryChargedToday    float64   // kWh - Energy charged to battery today
	BatteryDischargedToday float64   // kWh - Energy discharged from battery today
	BatterySOC             int       // State of charge percentage (0-100)
}
