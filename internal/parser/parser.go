// Package parser provides utilities for parsing Enphase Cloud API v4 telemetry responses.
//
// PURPOSE
// -------
// Handles two response formats returned by different API endpoints (nested arrays
// for import/export, flat arrays for production/consumption/battery).
//
// For API endpoint details and response format documentation, see:
//   - internal/api/client.go: endpoint list and response format examples
//   - docs/ARCHITECTURE.md: "Data Flow Diagram" section
//
// RESPONSE FORMATS
// ----------------
// Format 1 - Nested arrays (energy_import_telemetry, energy_export_telemetry):
//   { "intervals": [[{...}, {...}], [{...}, {...}]] }
//
// Format 2 - Flat arrays (production_meter, consumption_meter, battery):
//   { "intervals": [{...}, {...}, {...}] }

package parser

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"enphase-monitor/internal/constants"
)

// TelemetryResponse represents the response from telemetry endpoints.
// Note: production_meter, consumption_meter, battery return a single array.
type TelemetryResponse struct {
	LastReportedAggregateSOC string              `json:"last_reported_aggregate_soc,omitempty"` // Battery state of charge percentage as string (e.g., "97%")
	Intervals                []TelemetryInterval `json:"intervals"`
}

// TelemetryResponseNested represents the response from energy_import_telemetry and energy_export_telemetry,
// which return intervals as an array of arrays.
type TelemetryResponseNested struct {
	Intervals [][]TelemetryInterval `json:"intervals"`
}

// TelemetryInterval represents a single 15-minute interval.
type TelemetryInterval struct {
	EndAt      int64   `json:"end_at"`      // Unix timestamp
	WhDel      float64 `json:"wh_del"`      // Energy delivered (for production_meter)
	WhRcv      float64 `json:"wh_rcv"`      // Energy received (legacy - not used in current endpoints)
	WhImported float64 `json:"wh_imported"` // Energy imported (for energy_import_telemetry)
	WhExported float64 `json:"wh_exported"` // Energy exported (for energy_export_telemetry)
	Enwh       float64 `json:"enwh"`        // Energy in Wh (for production_meter, consumption_meter)
	Charge     struct {
		Enwh float64 `json:"enwh"` // Energy charged in Wh
	} `json:"charge"`
	Discharge struct {
		Enwh float64 `json:"enwh"` // Energy discharged in Wh
	} `json:"discharge"`
}

// LifetimeResponse represents the response from _lifetime endpoints (daily aggregated data).
// These endpoints return daily totals as arrays of numbers instead of 15-minute intervals.
// The array starts from start_date and each element represents one day's energy in Wh.
// Different endpoints use different field names for the data array.
type LifetimeResponse struct {
	SystemID    int64     `json:"system_id"`
	StartDate   string    `json:"start_date"`  // Format: "YYYY-MM-DD"
	Production  []float64 `json:"production"`  // For energy_lifetime
	Consumption []float64 `json:"consumption"` // For consumption_lifetime
	Import      []float64 `json:"import"`      // For energy_import_lifetime
	Export      []float64 `json:"export"`      // For energy_export_lifetime
	Intervals   []float64 `json:"intervals"`   // Fallback field for other endpoints
}

// DailyInterval represents a single day's aggregated energy value with its date.
// This is created by parsing LifetimeResponse arrays.
type DailyInterval struct {
	Date string  // Format: "YYYY-MM-DD"
	Enwh float64 // Energy in Wh for this day
}

// ParseNestedTelemetryResponse parses a telemetry response that has nested intervals (array of arrays).
// This is used for energy_import_telemetry and energy_export_telemetry endpoints.
func ParseNestedTelemetryResponse(bodyBytes []byte) ([]TelemetryInterval, error) {
	var data TelemetryResponseNested
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		bodyPreview := string(bodyBytes)
		if len(bodyPreview) > constants.ResponseBodyPreviewLength {
			bodyPreview = bodyPreview[:constants.ResponseBodyPreviewLength] + "..."
		}
		return nil, fmt.Errorf("failed to decode nested telemetry response (body preview: %s): %w", bodyPreview, err)
	}

	// Flatten the array of arrays into a single array
	var allIntervals []TelemetryInterval
	for _, intervalArray := range data.Intervals {
		allIntervals = append(allIntervals, intervalArray...)
	}

	return allIntervals, nil
}

