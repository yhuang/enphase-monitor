// Package main - response_parser.go
//
// PURPOSE
// -------
// Utility functions for parsing Enphase Cloud API v4 telemetry responses.
// Handles two different response formats returned by different API endpoints.
//
// For API endpoint details and response format documentation, see:
//   - cloud_client.go: lines 67-82 for endpoint list and response format examples
//   - ARCHITECTURE.md: "Data Flow Diagram" section
//
// RESPONSE FORMATS
// ----------------
// Format 1 - Nested arrays (energy_import_telemetry, energy_export_telemetry):
//   { "intervals": [[{...}, {...}], [{...}, {...}]] }
//
// Format 2 - Flat arrays (production_meter, consumption_meter, battery):
//   { "intervals": [{...}, {...}, {...}] }

package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// parseNestedTelemetryResponse parses a telemetry response that has nested intervals (array of arrays).
// This is used for energy_import_telemetry and energy_export_telemetry endpoints.
func parseNestedTelemetryResponse(bodyBytes []byte) ([]TelemetryInterval, error) {
	var data TelemetryResponseNested
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		bodyPreview := string(bodyBytes)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
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

// parseTelemetryResponse parses a telemetry response with a single intervals array.
// This is used for production_meter, consumption_meter, and battery endpoints.
func parseTelemetryResponse(bodyBytes []byte) ([]TelemetryInterval, error) {
	var data TelemetryResponse
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to decode telemetry response: %w", err)
	}

	return data.Intervals, nil
}

// readResponseBody reads the entire response body and returns the bytes.
// This is necessary because the body can only be read once.
//
// Note: The caller is responsible for closing the response body (typically via defer).
// This function only reads the body, it does not close it.
func readResponseBody(respBody io.ReadCloser) ([]byte, error) {
	bodyBytes, err := io.ReadAll(respBody)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return bodyBytes, nil
}

// sumIntervalValues sums a specific field from telemetry intervals.
// The fieldName parameter determines which field to sum:
//   - "wh_imported": sums WhImported values
//   - "wh_exported": sums WhExported values
//   - "wh_del": sums WhDel values
//   - "enwh": sums Enwh values
func sumIntervalValues(intervals []TelemetryInterval, fieldName string) float64 {
	var total float64
	for _, interval := range intervals {
		switch fieldName {
		case "wh_imported":
			total += interval.WhImported
		case "wh_exported":
			total += interval.WhExported
		case "wh_del":
			total += interval.WhDel
		case "enwh":
			total += interval.Enwh
		}
	}
	return total
}
