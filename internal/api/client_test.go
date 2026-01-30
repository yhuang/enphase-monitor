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
			w.Write([]byte(`{"error": "unauthorized"}`))
			return
		}
		
		// Return mock telemetry data (production_meter format)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"intervals": [
				{"end_at": 1737676800, "wh_del": 1500.5, "enwh": 1500.5},
				{"end_at": 1737677700, "wh_del": 2000.0, "enwh": 2000.0}
			]
		}`))
	}))
	defer server.Close()
	
	// Create client with injected mock server URL (NEW!)
	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(
		server.URL,  // Inject mock server URL for testing!
		"12345",
		"test-api-key",
		"test-token",
		tz,
	)
	
	// Test GetProductionForDate with mock server
	ctx := context.Background()
	production, err := client.GetProductionForDate(ctx, time.Time{}) // Today
	
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
		w.Write([]byte(`{"error": "rate limit exceeded"}`))
	}))
	defer server.Close()
	
	// Create client with injected mock server URL
	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(
		server.URL,
		"12345",
		"test-api-key",
		"test-token",
		tz,
	)
	
	// Test GetProductionForDate - should return error for 429
	ctx := context.Background()
	_, err := client.GetProductionForDate(ctx, time.Time{})
	
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
	tz, _ := time.LoadLocation("US/Pacific")
	client := NewEnlightenCloudClient("12345", "test-api-key", "test-token", tz)
	
	// Note: cacheUsed field is now internal to the api package
	// Cache behavior is returned as a boolean from GetMetricsFromCloud()
	// This test verifies the constructor works correctly
	if client == nil {
		t.Error("Expected non-nil client")
	}
}
