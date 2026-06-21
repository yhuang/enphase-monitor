// Package location resolves the geographic location of the configured Enphase
// systems from the Enphase Cloud API, so callers (e.g. weather lookups) don't
// need a hand-configured ZIP code.
//
// WHY THIS EXISTS
// ---------------
// The Enphase /api/v4/systems endpoint returns each system's mailing address
// (city/state/country/postal_code) and timezone, but NOT latitude/longitude.
// This package fetches that metadata, turns the postal_code into coordinates
// via the geocode package, and caches the result.
//
// CACHING AND THE API BUDGET
// --------------------------
// A system's address is effectively static, so the resolved metadata is cached
// to disk with a long TTL (see DefaultCacheTTL). On a warm cache the resolver
// makes ZERO network calls — neither to Enphase nor to the geocoder — which
// keeps it out of the per-run path and clear of the Enphase rate budget (the
// telemetry calls already sit at that limit). Only a cold or expired cache
// triggers a single /systems call, recorded against the budget for honesty.
package location

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/geocode"
)

// DefaultCacheTTL is how long resolved system locations stay fresh before the
// resolver refetches. A system's address is effectively static, so this is
// intentionally long: re-resolving means a /systems call that, on a live
// current-day report, competes with the per-minute telemetry budget.
const DefaultCacheTTL = 30 * 24 * time.Hour

// cacheFileName is the on-disk cache file, stored under the shared cache dir.
const cacheFileName = "systems_location.json"

