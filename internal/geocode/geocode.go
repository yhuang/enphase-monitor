// Package geocode resolves US ZIP codes to geographic coordinates.
//
// PURPOSE
// -------
// The weather query (daily temperature range from Open-Meteo) is addressed by
// latitude/longitude, but the user configures their location as a ZIP code.
// This package bridges that gap by looking up coordinates for a ZIP code.
//
// DATA SOURCE
// -----------
// Lookups use Zippopotam.us (https://api.zippopotam.us), a free service that
// requires no API key or signup. It is queried at most once per run to resolve
// the configured ZIP, so it does not interact with the Enphase API Budget.
//
// LEADING ZEROS
// -------------
// ZIP codes are strings, not numbers: codes like "01234" (Massachusetts) have a
// leading zero that an integer would drop. Callers must pass the ZIP as a string
// and, in config.yaml, quote it (zip: "01234").
package geocode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// zippopotamBaseURL is the US lookup endpoint; the ZIP code is appended as a
// path segment. It is a var (not a const) so tests can redirect it to a local
// httptest server.
var zippopotamBaseURL = "https://api.zippopotam.us/us/"

// lookupTimeout bounds a single geocoding request. The lookup is a one-time,
// non-critical enrichment step, so it fails fast rather than stalling startup.
const lookupTimeout = 10 * time.Second

// Coordinates is a geographic point in decimal degrees.
type Coordinates struct {
	Latitude  float64
	Longitude float64
}

// geocodeHTTPClient is reused across lookups to enable connection reuse.
var geocodeHTTPClient = &http.Client{
	Timeout: lookupTimeout,
}

// zippopotamResponse mirrors the subset of the Zippopotam.us payload we need.
// Latitude and longitude arrive as JSON strings, not numbers.
type zippopotamResponse struct {
	PostCode string `json:"post code"`
	Places   []struct {
		PlaceName string `json:"place name"`
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
	} `json:"places"`
}

// LookupZip resolves a US ZIP code to its coordinates via Zippopotam.us.
//
// The ZIP must be a 5-digit US code (passed as a string to preserve leading
// zeros). An unknown ZIP returns a 404 from the service, surfaced here as an
// error.
func LookupZip(ctx context.Context, zip string) (Coordinates, error) {
	zip = strings.TrimSpace(zip)
	if zip == "" {
		return Coordinates{}, errors.New("zip code is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zippopotamBaseURL+url.PathEscape(zip), nil)
	if err != nil {
		return Coordinates{}, fmt.Errorf("failed to create geocode request: %w", err)
	}

	resp, err := geocodeHTTPClient.Do(req)
	if err != nil {
		return Coordinates{}, fmt.Errorf("failed to look up zip %q: %w", zip, err)
	}
	defer resp.Body.Close()

	// Zippopotam returns 404 for an unrecognized ZIP code.
	if resp.StatusCode == http.StatusNotFound {
		return Coordinates{}, fmt.Errorf("zip code %q not found", zip)
	}
	if resp.StatusCode != http.StatusOK {
		return Coordinates{}, fmt.Errorf("geocode lookup for zip %q failed with status %d", zip, resp.StatusCode)
	}

	var parsed zippopotamResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Coordinates{}, fmt.Errorf("failed to decode geocode response for zip %q: %w", zip, err)
	}
	if len(parsed.Places) == 0 {
		return Coordinates{}, fmt.Errorf("no location found for zip %q", zip)
	}

	place := parsed.Places[0]
	lat, err := strconv.ParseFloat(strings.TrimSpace(place.Latitude), 64)
	if err != nil {
		return Coordinates{}, fmt.Errorf("invalid latitude %q for zip %q: %w", place.Latitude, zip, err)
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(place.Longitude), 64)
	if err != nil {
		return Coordinates{}, fmt.Errorf("invalid longitude %q for zip %q: %w", place.Longitude, zip, err)
	}

	return Coordinates{Latitude: lat, Longitude: lon}, nil
}
