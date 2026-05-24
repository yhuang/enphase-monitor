// types.go defines types used by the CloudClient interface (e.g. LocalMetrics).
// Package comment is in client.go.
package api

import "time"

// LocalMetrics contains processed metrics from the Cloud API for a single System.
// All energy values are in kilowatt-hours (kWh). BatterySOC is 0–100 (percent).
// Field names retain the `Today` suffix from when the API was Day-Mode only;
// the values now reflect whatever Query Mode produced them (Day / Month / Year / True-Up).
type LocalMetrics struct {
	Timestamp              time.Time // When these metrics were collected
	ProductionToday        float64   // kWh - Production for the queried period
	ConsumptionToday       float64   // kWh - Consumption for the queried period
	GridImportToday        float64   // kWh - Grid Import for the queried period
	GridExportToday        float64   // kWh - Grid Export for the queried period
	BatteryChargedToday    float64   // kWh - Battery Charge for the queried period
	BatteryDischargedToday float64   // kWh - Battery Discharge for the queried period
	BatterySOC             int       // Battery State of Charge (SOC), 0–100 (percent)
}
