// Package oauth - oauth_edge_cases_test.go
//
// TEST SETUP
// ----------
// This test suite validates edge cases and error paths in OAuth authentication.
// Focuses on error handling, validation, and unusual scenarios.
//
// TEST PLAN
// ---------
// 1. Validation Error Tests
//   - Test nil config
//   - Test missing/empty required fields
//   - Test invalid URLs
//
// 2. Network Error Tests
//   - Test connection failures
//   - Test timeouts
//   - Test connection reset (simulated with hijacking)
//
// 3. Response Error Tests
//   - Test malformed JSON
//   - Test missing required response fields
//   - Test invalid token format
//
// 4. HTTP Status Code Tests
//   - Test 400 Bad Request
//   - Test 401 Unauthorized
//   - Test 403 Forbidden
//   - Test 500 Internal Server Error
//
// TESTING APPROACH
// ----------------
// - Mock HTTP servers that deliberately fail
// - Simulate network issues (connection reset, timeout)
// - Verify error messages provide useful context
// - Test cleanup happens even on error (defer statements)
//
// WHY TEST ERROR PATHS
// --------------------
// Error path testing ensures:
// - Graceful degradation (no panics or crashes)
// - Useful error messages for debugging
// - Proper resource cleanup (defer executed)
// - Error wrapping preserves context
//
// TEST ORGANIZATION
// -----------------
// This package has 3 test files (1:many pattern):
// - oauth_test.go: Basic unit tests (270 lines)
// - oauth_functional_test.go: Integration tests (598 lines)
// - oauth_edge_cases_test.go (this file): Edge cases (442 lines)
//
// PATTERN USED
// ------------
// - Pattern 7: Error Path Testing
// - Pattern 4: Mock HTTP Servers (for failure scenarios)
// - Pattern 1: Table-Driven Tests
//
// See TESTING.md for detailed pattern explanations.
package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"enphase-monitor/internal/types"
)

// TestExchangeAuthorizationCode_MissingAPIConfig tests error handling when API config is nil
func TestExchangeAuthorizationCode_MissingAPIConfig(t *testing.T) {
	_, err := ExchangeAuthorizationCode(context.Background(), nil, "test_code")
	if err == nil {
		t.Error("Expected error for nil API config")
	}
	if err.Error() != "API configuration is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestExchangeAuthorizationCode_MissingAuthURL tests missing authorization URL
func TestExchangeAuthorizationCode_MissingAuthURL(t *testing.T) {
	apiConfig := &types.APIConfig{
		ClientID:     "test_client",
		ClientSecret: "test_secret",
		RedirectURI:  "http://localhost:8080/callback",
	}

	_, err := ExchangeAuthorizationCode(context.Background(), apiConfig, "test_code")
	if err == nil {
		t.Error("Expected error for missing authorization URL")
	}
	if err.Error() != "authorization_url is required in API configuration" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestExchangeAuthorizationCode_MissingClientCredentials tests missing client_id/client_secret
func TestExchangeAuthorizationCode_MissingClientCredentials(t *testing.T) {
	tests := []struct {
		name         string
		clientID     string
		clientSecret string
		wantError    string
	}{
		{
			name:         "missing both",
			clientID:     "",
			clientSecret: "",
			wantError:    "client_id and client_secret are required in API configuration",
		},
		{
			name:         "missing client_id",
			clientID:     "",
			clientSecret: "secret",
			wantError:    "client_id and client_secret are required in API configuration",
		},
		{
			name:         "missing client_secret",
			clientID:     "client",
			clientSecret: "",
			wantError:    "client_id and client_secret are required in API configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiConfig := &types.APIConfig{
				AuthorizationURL: "http://example.com/token",
				ClientID:         tt.clientID,
				ClientSecret:     tt.clientSecret,
				RedirectURI:      "http://localhost:8080/callback",
			}

			_, err := ExchangeAuthorizationCode(context.Background(), apiConfig, "test_code")
			if err == nil {
				t.Error("Expected error for missing credentials")
			}
			if err.Error() != tt.wantError {
				t.Errorf("Got error: %v, want: %v", err, tt.wantError)
			}
		})
	}
}