// ParseTelemetryResponse parses a telemetry response with a single intervals array.
// This is used for production_meter, consumption_meter, and battery endpoints.
func ParseTelemetryResponse(bodyBytes []byte) ([]TelemetryInterval, error) {
	var data TelemetryResponse
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to decode telemetry response: %w", err)
	}

	return data.Intervals, nil
}

// ReadResponseBody reads the entire response body and returns the bytes.
// This is necessary because the body can only be read once.
//
// Note: The caller is responsible for closing the response body (typically via defer).
// This function only reads the body, it does not close it.
func ReadResponseBody(respBody io.ReadCloser) ([]byte, error) {
	bodyBytes, err := io.ReadAll(respBody)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return bodyBytes, nil
}

// SumIntervalValues sums a specific field from telemetry intervals.
// The fieldName should use one of the field constants defined in constants.go:
//   - FieldWhImported: sums WhImported values (grid import)
//   - FieldWhExported: sums WhExported values (grid export)
//   - FieldWhDel: sums WhDel values (production)
//   - FieldEnwh: sums Enwh values (consumption/battery)
func SumIntervalValues(intervals []TelemetryInterval, fieldName string) float64 {
	var total float64
	for _, interval := range intervals {
		switch fieldName {
		case constants.FieldWhImported:
			total += interval.WhImported
		case constants.FieldWhExported:
			total += interval.WhExported
		case constants.FieldWhDel:
			total += interval.WhDel
		case constants.FieldEnwh:
			total += interval.Enwh
		}
	}
	return total
}

// ParseLifetimeResponse parses a lifetime response that contains daily aggregated data.
// Returns the daily intervals and handles both "production" and "intervals" field names.
// The API returns arrays of numbers where each element is one day's energy in Wh.
func ParseLifetimeResponse(bodyBytes []byte) ([]DailyInterval, error) {
	var data LifetimeResponse
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		bodyPreview := string(bodyBytes)
		if len(bodyPreview) > constants.ResponseBodyPreviewLength {
			bodyPreview = bodyPreview[:constants.ResponseBodyPreviewLength] + "..."
		}
		return nil, fmt.Errorf("failed to decode lifetime response (body preview: %s): %w", bodyPreview, err)
	}

	// Parse start_date
	startDate, err := time.Parse(constants.DateFormat, data.StartDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start_date format: %w", err)
	}

	// Convert array of numbers to DailyInterval objects
	// Different endpoints use different field names:
	// - energy_lifetime: "production"
	// - consumption_lifetime: "consumption"
	// - energy_import_lifetime: "import"
	// - energy_export_lifetime: "export"
	// - fallback: "intervals"
	var values []float64
	if len(data.Production) > 0 {
		values = data.Production
	} else if len(data.Consumption) > 0 {
		values = data.Consumption
	} else if len(data.Import) > 0 {
		values = data.Import
	} else if len(data.Export) > 0 {
		values = data.Export
	} else {
		values = data.Intervals
	}

	dailyIntervals := make([]DailyInterval, 0, len(values))
	for i, value := range values {
		// Calculate date for this value (start_date + i days)
		date := startDate.AddDate(0, 0, i)
		dailyIntervals = append(dailyIntervals, DailyInterval{
			Date: date.Format(constants.DateFormat),
			Enwh: value,
		})
	}

	return dailyIntervals, nil
}

// SumDailyIntervals sums energy values from daily intervals within a date range.
// Filters intervals to only include dates within [startDate, endDate] inclusive.
func SumDailyIntervals(intervals []DailyInterval, startDate, endDate string) float64 {
	var total float64
	for _, interval := range intervals {
		// Filter by date range (inclusive)
		if interval.Date >= startDate && interval.Date <= endDate {
			total += interval.Enwh
		}
	}
	return total
}