// SystemLocation is the location of a single Enphase system, enriched with the
// coordinates geocoded from its postal code.
type SystemLocation struct {
	SystemID   int     `json:"system_id"`
	Name       string  `json:"name"`
	Timezone   string  `json:"timezone"`
	City       string  `json:"city"`
	State      string  `json:"state"`
	Country    string  `json:"country"`
	PostalCode string  `json:"postal_code"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
}

// Resolver fetches, caches, and geocodes Enphase system locations. The zero
// value is not ready for use; call NewResolver. Exported fields may be
// overridden after construction (tests inject a mock server, temp cache path,
// and a stub geocoder).
type Resolver struct {
	// BaseURL is the Enphase systems endpoint.
	BaseURL string
	// CacheTTL bounds how long a cached result is considered fresh.
	CacheTTL time.Duration
	// CachePath is the file the resolved locations are persisted to.
	CachePath string
	// HTTPClient performs the /systems request.
	HTTPClient *http.Client
	// Geocode converts a postal code to coordinates. Defaults to
	// geocode.LookupZip; overridden in tests to avoid network access.
	Geocode func(ctx context.Context, zip string) (geocode.Coordinates, error)
}

// NewResolver returns a Resolver wired with production defaults.
func NewResolver() *Resolver {
	return &Resolver{
		BaseURL:    constants.EnphaseAPIv4SystemsURL,
		CacheTTL:   DefaultCacheTTL,
		CachePath:  filepath.Join(cache.GetCacheDir(), cacheFileName),
		HTTPClient: &http.Client{Timeout: constants.APIRequestTimeout},
		Geocode:    geocode.LookupZip,
	}
}

// SystemLocations returns the location of every configured Enphase system,
// serving a fresh disk cache when available and otherwise fetching /systems
// once and geocoding each postal code.
func (r *Resolver) SystemLocations(ctx context.Context, apiKey, accessToken string) ([]SystemLocation, error) {
	if cached, ok := r.loadCache(); ok {
		return cached, nil
	}
	return r.resolve(ctx, apiKey, accessToken)
}

// RefreshSystemLocations always fetches /systems and re-geocodes, ignoring any
// cached value, then overwrites the cache. Used by a forced --init.
func (r *Resolver) RefreshSystemLocations(ctx context.Context, apiKey, accessToken string) ([]SystemLocation, error) {
	return r.resolve(ctx, apiKey, accessToken)
}

// resolve fetches /systems, geocodes each postal code, and caches the result.
func (r *Resolver) resolve(ctx context.Context, apiKey, accessToken string) ([]SystemLocation, error) {
	locs, err := r.fetchSystems(ctx, apiKey, accessToken)
	if err != nil {
		return nil, err
	}
	// The /systems fetch is a single live Enphase call; record it so the API
	// budget accounting stays accurate. Geocoding hits a separate service and
	// is not part of the Enphase budget.
	cache.RecordAPICall()

	if err := r.resolveCoordinates(ctx, locs); err != nil {
		return nil, err
	}

	r.saveCache(locs) // best effort; a cache write failure must not fail the lookup
	return locs, nil
}

// CachedSystemLocations returns the resolved system locations from the disk
// cache without any network access, reporting ok=false when the cache is
// missing or expired (i.e. --init has not been run, or the cache was cleared).
// Report paths use this so location resolution never makes a /systems call that
// would compete with the per-minute telemetry budget.
func (r *Resolver) CachedSystemLocations() ([]SystemLocation, bool) {
	return r.loadCache()
}

// CachedPrimaryCoordinates returns the first configured system's coordinates
// from the cache only (the common case where all systems share a site). ok is
// false when the location cache is unavailable.
func (r *Resolver) CachedPrimaryCoordinates() (geocode.Coordinates, bool) {
	locs, ok := r.loadCache()
	if !ok || len(locs) == 0 {
		return geocode.Coordinates{}, false
	}
	return geocode.Coordinates{Latitude: locs[0].Latitude, Longitude: locs[0].Longitude}, true
}

// systemsResponse mirrors the subset of the /api/v4/systems payload we need.
type systemsResponse struct {
	Systems []struct {
		SystemID int    `json:"system_id"`
		Name     string `json:"name"`
		Timezone string `json:"timezone"`
		Address  struct {
			City       string `json:"city"`
			State      string `json:"state"`
			Country    string `json:"country"`
			PostalCode string `json:"postal_code"`
		} `json:"address"`
	} `json:"systems"`
}

// fetchSystems calls /api/v4/systems and maps the response to SystemLocations
// (without coordinates, which resolveCoordinates fills in).
func (r *Resolver) fetchSystems(ctx context.Context, apiKey, accessToken string) ([]SystemLocation, error) {
	endpoint := r.BaseURL + "?" + url.Values{"key": {apiKey}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create systems request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch systems: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("systems request failed with status %d", resp.StatusCode)
	}

	var parsed systemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode systems response: %w", err)
	}
	if len(parsed.Systems) == 0 {
		return nil, errors.New("Enphase API returned no systems")
	}

	locs := make([]SystemLocation, len(parsed.Systems))
	for i, s := range parsed.Systems {
		locs[i] = SystemLocation{
			SystemID:   s.SystemID,
			Name:       s.Name,
			Timezone:   s.Timezone,
			City:       s.Address.City,
			State:      s.Address.State,
			Country:    s.Address.Country,
			PostalCode: s.Address.PostalCode,
		}
	}
	return locs, nil
}

// resolveCoordinates geocodes each system's postal code in place.
func (r *Resolver) resolveCoordinates(ctx context.Context, locs []SystemLocation) error {
	for i := range locs {
		if locs[i].PostalCode == "" {
			return fmt.Errorf("system %d (%s) has no postal_code to geocode", locs[i].SystemID, locs[i].Name)
		}
		coords, err := r.Geocode(ctx, locs[i].PostalCode)
		if err != nil {
			return fmt.Errorf("geocode system %d (%s): %w", locs[i].SystemID, locs[i].Name, err)
		}
		locs[i].Latitude = coords.Latitude
		locs[i].Longitude = coords.Longitude
	}
	return nil
}

// cacheFile is the on-disk cache envelope.
type cacheFile struct {
	FetchedAt time.Time        `json:"fetched_at"`
	Systems   []SystemLocation `json:"systems"`
}

// loadCache returns the cached locations when the file exists and is within TTL.
func (r *Resolver) loadCache() ([]SystemLocation, bool) {
	data, err := os.ReadFile(r.CachePath)
	if err != nil {
		return nil, false
	}
	var cf cacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, false
	}
	if time.Since(cf.FetchedAt) > r.CacheTTL {
		return nil, false
	}
	return cf.Systems, true
}

// saveCache persists the resolved locations, pretty-printed. Best effort.
func (r *Resolver) saveCache(locs []SystemLocation) {
	data, err := json.MarshalIndent(cacheFile{FetchedAt: time.Now(), Systems: locs}, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(r.CachePath), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(r.CachePath, append(data, '\n'), 0o644)
}
