package location

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"enphase-monitor/internal/geocode"
)

// testProjectRoot resolves the repo root from this file (internal/location/ is
// two dirs down) so tests can read the shared test-data/ fixtures.
func testProjectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Join(filepath.Dir(filename), "../..")
}

// stubGeocoder returns deterministic coordinates without any network access.
func stubGeocoder(_ context.Context, zip string) (geocode.Coordinates, error) {
	if zip == "94566" {
		return geocode.Coordinates{Latitude: 37.6658, Longitude: -121.8755}, nil
	}
	return geocode.Coordinates{Latitude: 1, Longitude: 2}, nil
}

// newTestResolver wires a resolver to a fixture server, a temp cache file, and
// the stub geocoder. It returns the resolver and a pointer to the server hit
// counter so tests can assert when the network was (not) used.
func newTestResolver(t *testing.T) (*Resolver, *int) {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join(testProjectRoot(t), "test-data", "enphase_systems.json"))
	if err != nil {
		t.Fatalf("read systems fixture: %v", err)
	}

	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Query().Get("key") == "" {
			t.Errorf("request missing key query param: %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer test-token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(server.Close)

	r := &Resolver{
		BaseURL:    server.URL,
		CacheTTL:   time.Hour,
		CachePath:  filepath.Join(t.TempDir(), cacheFileName),
		HTTPClient: server.Client(),
		Geocode:    stubGeocoder,
	}
	return r, &hits
}

func TestSystemLocations_FetchAndGeocode(t *testing.T) {
	r, hits := newTestResolver(t)

	locs, err := r.SystemLocations(context.Background(), "test-key", "test-token", nil, "")
	if err != nil {
		t.Fatalf("SystemLocations: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("got %d systems, want 2", len(locs))
	}
	first := locs[0]
	if first.SystemID != 5392556 {
		t.Errorf("SystemID = %d, want 5392556", first.SystemID)
	}
	if first.PostalCode != "94566" {
		t.Errorf("PostalCode = %q, want 94566", first.PostalCode)
	}
	if first.Timezone != "US/Pacific" {
		t.Errorf("Timezone = %q, want US/Pacific", first.Timezone)
	}
	if first.Latitude != 37.6658 || first.Longitude != -121.8755 {
		t.Errorf("coords = (%v, %v), want (37.6658, -121.8755)", first.Latitude, first.Longitude)
	}
	if *hits != 1 {
		t.Errorf("server hit %d times on first call, want 1", *hits)
	}
}

func TestSystemLocations_WarmCacheSkipsNetwork(t *testing.T) {
	r, hits := newTestResolver(t)

	if _, err := r.SystemLocations(context.Background(), "test-key", "test-token", nil, ""); err != nil {
		t.Fatalf("first SystemLocations: %v", err)
	}
	// Second call should be served entirely from the disk cache.
	locs, err := r.SystemLocations(context.Background(), "test-key", "test-token", nil, "")
	if err != nil {
		t.Fatalf("second SystemLocations: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("cached result has %d systems, want 2", len(locs))
	}
	if *hits != 1 {
		t.Errorf("server hit %d times across two calls, want 1 (cache should serve the second)", *hits)
	}
}

func TestSystemLocations_ExpiredCacheRefetches(t *testing.T) {
	r, hits := newTestResolver(t)
	r.CacheTTL = -time.Second // force every cached entry to be considered stale

	if _, err := r.SystemLocations(context.Background(), "test-key", "test-token", nil, ""); err != nil {
		t.Fatalf("first SystemLocations: %v", err)
	}
	if _, err := r.SystemLocations(context.Background(), "test-key", "test-token", nil, ""); err != nil {
		t.Fatalf("second SystemLocations: %v", err)
	}
	if *hits != 2 {
		t.Errorf("server hit %d times, want 2 (expired cache should refetch)", *hits)
	}
}

func TestCachedPrimaryCoordinates_MissBeforeResolve(t *testing.T) {
	r, _ := newTestResolver(t)

	if _, ok := r.CachedPrimaryCoordinates(); ok {
		t.Error("CachedPrimaryCoordinates should report ok=false before any resolve")
	}
}

func TestCachedPrimaryCoordinates_HitAfterResolve(t *testing.T) {
	r, _ := newTestResolver(t)

	// Resolve once (populates the cache via the /systems fetch).
	if _, err := r.SystemLocations(context.Background(), "test-key", "test-token", nil, ""); err != nil {
		t.Fatalf("SystemLocations: %v", err)
	}
	// The report path reads cache-only, with no network access.
	coords, ok := r.CachedPrimaryCoordinates()
	if !ok {
		t.Fatal("CachedPrimaryCoordinates should report ok=true after resolve")
	}
	if coords.Latitude != 37.6658 || coords.Longitude != -121.8755 {
		t.Errorf("coords = %+v, want {37.6658 -121.8755}", coords)
	}
}

func TestRefreshSystemLocations_BypassesCache(t *testing.T) {
	r, hits := newTestResolver(t)

	// Warm the cache.
	if _, err := r.SystemLocations(context.Background(), "test-key", "test-token", nil, ""); err != nil {
		t.Fatalf("SystemLocations: %v", err)
	}
	// A normal call is served from cache (no second hit)...
	if _, err := r.SystemLocations(context.Background(), "test-key", "test-token", nil, ""); err != nil {
		t.Fatalf("cached SystemLocations: %v", err)
	}
	if *hits != 1 {
		t.Fatalf("server hit %d times, want 1 before refresh", *hits)
	}
	// ...but a forced refresh ignores the cache and refetches.
	if _, err := r.RefreshSystemLocations(context.Background(), "test-key", "test-token", nil, ""); err != nil {
		t.Fatalf("RefreshSystemLocations: %v", err)
	}
	if *hits != 2 {
		t.Errorf("server hit %d times, want 2 (refresh should bypass cache)", *hits)
	}
}

func TestSystemLocations_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	r := &Resolver{
		BaseURL:    server.URL,
		CacheTTL:   time.Hour,
		CachePath:  filepath.Join(t.TempDir(), cacheFileName),
		HTTPClient: server.Client(),
		Geocode:    stubGeocoder,
	}
	if _, err := r.SystemLocations(context.Background(), "k", "t", nil, ""); err == nil {
		t.Fatal("expected error on 401, got nil")
	}
}
