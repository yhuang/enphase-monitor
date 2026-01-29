package main

import (
	"fmt"
	"time"
)

// buildTelemetryURL constructs a URL for a telemetry endpoint with date range parameters.
// This centralizes URL construction logic and ensures consistent parameter formatting.
func buildTelemetryURL(systemID, endpoint, apiKey string, dayStart, dayEnd time.Time) string {
	baseURL := fmt.Sprintf("https://api.enphaseenergy.com/api/v4/systems/%s/%s", systemID, endpoint)
	return fmt.Sprintf("%s?key=%s&start_at=%d&end_at=%d",
		baseURL,
		apiKey,
		dayStart.Unix(),
		dayEnd.Unix(),
	)
}
