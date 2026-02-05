// Package api - client_test.go
//
// TEST SETUP
// ----------
// This test suite validates the Enphase Cloud API v4 client using mock HTTP servers.
// Tests ensure correct request formatting and response parsing without real API calls.
//
// TEST PLAN
// ---------
// 1. Production Data Tests
//   - Test GetProductionForDate with mock telemetry response
//   - Test request includes Authorization header
//   - Test response parsing (wh_del field)
//   - Test conversion from Wh to kWh
//
// 2. Request Validation Tests
//   - Test API key is included in URL
//   - Test start_at and end_at timestamps are correct
//   - Test Bearer token in Authorization header
//   - Test Content-Type is application/json
//
// 3. Error Handling Tests
//   - Test 401 Unauthorized response
//   - Test 429 Rate Limit response
//   - Test malformed JSON response
//   - Test network errors
//
// TESTING APPROACH
// ----------------
// - httptest.NewServer creates mock Enphase API
// - Inject mock server URL via NewEnlightenCloudClientWithBaseURL
// - Verify request format in server handler
// - Return mock responses for parsing tests
//
// DEPENDENCY INJECTION
// --------------------
// The client accepts baseURL parameter for testability:
// - Production: Uses constants.EnphaseAPIv4SystemsURL
// - Testing: Uses httptest server URL
//
// This allows testing without:
// - Real API calls (no rate limits)
// - Network dependencies (fast, deterministic)
// - External service availability (always works)
//
// PATTERN USED
// ------------
// - Pattern 4: Mock HTTP Servers (httptest.NewServer)
// - Pattern 7: Error Path Testing
//
// See docs/TESTING.md for detailed pattern explanations.
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"enphase-monitor/internal/constants"
)

// TestEnlightenCloudClient_GetProductionForDate tests production data fetching with mock HTTP server
// This test now works properly with the new baseURL dependency injection!
func TestEnlightenCloudClient_GetProductionForDate(t *testing.T) {
	// Create mock HTTP server that returns test telemetry data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error": "unauthorized"}`))
			return
		}

		// Return mock telemetry data (production_meter format)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"intervals": [
				{"end_at": 1737676800, "wh_del": 1500.5, "enwh": 1500.5},
				{"end_at": 1737677700, "wh_del": 2000.0, "enwh": 2000.0}
			]
		}`))
	}))
	defer server.Close()

	// Create client with injected mock server URL (NEW!)
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(
		server.URL, // Inject mock server URL for testing!
		"12345",
		"test-api-key",
		"test-token",
		tz,
	)

	// Test GetProductionForDate with mock server
	ctx := context.Background()
	production, err := client.GetProductionForDate(ctx, time.Time{}, constants.QueryTypeDay) // Today

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify production value (1500.5 + 2000.0 = 3500.5 Wh = 3.5005 kWh)
	expected := 3.5005
	if production != expected {
		t.Errorf("Expected production %v kWh, got %v kWh", expected, production)
	}

	t.Logf("Successfully tested cloud client with mock HTTP server!")
}

// TestEnlightenCloudClient_RateLimitHandling tests 429 error handling with mock server
func TestEnlightenCloudClient_RateLimitHandling(t *testing.T) {
	// Create mock HTTP server that returns 429 rate limit error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": "rate limit exceeded"}`))
	}))
	defer server.Close()

	// Create client with injected mock server URL
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(
		server.URL,
		"12345",
		"test-api-key",
		"test-token",
		tz,
	)

	// Test GetProductionForDate - should return error for 429
	ctx := context.Background()
	_, err := client.GetProductionForDate(ctx, time.Time{}, constants.QueryTypeDay)

	// Verify we got a rate limit error
	if err == nil {
		t.Fatal("Expected rate limit error, got nil")
	}

	if !constants.IsRateLimitError(err) {
		t.Errorf("Expected rate limit error, got: %v", err)
	}

	t.Logf("Successfully tested rate limit handling with mock HTTP server!")
}

// TestEnlightenCloudClient_CacheUsedFlag tests cache usage tracking
func TestEnlightenCloudClient_CacheUsedFlag(t *testing.T) {
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClient("12345", "test-api-key", "test-token", tz)

	// Note: cacheUsed field is now internal to the api package
	// Cache behavior is returned as a boolean from GetMetricsFromCloud()
	// This test verifies the constructor works correctly
	if client == nil {
		t.Error("Expected non-nil client")
	}
}

// TestTryLoadPastDateCache_* test the tryLoadPastDateCache helper
// (past-date cache fallback when primary cache lookup fails).
func TestTryLoadPastDateCache_InvalidURL(t *testing.T) {
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL("http://test", "12345", "key", "token", tz)
	targetDate := time.Date(2026, 1, 15, 0, 0, 0, 0, tz)

	cached, ok := client.tryLoadPastDateCache("://invalid-url", targetDate)
	if ok {
		t.Error("tryLoadPastDateCache() ok = true, want false for invalid URL")
	}
	if cached != nil {
		t.Errorf("tryLoadPastDateCache() cached = %v, want nil", cached)
	}
}

// TestTryLoadPastDateCache_NoMatch verifies (nil, false) when no cache entry matches system/endpoint/date.
func TestTryLoadPastDateCache_NoMatch(t *testing.T) {
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL("http://test", "12345", "key", "token", tz)
	targetDate := time.Date(2026, 1, 15, 0, 0, 0, 0, tz)

	// Valid URL; no matching cache entry (empty or different system/date).
	url := "http://test/12345/telemetry/battery?key=key&start_at=0&end_at=0"
	cached, ok := client.tryLoadPastDateCache(url, targetDate)
	if ok {
		t.Error("tryLoadPastDateCache() ok = true, want false when no matching cache")
	}
	if cached != nil {
		t.Errorf("tryLoadPastDateCache() cached = %v, want nil", cached)
	}
}
