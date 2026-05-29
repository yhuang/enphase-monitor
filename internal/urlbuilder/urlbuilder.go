// Package urlbuilder provides utility functions for constructing Enphase API URLs.
//
// PURPOSE
// -------
// Centralizes URL building so that every caller produces identically formatted
// URLs — consistent parameter ordering, Unix-timestamp date ranges, and injected
// API keys — which in turn ensures cache hits are not missed due to URL variance.
// The api package (live requests) and api/cache_check.go (preflight cache probe)
// both build their URLs here so the two can never drift apart.
//
// KEY FEATURES
// ------------
//   - Interval Data (Day Mode) URLs with Unix-timestamp date ranges
//   - Lifetime Data URLs with a YYYY-MM-DD start_date
//   - API key injection
//   - Caller-supplied base URL (injectable for testing)
package urlbuilder

import (
	"fmt"
	"strconv"
	"time"
)

// BuildTelemetryURL constructs an Interval Data (Day Mode) request URL for an endpoint.
// base is the API systems base URL (e.g. constants.EnphaseAPIv4SystemsURL), passed in
// so callers can inject a test server URL.
func BuildTelemetryURL(base, systemID, endpoint, apiKey string, dayStart, dayEnd time.Time) string {
	return fmt.Sprintf("%s/%s/%s", base, systemID, endpoint) +
		"?key=" + apiKey +
		"&start_at=" + strconv.FormatInt(dayStart.Unix(), 10) +
		"&end_at=" + strconv.FormatInt(dayEnd.Unix(), 10)
}

// BuildLifetimeURL constructs a Lifetime Data request URL for an endpoint.
// base is the API systems base URL (injectable for testing); startDate is a
// YYYY-MM-DD string.
func BuildLifetimeURL(base, systemID, endpoint, apiKey, startDate string) string {
	return fmt.Sprintf("%s/%s/%s?key=%s&start_date=%s", base, systemID, endpoint, apiKey, startDate)
}
