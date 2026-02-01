// Package urlbuilder provides utility functions for constructing Enphase API URLs.
//
// PURPOSE
// -------
// This package provides utility functions for constructing Enphase API URLs.
// Centralizes URL building logic to ensure consistent formatting and parameter handling.
//
// KEY FEATURES
// ------------
//   - Date range parameter formatting (Unix timestamps)
//   - API key injection
//   - System ID and endpoint path construction
//   - Uses constants from internal/constants for base URLs
package urlbuilder

import (
	"fmt"
	"strconv"
	"time"

	"enphase-monitor/internal/constants"
)

// BuildTelemetryURL constructs a URL for a telemetry endpoint with date range parameters.
// This centralizes URL construction logic and ensures consistent parameter formatting.
func BuildTelemetryURL(systemID, endpoint, apiKey string, dayStart, dayEnd time.Time) string {
	baseURL := fmt.Sprintf("%s/%s/%s", constants.EnphaseAPIv4SystemsURL, systemID, endpoint)
	return baseURL + "?key=" + apiKey +
		"&start_at=" + strconv.FormatInt(dayStart.Unix(), 10) +
		"&end_at=" + strconv.FormatInt(dayEnd.Unix(), 10)
}