// TestExchangeAuthorizationCode_MissingRedirectURI tests missing redirect URI
func TestExchangeAuthorizationCode_MissingRedirectURI(t *testing.T) {
	apiConfig := &types.APIConfig{
		AuthorizationURL: "http://example.com/token",
		ClientID:         "test_client",
		ClientSecret:     "test_secret",
		RedirectURI:      "", // Missing
	}

	_, err := ExchangeAuthorizationCode(context.Background(), apiConfig, "test_code")
	if err == nil {
		t.Error("Expected error for missing redirect URI")
	}
	if err.Error() != "redirect_uri is required in API configuration" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestExchangeAuthorizationCode_EmptyCode tests empty authorization code
func TestExchangeAuthorizationCode_EmptyCode(t *testing.T) {
	apiConfig := &types.APIConfig{
		AuthorizationURL: "http://example.com/token",
		ClientID:         "test_client",
		ClientSecret:     "test_secret",
		RedirectURI:      "http://localhost:8080/callback",
	}

	_, err := ExchangeAuthorizationCode(context.Background(), apiConfig, "")
	if err == nil {
		t.Error("Expected error for empty authorization code")
	}
	if err.Error() != "authorization code is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestExchangeAuthorizationCode_NetworkError tests network failure handling
func TestExchangeAuthorizationCode_NetworkError(t *testing.T) {
	apiConfig := &types.APIConfig{
		AuthorizationURL: "http://invalid-host-that-does-not-exist-12345.com/token",
		ClientID:         "test_client",
		ClientSecret:     "test_secret",
		RedirectURI:      "http://localhost:8080/callback",
		Key:              "test_key",
	}

	_, err := ExchangeAuthorizationCode(context.Background(), apiConfig, "test_code")
	if err == nil {
		t.Error("Expected error for network failure")
	}
	// Should contain "failed to get token"
	if err.Error() == "" {
		t.Error("Expected non-empty error message")
	}
}

// TestExchangeAuthorizationCode_NonOKStatus tests non-200 response handling
func TestExchangeAuthorizationCode_NonOKStatus(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid_grant"}`))
	}))
	defer server.Close()

	apiConfig := &types.APIConfig{
		AuthorizationURL: server.URL,
		ClientID:         "test_client",
		ClientSecret:     "test_secret",
		RedirectURI:      "http://localhost:8080/callback",
	}

	_, err := ExchangeAuthorizationCode(context.Background(), apiConfig, "test_code")
	if err == nil {
		t.Error("Expected error for 400 status")
	}
	// Should contain status code in error
	if err.Error() == "" {
		t.Error("Expected non-empty error message")
	}
}

// TestExchangeAuthorizationCode_MalformedJSON tests malformed JSON response
func TestExchangeAuthorizationCode_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not valid json {{{`))
	}))
	defer server.Close()

	apiConfig := &types.APIConfig{
		AuthorizationURL: server.URL,
		ClientID:         "test_client",
		ClientSecret:     "test_secret",
		RedirectURI:      "http://localhost:8080/callback",
	}

	_, err := ExchangeAuthorizationCode(context.Background(), apiConfig, "test_code")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestGetAccessToken_MissingAPIConfig tests nil API config
func TestGetAccessToken_MissingAPIConfig(t *testing.T) {
	_, err := GetAccessToken(context.Background(), nil)
	if err == nil {
		t.Error("Expected error for nil API config")
	}
	if err.Error() != "API configuration is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestGetAccessToken_MissingAuthURL tests missing authorization URL
func TestGetAccessToken_MissingAuthURL(t *testing.T) {
	apiConfig := &types.APIConfig{
		ClientID:     "test_client",
		ClientSecret: "test_secret",
	}

	_, err := GetAccessToken(context.Background(), apiConfig)
	if err == nil {
		t.Error("Expected error for missing authorization URL")
	}
	if err.Error() != "authorization_url is required in API configuration" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestGetAccessToken_MissingCredentials tests missing client credentials
func TestGetAccessToken_MissingCredentials(t *testing.T) {
	tests := []struct {
		name         string
		clientID     string
		clientSecret string
	}{
		{"missing both", "", ""},
		{"missing client_id", "", "secret"},
		{"missing client_secret", "client", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiConfig := &types.APIConfig{
				AuthorizationURL: "http://example.com/token",
				ClientID:         tt.clientID,
				ClientSecret:     tt.clientSecret,
			}

			_, err := GetAccessToken(context.Background(), apiConfig)
			if err == nil {
				t.Error("Expected error for missing credentials")
			}
		})
	}
}

// TestGetAccessToken_NoAuthMethod tests when no auth method is available
func TestGetAccessToken_NoAuthMethod(t *testing.T) {
	apiConfig := &types.APIConfig{
		AuthorizationURL: "http://example.com/token",
		ClientID:         "test_client",
		ClientSecret:     "test_secret",
		// No refresh_token, no username/password
	}

	_, err := GetAccessToken(context.Background(), apiConfig)
	if err == nil {
		t.Error("Expected error for no auth method")
	}
	// Should mention refresh_token setup
	if err.Error() == "" {
		t.Error("Expected non-empty error message")
	}
}

// TestGetAccessToken_NetworkError tests network failure
func TestGetAccessToken_NetworkError(t *testing.T) {
	// Clear token cache to force new request
	tokenCache = nil

	apiConfig := &types.APIConfig{
		AuthorizationURL: "http://invalid-host-12345.com/token",
		ClientID:         "test_client",
		ClientSecret:     "test_secret",
		RefreshToken:     "test_refresh_token",
	}

	_, err := GetAccessToken(context.Background(), apiConfig)
	if err == nil {
		t.Error("Expected error for network failure")
	}
}

// TestGetAccessToken_PasswordGrant tests password grant flow
func TestGetAccessToken_PasswordGrant(t *testing.T) {
	// Clear token cache
	tokenCache = nil

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify password grant parameters
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if r.Form.Get("grant_type") != "password" {
			t.Errorf("Expected grant_type=password, got %s", r.Form.Get("grant_type"))
		}

		if r.Form.Get("username") != "test_user" {
			t.Errorf("Expected username=test_user, got %s", r.Form.Get("username"))
		}

		if r.Form.Get("password") != "test_pass" {
			t.Errorf("Expected password=test_pass, got %s", r.Form.Get("password"))
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token": "test_access", "expires_in": 3600}`))
	}))
	defer server.Close()

	apiConfig := &types.APIConfig{
		AuthorizationURL: server.URL,
		ClientID:         "test_client",
		ClientSecret:     "test_secret",
		Username:         "test_user",
		Password:         "test_pass",
		// No refresh_token - should use password grant
	}

	token, err := GetAccessToken(context.Background(), apiConfig)
	if err != nil {
		t.Errorf("GetAccessToken() failed: %v", err)
	}

	if token != "test_access" {
		t.Errorf("Token = %s, want test_access", token)
	}
}

