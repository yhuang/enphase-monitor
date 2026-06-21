// Tests for LookupZip, in two groups:
//
//   - Unit tests drive LookupZip with hand-crafted httptest responses to cover
//     request construction and the error/edge branches (empty input, 404, empty
//     places, malformed coordinates).
//   - The fixture test replays real Zippopotam responses recorded under
//     test-data/ and asserts the parsed coordinates, so the happy path is
//     verified against real-world data with no network access.
//
// The tests never hit the network. To refresh the recorded fixtures from the
// live Zippopotam service (rarely needed — ZIP centroids are stable):
//
//	for z in 94566 90069 10001 97201 60601; do
//	  curl -s https://api.zippopotam.us/us/$z | jq . > test-data/zippopotam_api_$z.json
//	done
package geocode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// withBaseURL points zippopotamBaseURL at a test server for the duration of a
// test and restores the original afterward.
func withBaseURL(t *testing.T, url string) {
	t.Helper()
	original := zippopotamBaseURL
	zippopotamBaseURL = url
	t.Cleanup(func() { zippopotamBaseURL = original })
}

// --- Unit tests: request construction and error handling ---

// TestLookupZip_RequestPath verifies the ZIP is appended to the endpoint as a
// path segment — the one piece of request construction the fixture test (whose
// server ignores the path) does not exercise.
func TestLookupZip_RequestPath(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"places": [{"latitude": "37.6", "longitude": "-121.9"}]}`))
	}))
	defer server.Close()
	withBaseURL(t, server.URL+"/")

	if _, err := LookupZip(context.Background(), "94566"); err != nil {
		t.Fatalf("LookupZip returned error: %v", err)
	}
	if want := "/94566"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
}

func TestLookupZip_EmptyZip(t *testing.T) {
	if _, err := LookupZip(context.Background(), "   "); err == nil {
		t.Fatal("expected error for empty zip, got nil")
	}
}

func TestLookupZip_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	withBaseURL(t, server.URL+"/")

	_, err := LookupZip(context.Background(), "00000")
	if err == nil {
		t.Fatal("expected error for unknown zip, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "not found")
	}
}

func TestLookupZip_NoPlaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"post code": "94566", "places": []}`))
	}))
	defer server.Close()
	withBaseURL(t, server.URL+"/")

	if _, err := LookupZip(context.Background(), "94566"); err == nil {
		t.Fatal("expected error when no places returned, got nil")
	}
}

func TestLookupZip_InvalidCoordinate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"places": [{"latitude": "not-a-number", "longitude": "-121.8746"}]
		}`))
	}))
	defer server.Close()
	withBaseURL(t, server.URL+"/")

	if _, err := LookupZip(context.Background(), "94566"); err == nil {
		t.Fatal("expected error for non-numeric latitude, got nil")
	}
}

// --- Fixture test: parsing real recorded responses, offline ---

// geocodeCase is one city: its ZIP and the coordinates LookupZip should return
// from the recorded fixture.
type geocodeCase struct {
	name string
	zip  string
	lat  float64
	lon  float64
}

var geocodeCases = []geocodeCase{
	{"Pleasanton CA", "94566", 37.6658, -121.8755},
	{"West Hollywood CA", "90069", 34.0906, -118.3788},
	{"New York NY", "10001", 40.7484, -73.9967},
	{"Portland OR", "97201", 45.5078, -122.6897},
	{"Chicago IL", "60601", 41.8858, -87.6181},
}

// withinUSBounds is a cheap plausibility filter: coordinates for a US ZIP must
// fall in the rough bounding box of US territory. Catches a sign flip or
// swapped lat/lon.
func withinUSBounds(c Coordinates) bool {
	return c.Latitude >= 18 && c.Latitude <= 72 &&
		c.Longitude >= -180 && c.Longitude <= -66
}

// testProjectRoot resolves the repo root from this file (internal/geocode/ is
// two dirs down) so the fixtures can live in the shared test-data/ directory.
func testProjectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Join(filepath.Dir(filename), "../..")
}

// forwardFixture names the recorded Zippopotam response for a ZIP. The geocode_
// prefix groups it within test-data/ alongside the enphase_api_*.json files.
func forwardFixture(zip string) string { return "zippopotam_api_" + zip + ".json" }

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(testProjectRoot(t), "test-data", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

// staticHandler serves a fixed JSON body for any request.
func staticHandler(body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}

// TestLookupZip_FixtureCoordinates replays each recorded Zippopotam response and
// asserts LookupZip extracts the expected coordinates — fully offline.
func TestLookupZip_FixtureCoordinates(t *testing.T) {
	for _, tc := range geocodeCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(staticHandler(readFixture(t, forwardFixture(tc.zip))))
			defer server.Close()
			withBaseURL(t, server.URL+"/")

			coords, err := LookupZip(context.Background(), tc.zip)
			if err != nil {
				t.Fatalf("LookupZip(%s): %v", tc.zip, err)
			}
			if coords.Latitude != tc.lat || coords.Longitude != tc.lon {
				t.Errorf("coords = %+v, want {%v %v}", coords, tc.lat, tc.lon)
			}
			if !withinUSBounds(coords) {
				t.Errorf("coords %+v are outside plausible US bounds", coords)
			}
		})
	}
}
