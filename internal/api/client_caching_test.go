package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
)

// These are characterization tests: they pin down the CURRENT behavior of
// makeCachedAPIRequest's fallback branches (validation mode, no-cache mode,
// and the 429/503/network-error → cache fallbacks in the normal path) so a
// refactor of that function can be verified to preserve behavior. They are
// intentionally driven through the public Get*ForDate API.
//
// The key technique is switchableServer: a single mock server (one stable
// base URL, hence one stable cache key) whose status code can be flipped
// between calls. That lets a test populate the cache with a 200 response and
// then force a 429/503 against the *same* URL, reaching the with-cache
// fallback branches that separate-server tests cannot.

// prodOKBody is a production_meter day response summing to 1.5005 kWh.
const prodOKBody = `{"intervals":[{"end_at":1737676800,"wh_del":1500.5,"enwh":1500.5}]}`

const prodOKValueKWh = 1.5005

// switchableServer is a mock Enphase server whose HTTP status can be changed
// between requests. Status 200 returns prodOKBody; any other status returns a
// short error body. The status is held in an atomic so the change made on the
// test goroutine is visible to the server goroutine.
type switchableServer struct {
	*httptest.Server
	status atomic.Int32
}

func newSwitchableServer() *switchableServer {
	s := &switchableServer{}
	s.status.Store(http.StatusOK)
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := int(s.status.Load())
		if code == http.StatusOK {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(prodOKBody))
			return
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(`{"error":"forced status"}`))
	}))
	return s
}

func (s *switchableServer) setStatus(code int) { s.status.Store(int32(code)) }

// newProdClient builds a client pointed at the given base URL for production
// day queries (which exercise the Interval Data path through makeCachedAPIRequest).
func newProdClient(t *testing.T, baseURL string) *EnlightenCloudClient {
	t.Helper()
	tz := mustLoadLocation(t, "US/Pacific")
	return NewEnlightenCloudClientWithBaseURL(baseURL, "12345", "test-key", "test-token", tz)
}

// fetchToday issues a today (current-period) production day query.
func fetchToday(client *EnlightenCloudClient) (float64, error) {
	return client.GetProductionForDate(context.Background(), time.Time{}, constants.QueryModeDay)
}

// --- Validation mode ----------------------------------------------------------

// TestMakeCachedAPIRequest_ValidationMode_CacheHit: with validation mode on and
// a populated cache, the request is served from cache (no live call).
func TestMakeCachedAPIRequest_ValidationMode_CacheHit(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv := newSwitchableServer()
	defer srv.Close()
	client := newProdClient(t, srv.URL)

	// Populate cache with a live 200 response.
	if _, err := fetchToday(client); err != nil {
		t.Fatalf("populate: unexpected error: %v", err)
	}

	// Turn on validation mode and point the server at 500 to prove it is never called.
	cache.SetValidationMode(true)
	defer cache.SetValidationMode(false)
	srv.setStatus(http.StatusInternalServerError)

	got, err := fetchToday(client)
	if err != nil {
		t.Fatalf("validation-mode cache hit: unexpected error: %v", err)
	}
	if got != prodOKValueKWh {
		t.Errorf("validation-mode cache hit: got %v kWh, want %v", got, prodOKValueKWh)
	}
}

// TestMakeCachedAPIRequest_ValidationMode_CacheMiss: with validation mode on and
// no cache, a descriptive error is returned and no live call is made.
func TestMakeCachedAPIRequest_ValidationMode_CacheMiss(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv := newSwitchableServer()
	defer srv.Close()
	client := newProdClient(t, srv.URL)

	cache.SetValidationMode(true)
	defer cache.SetValidationMode(false)

	_, err := fetchToday(client)
	if err == nil {
		t.Fatal("validation-mode cache miss: want error, got nil")
	}
	if !strings.Contains(err.Error(), "validation mode") {
		t.Errorf("validation-mode cache miss: error = %v, want it to mention validation mode", err)
	}
}

// --- No-cache mode (--no-cache) -----------------------------------------------

// TestMakeCachedAPIRequest_NoCache_429_RateLimitError: with cache disabled, a 429
// surfaces as a rate-limit error even when a cached response exists — the
// no-cache path never serves stale cache, so the aggregator can fail over to a
// spare credential (and Backfill Mode fails the day) instead of returning stale
// data.
func TestMakeCachedAPIRequest_NoCache_429_RateLimitError(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()
	cache.SetCacheDisabled(true)
	defer cache.SetCacheDisabled(false)

	srv := newSwitchableServer()
	defer srv.Close()
	client := newProdClient(t, srv.URL)

	// Populate the cache first; it must NOT be served on the subsequent 429.
	if _, err := fetchToday(client); err != nil {
		t.Fatalf("populate: unexpected error: %v", err)
	}

	srv.setStatus(http.StatusTooManyRequests)
	if _, err := fetchToday(client); err == nil || !constants.IsRateLimitError(err) {
		t.Errorf("no-cache 429 with cache present: error = %v, want rate-limit error", err)
	}
}