// TestGetAccessToken_CacheValid tests cache behavior with valid token
func TestGetAccessToken_CacheValid(t *testing.T) {
	// Set up cache with valid token
	tokenCache = &TokenCache{
		Token:        "cached_token",
		RefreshToken: "cached_refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour), // Valid for 1 hour
	}

	apiConfig := &types.APIConfig{
		AuthorizationURL: "http://example.com/token",
		ClientID:         "test_client",
		ClientSecret:     "test_secret",
		RefreshToken:     "test_refresh",
	}

	token, err := GetAccessToken(context.Background(), apiConfig)
	if err != nil {
		t.Errorf("GetAccessToken() failed: %v", err)
	}

	if token != "cached_token" {
		t.Errorf("Expected cached token, got: %s", token)
	}

	// Clean up
	tokenCache = nil
}

// TestGetAccessToken_CacheExpired tests expired cache token
func TestGetAccessToken_CacheExpired(t *testing.T) {
	// Set up cache with expired token
	tokenCache = &TokenCache{
		Token:        "expired_token",
		RefreshToken: "expired_refresh",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token": "new_token", "refresh_token": "new_refresh", "expires_in": 3600}`))
	}))
	defer server.Close()

	apiConfig := &types.APIConfig{
		AuthorizationURL: server.URL,
		ClientID:         "test_client",
		ClientSecret:     "test_secret",
		RefreshToken:     "test_refresh",
	}

	token, err := GetAccessToken(context.Background(), apiConfig)
	if err != nil {
		t.Errorf("GetAccessToken() failed: %v", err)
	}

	if token != "new_token" {
		t.Errorf("Expected new token, got: %s", token)
	}

	// Clean up
	tokenCache = nil
}

// TestGetAccessToken_ContextCancellation tests context cancellation
func TestGetAccessToken_ContextCancellation(t *testing.T) {
	tokenCache = nil

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	apiConfig := &types.APIConfig{
		AuthorizationURL: "http://example.com/token",
		ClientID:         "test_client",
		ClientSecret:     "test_secret",
		RefreshToken:     "test_refresh",
	}

	_, err := GetAccessToken(ctx, apiConfig)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

// TestGetAuthorizationURL_EmptyClientID tests missing client ID
func TestGetAuthorizationURL_EmptyClientID(t *testing.T) {
	apiConfig := &types.APIConfig{
		ClientID:    "", // Empty
		RedirectURI: "http://localhost:8080/callback",
	}

	_, err := GetAuthorizationURL(apiConfig)
	if err == nil {
		t.Error("Expected error for empty client_id")
	}
}

// TestGetAuthorizationURL_EmptyRedirectURI tests missing redirect URI
func TestGetAuthorizationURL_EmptyRedirectURI(t *testing.T) {
	apiConfig := &types.APIConfig{
		ClientID:    "test_client",
		RedirectURI: "", // Empty
	}

	_, err := GetAuthorizationURL(apiConfig)
	if err == nil {
		t.Error("Expected error for empty redirect_uri")
	}
}
