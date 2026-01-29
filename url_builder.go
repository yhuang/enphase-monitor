// Package main - url_builder.go
//
// PURPOSE
// -------
// This file provides utility functions for constructing Enphase API URLs.
// Centralizes URL building logic to ensure consistent formatting and parameter handling.
//
// KEY FEATURES
// ------------
//   - Date range parameter formatting (Unix timestamps)
//   - API key injection
//   - System ID and endpoint path construction
//   - Uses constants from constants.go for base URLs
package main

import (
	"fmt"
	"time"
)

// buildTelemetryURL constructs a URL for a telemetry endpoint with date range parameters.
// This centralizes URL construction logic and ensures consistent parameter formatting.
func buildTelemetryURL(systemID, endpoint, apiKey string, dayStart, dayEnd time.Time) string {
	baseURL := fmt.Sprintf("%s/%s/%s", EnphaseAPIv4SystemsURL, systemID, endpoint)
	return fmt.Sprintf("%s?key=%s&start_at=%d&end_at=%d",
		baseURL,
		apiKey,
		dayStart.Unix(),
		dayEnd.Unix(),
	)
}