// TestMakeCachedAPIRequest_NoCache_429_NoCache_RateLimitError: with cache disabled
// and no saved response, a 429 surfaces as a rate-limit error.
func TestMakeCachedAPIRequest_NoCache_429_NoCache_RateLimitError(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()
	cache.SetCacheDisabled(true)
	defer cache.SetCacheDisabled(false)

	srv := newSwitchableServer()
	defer srv.Close()
	srv.setStatus(http.StatusTooManyRequests)
	client := newProdClient(t, srv.URL)

	_, err := fetchToday(client)
	if err == nil {
		t.Fatal("no-cache 429 no cache: want error, got nil")
	}
	if !constants.IsRateLimitError(err) {
		t.Errorf("no-cache 429 no cache: error = %v, want rate-limit error", err)
	}
}

// TestMakeCachedAPIRequest_NoCache_503_Error: with cache disabled, a 503 surfaces
// as an error even when a cached response exists — the no-cache path never serves
// stale cache.
func TestMakeCachedAPIRequest_NoCache_503_Error(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()
	cache.SetCacheDisabled(true)
	defer cache.SetCacheDisabled(false)

	srv := newSwitchableServer()
	defer srv.Close()
	client := newProdClient(t, srv.URL)

	if _, err := fetchToday(client); err != nil {
		t.Fatalf("populate: unexpected error: %v", err)
	}

	srv.setStatus(http.StatusServiceUnavailable)
	if _, err := fetchToday(client); err == nil {
		t.Error("no-cache 503 with cache present: want error, got nil")
	}
}

// TestMakeCachedAPIRequest_NoCache_503_NoCache_Error: with cache disabled and no
// saved response, a 503 surfaces as an error mentioning the 503 status.
func TestMakeCachedAPIRequest_NoCache_503_NoCache_Error(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()
	cache.SetCacheDisabled(true)
	defer cache.SetCacheDisabled(false)

	srv := newSwitchableServer()
	defer srv.Close()
	srv.setStatus(http.StatusServiceUnavailable)
	client := newProdClient(t, srv.URL)

	_, err := fetchToday(client)
	if err == nil {
		t.Fatal("no-cache 503 no cache: want error, got nil")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("no-cache 503 no cache: error = %v, want it to mention 503", err)
	}
}

// --- Normal mode fallbacks ----------------------------------------------------

// TestMakeCachedAPIRequest_429_PropagatesError: a current-period query that 429s
// surfaces a rate-limit error even when a fresh cache exists — the live path no
// longer serves stale cache on error, so the aggregator can fail over to a spare
// credential instead.
func TestMakeCachedAPIRequest_429_PropagatesError(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv := newSwitchableServer()
	defer srv.Close()
	client := newProdClient(t, srv.URL)

	if _, err := fetchToday(client); err != nil {
		t.Fatalf("populate: unexpected error: %v", err)
	}

	srv.setStatus(http.StatusTooManyRequests)
	if _, err := fetchToday(client); err == nil || !constants.IsRateLimitError(err) {
		t.Errorf("429 with cache present: error = %v, want rate-limit error", err)
	}
}

// TestMakeCachedAPIRequest_503_PropagatesError: a current-period query that 503s
// surfaces an error even when a fresh cache exists (no stale-cache fallback).
func TestMakeCachedAPIRequest_503_PropagatesError(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv := newSwitchableServer()
	defer srv.Close()
	client := newProdClient(t, srv.URL)

	if _, err := fetchToday(client); err != nil {
		t.Fatalf("populate: unexpected error: %v", err)
	}

	srv.setStatus(http.StatusServiceUnavailable)
	if _, err := fetchToday(client); err == nil || !strings.Contains(err.Error(), "503") {
		t.Errorf("503 with cache present: error = %v, want it to mention 503", err)
	}
}

// TestMakeCachedAPIRequest_503_NoCache_Error: a current-period query that 503s
// with no cache available surfaces an error mentioning the 503 status.
func TestMakeCachedAPIRequest_503_NoCache_Error(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv := newSwitchableServer()
	defer srv.Close()
	srv.setStatus(http.StatusServiceUnavailable)
	client := newProdClient(t, srv.URL)

	_, err := fetchToday(client)
	if err == nil {
		t.Fatal("503 no cache: want error, got nil")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("503 no cache: error = %v, want it to mention 503", err)
	}
}

// TestMakeCachedAPIRequest_NetworkError_PropagatesError: when the live call fails
// at the transport level (server closed), the error propagates even with a fresh
// cache present — the live path no longer masks failures with stale cache.
func TestMakeCachedAPIRequest_NetworkError_PropagatesError(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv := newSwitchableServer()
	client := newProdClient(t, srv.URL)

	if _, err := fetchToday(client); err != nil {
		t.Fatalf("populate: unexpected error: %v", err)
	}

	// Close the server so the next request fails at the transport level.
	srv.Close()
	if _, err := fetchToday(client); err == nil {
		t.Error("network error with cache present: want error, got nil")
	}
}
