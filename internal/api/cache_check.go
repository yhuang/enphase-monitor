package api

import (
	"fmt"
	"time"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/timezone"
)

// EndpointStatus reports the cache state for one API endpoint of one system.
type EndpointStatus struct {
	Endpoint string
	Present  bool
	Age      time.Duration // valid only when Present is true
	Required bool          // false for battery (optional — system may not have one)
}

// SystemCacheStatus aggregates cache coverage across all endpoints for one system.
type SystemCacheStatus struct {
	SystemID   string
	SystemName string
	Endpoints  []EndpointStatus
}

// AllRequiredPresent returns true when every Required endpoint has a cache entry.
func (s SystemCacheStatus) AllRequiredPresent() bool {
	for _, e := range s.Endpoints {
		if e.Required && !e.Present {
			return false
		}
	}
	return true
}

// CheckCacheForSystem checks whether each API endpoint needed for the given query
// is available in the on-disk cache. No network requests are made.
//
// The URLs it builds are identical to those GetMetricsFromCloud would request, so
// a hit here guarantees that running in test mode will succeed without API calls.
func CheckCacheForSystem(systemID, systemName, apiKey string, testDate time.Time, queryType constants.QueryType, tz *time.Location) SystemCacheStatus {
	status := SystemCacheStatus{
		SystemID:   systemID,
		SystemName: systemName,
	}

	type ep struct {
		name     string
		url      string
		required bool
	}

	base := constants.EnphaseAPIv4SystemsURL
	var eps []ep

	if queryType == constants.QueryTypeDay {
		periodStart, periodEnd := timezone.GetBoundaries(testDate, queryType, tz)
		build := func(name string, required bool) ep {
			return ep{
				name:     name,
				url:      fmt.Sprintf("%s/%s/%s?key=%s&start_at=%d&end_at=%d", base, systemID, name, apiKey, periodStart.Unix(), periodEnd.Unix()),
				required: required,
			}
		}
		eps = []ep{
			build("energy_import_telemetry", true),
			build("energy_export_telemetry", true),
			build("telemetry/production_meter", true),
			build("telemetry/consumption_meter", true),
			build("telemetry/battery", false),
		}
	} else {
		// month / year / true-up → lifetime endpoints
		periodStart, _ := timezone.GetBoundaries(testDate, queryType, tz)
		startDate := periodStart.Format(constants.DateFormat)
		build := func(name string, required bool) ep {
			return ep{
				name:     name,
				url:      fmt.Sprintf("%s/%s/%s?key=%s&start_date=%s", base, systemID, name, apiKey, startDate),
				required: required,
			}
		}
		eps = []ep{
			build("energy_import_lifetime", true),
			build("energy_export_lifetime", true),
			build("energy_lifetime", true),
			build("consumption_lifetime", true),
			build("battery_lifetime", false),
		}
	}

	for _, e := range eps {
		cached, err := cache.LoadCachedResponse(e.url, tz)
		if err != nil {
			status.Endpoints = append(status.Endpoints, EndpointStatus{
				Endpoint: e.name,
				Present:  false,
				Required: e.required,
			})
		} else {
			status.Endpoints = append(status.Endpoints, EndpointStatus{
				Endpoint: e.name,
				Present:  true,
				Age:      time.Since(cached.CachedAt),
				Required: e.required,
			})
		}
	}

	return status
}
