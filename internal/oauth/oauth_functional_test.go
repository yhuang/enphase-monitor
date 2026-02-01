// Package oauth - oauth_functional_test.go
//
// TEST SETUP
// ----------
// This test suite validates OAuth 2.0 flows using mock HTTP servers.
// Uses httptest.NewServer to simulate Enphase authorization server without real network calls.
//
// TEST PLAN
// ---------
// 1. Token Exchange Tests (Authorization Code Flow)
//   - Test successful code exchange
//   - Test HTTP method validation (must be POST)
//   - Test request header validation (Content-Type, Authorization)
//   - Test request body parsing
//
// 2. Token Refresh Tests
//   - Test successful token refresh
//   - Test expired token refresh
//   - Test refresh token validation
//
// 3. HTTP Error Tests
//   - Test 401 Unauthorized response
//   - Test 500 Server Error response
//   - Test malformed JSON response
//
// TESTING APPROACH
// ----------------
// - httptest.NewServer creates a mock OAuth server
// - Server handler validates requests and returns mock responses
// - Tests verify both request format and response parsing
// - Mock server is closed after each test (defer server.Close())
//
// WHY MOCK HTTP SERVER
// --------------------
// Mock HTTP servers enable:
// - Testing real HTTP interactions without external dependencies
// - Control over response codes, headers, and body content
// - Fast, deterministic tests (no network latency or failures)
// - No API rate limits or quota consumption
//
// TEST ORGANIZATION
// -----------------
// This package has 3 test files (1:many pattern):
// - oauth_test.go: Basic unit tests (270 lines)
// - oauth_functional_test.go (this file): Integration tests (598 lines)
// - oauth_edge_cases_test.go: Edge cases and error paths (442 lines)
//
// PATTERN USED
// ------------
// - Pattern 4: Mock HTTP Servers (httptest.NewServer)
// - Pattern 1: Table-Driven Tests
// - Pattern 7: Error Path Testing
//
// See TESTING.md for detailed pattern explanations.
package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/types"
)

// TestExchangeAuthorizationCode_Success tests successful token exchange with mock HTTP server
func TestExchangeAuthorizationCode_Success(t *testing.T) {
	// Create mock OAuth server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and headers
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Verify Basic Auth
		username, password, ok := r.BasicAuth()
		if !ok || username != "test-client-id" || password != "test-client-secret" {
			t.Errorf("Invalid Basic Auth: username=%s, password=%s", username, password)
		}

		// Verify Content-Type
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Expected Content-Type application/x-www-form-urlencoded, got %s", ct)
		}

		// Verify key header
		if key := r.Header.Get("key"); key != "test-api-key" {
			t.Errorf("Expected key header test-api-key, got %s", key)
		}

		// Parse form data
		if err := r.ParseForm(); err != nil {
			t.Fatalf("Failed to parse form: %v", err)
		}

		// Verify grant_type, code, redirect_uri
		if gt := r.Form.Get("grant_type"); gt != "authorization_code" {
			t.Errorf("Expected grant_type authorization_code, got %s", gt)
		}
		if code := r.Form.Get("code"); code != "test-auth-code" {
			t.Errorf("Expected code test-auth-code, got %s", code)
		}
		if ru := r.Form.Get("redirect_uri"); ru != "http://localhost:8080/callback" {
			t.Errorf("Expected redirect_uri http://localhost:8080/callback, got %s", ru)
		}

		// Return mock token response
		resp := OAuthTokenResponse{
			AccessToken:  "mock-access-token",
			TokenType:    "Bearer",
			RefreshToken: "mock-refresh-token",
			ExpiresIn:    3600,
			Scope:        "read write",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Test with mock server
	ctx := context.Background()
	config := &types.APIConfig{
		Key:              "test-api-key",
		AuthorizationURL: server.URL,
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		RedirectURI:      "http://localhost:8080/callback",
	}

	tokenResp, err := ExchangeAuthorizationCode(ctx, config, "test-auth-code")
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
	}

	// Verify response
	if tokenResp.AccessToken != "mock-access-token" {
		t.Errorf("AccessToken = %s, want mock-access-token", tokenResp.AccessToken)
	}
	if tokenResp.RefreshToken != "mock-refresh-token" {
		t.Errorf("RefreshToken = %s, want mock-refresh-token", tokenResp.RefreshToken)
	}
	if tokenResp.TokenType != "Bearer" {
		t.Errorf("TokenType = %s, want Bearer", tokenResp.TokenType)
	}
	if tokenResp.ExpiresIn != 3600 {
		t.Errorf("ExpiresIn = %d, want 3600", tokenResp.ExpiresIn)
	}
}

// TestExchangeAuthorizationCode_HTTPError tests HTTP error handling
func TestExchangeAuthorizationCode_HTTPError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedErrMsg string
	}{
		{
			name:           "unauthorized",
			statusCode:     401,
			responseBody:   `{"error": "invalid_client"}`,
			expectedErrMsg: "token request failed with status 401",
		},
		{
			name:           "invalid_grant",
			statusCode:     400,
			responseBody:   `{"error": "invalid_grant"}`,
			expectedErrMsg: "token request failed with status 400",
		},
		{
			name:           "server_error",
			statusCode:     500,
			responseBody:   "Internal Server Error",
			expectedErrMsg: "token request failed with status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			ctx := context.Background()
			config := &types.APIConfig{
				AuthorizationURL: server.URL,
				ClientID:         "test-client-id",
				ClientSecret:     "test-client-secret",
				RedirectURI:      "http://localhost:8080/callback",
			}

			_, err := ExchangeAuthorizationCode(ctx, config, "test-code")
			if err == nil {
				t.Fatal("Expected error but got nil")
			}

			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Error = %v, want substring %s", err, tt.expectedErrMsg)
			}
		})
	}
}

// TestExchangeAuthorizationCode_InvalidJSON tests JSON parsing error handling
func TestExchangeAuthorizationCode_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"invalid json`))
	}))
	defer server.Close()

	ctx := context.Background()
	config := &types.APIConfig{
		AuthorizationURL: server.URL,
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		RedirectURI:      "http://localhost:8080/callback",
	}

	_, err := ExchangeAuthorizationCode(ctx, config, "test-code")
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}

	if !strings.Contains(err.Error(), "failed to decode") {
		t.Errorf("Error should mention decode failure, got: %v", err)
	}
}

// TestGetAccessToken_WithRefreshToken tests token acquisition using refresh_token grant
func TestGetAccessToken_WithRefreshToken(t *testing.T) {
	// Clear token cache before test
	originalCache := tokenCache
	tokenCache = nil
	defer func() {
		tokenCache = originalCache
	}()

	// Create mock OAuth server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse form data
		if err := r.ParseForm(); err != nil {
			t.Fatalf("Failed to parse form: %v", err)
		}

		// Verify refresh_token grant
		if gt := r.Form.Get("grant_type"); gt != "refresh_token" {
			t.Errorf("Expected grant_type refresh_token, got %s", gt)
		}
		if rt := r.Form.Get("refresh_token"); rt != "test-refresh-token" {
			t.Errorf("Expected refresh_token test-refresh-token, got %s", rt)
		}

		// Return mock token response
		resp := OAuthTokenResponse{
			AccessToken: "new-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Test with refresh token
	ctx := context.Background()
	config := &types.APIConfig{
		Key:              "test-api-key",
		AuthorizationURL: server.URL,
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		RefreshToken:     "test-refresh-token",
	}

	accessToken, err := GetAccessToken(ctx, config)
	if err != nil {
		t.Fatalf("GetAccessToken() error = %v", err)
	}

	if accessToken != "new-access-token" {
		t.Errorf("AccessToken = %s, want new-access-token", accessToken)
	}

	// Verify token was cached
	if tokenCache == nil {
		t.Fatal("Token should be cached")
	}
	if tokenCache.Token != "new-access-token" {
		t.Errorf("Cached token = %s, want new-access-token", tokenCache.Token)
	}

	// Verify expiration time is reasonable (should be ~1 hour from now)
	expectedExpiry := time.Now().Add(3600 * time.Second)
	if tokenCache.ExpiresAt.Before(time.Now()) || tokenCache.ExpiresAt.After(expectedExpiry.Add(1*time.Minute)) {
		t.Errorf("ExpiresAt = %v, expected around %v", tokenCache.ExpiresAt, expectedExpiry)
	}
}

// TestGetAccessToken_WithPasswordGrant tests token acquisition using password grant
func TestGetAccessToken_WithPasswordGrant(t *testing.T) {
	// Clear token cache before test
	originalCache := tokenCache
	tokenCache = nil
	defer func() {
		tokenCache = originalCache
	}()

	// Create mock OAuth server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse form data
		if err := r.ParseForm(); err != nil {
			t.Fatalf("Failed to parse form: %v", err)
		}

		// Verify password grant
		if gt := r.Form.Get("grant_type"); gt != "password" {
			t.Errorf("Expected grant_type password, got %s", gt)
		}
		if un := r.Form.Get("username"); un != "test-user" {
			t.Errorf("Expected username test-user, got %s", un)
		}
		if pw := r.Form.Get("password"); pw != "test-pass" {
			t.Errorf("Expected password test-pass, got %s", pw)
		}

		// Return mock token response
		resp := OAuthTokenResponse{
			AccessToken: "password-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Test with username/password (no refresh token)
	ctx := context.Background()
	config := &types.APIConfig{
		AuthorizationURL: server.URL,
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		Username:         "test-user",
		Password:         "test-pass",
	}

	accessToken, err := GetAccessToken(ctx, config)
	if err != nil {
		t.Fatalf("GetAccessToken() error = %v", err)
	}

	if accessToken != "password-access-token" {
		t.Errorf("AccessToken = %s, want password-access-token", accessToken)
	}
}

// TestGetAccessToken_CacheHit tests that cached tokens are returned without API call
func TestGetAccessToken_CacheHit(t *testing.T) {
	// Set up cache with valid token
	originalCache := tokenCache
	tokenCache = &TokenCache{
		Token:        "cached-token",
		RefreshToken: "cached-refresh",
		ExpiresAt:    time.Now().Add(30 * time.Minute), // Valid for 30 more minutes
	}
	defer func() {
		tokenCache = originalCache
	}()

	// Mock server should NOT be called
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		t.Error("Server should not be called when token is cached")
	}))
	defer server.Close()

	ctx := context.Background()
	config := &types.APIConfig{
		AuthorizationURL: server.URL,
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		RefreshToken:     "test-refresh-token",
	}

	accessToken, err := GetAccessToken(ctx, config)
	if err != nil {
		t.Fatalf("GetAccessToken() error = %v", err)
	}

	if accessToken != "cached-token" {
		t.Errorf("AccessToken = %s, want cached-token", accessToken)
	}

	if serverCalled {
		t.Error("Server was called despite valid cached token")
	}
}

// TestGetAccessToken_CacheMiss_NearExpiry tests that tokens near expiry are refreshed
func TestGetAccessToken_CacheMiss_NearExpiry(t *testing.T) {
	// Set up cache with token that expires soon (within buffer window)
	originalCache := tokenCache
	tokenCache = &TokenCache{
		Token:        "expiring-token",
		RefreshToken: "test-refresh",
		ExpiresAt:    time.Now().Add(3 * time.Minute), // Expires in 3 minutes (< 5 min buffer)
	}
	defer func() {
		tokenCache = originalCache
	}()

	// Mock server should be called to refresh
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true

		resp := OAuthTokenResponse{
			AccessToken: "refreshed-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	config := &types.APIConfig{
		AuthorizationURL: server.URL,
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		RefreshToken:     "test-refresh-token",
	}

	accessToken, err := GetAccessToken(ctx, config)
	if err != nil {
		t.Fatalf("GetAccessToken() error = %v", err)
	}

	if accessToken != "refreshed-token" {
		t.Errorf("AccessToken = %s, want refreshed-token", accessToken)
	}

	if !serverCalled {
		t.Error("Server should be called to refresh token near expiry")
	}
}

// TestGetAccessToken_EmptyAccessToken tests error when API returns no access_token
func TestGetAccessToken_EmptyAccessToken(t *testing.T) {
	// Clear token cache
	originalCache := tokenCache
	tokenCache = nil
	defer func() {
		tokenCache = originalCache
	}()

	// Mock server returns response without access_token
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := OAuthTokenResponse{
			TokenType: "Bearer",
			ExpiresIn: 3600,
			// No AccessToken field
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	config := &types.APIConfig{
		AuthorizationURL: server.URL,
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		RefreshToken:     "test-refresh-token",
	}

	_, err := GetAccessToken(ctx, config)
	if err == nil {
		t.Fatal("Expected error for empty access_token")
	}

	if !strings.Contains(err.Error(), "no access token") {
		t.Errorf("Error should mention missing access token, got: %v", err)
	}
}

// TestGetAccessToken_UnauthorizedError tests helpful error message for 401 errors
func TestGetAccessToken_UnauthorizedError(t *testing.T) {
	// Clear token cache
	originalCache := tokenCache
	tokenCache = nil
	defer func() {
		tokenCache = originalCache
	}()

	tests := []struct {
		name              string
		hasRefreshToken   bool
		expectedErrSubstr string
	}{
		{
			name:              "with_refresh_token",
			hasRefreshToken:   true,
			expectedErrSubstr: "--setup-oauth",
		},
		{
			name:              "without_refresh_token",
			hasRefreshToken:   false,
			expectedErrSubstr: "check your credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock server returns 401
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"error": "invalid_token"}`))
			}))
			defer server.Close()

			ctx := context.Background()
			config := &types.APIConfig{
				AuthorizationURL: server.URL,
				ClientID:         "test-client-id",
				ClientSecret:     "test-client-secret",
			}

			if !tt.hasRefreshToken {
				config.Username = "test-user"
				config.Password = "wrong-password"
			}
			if tt.hasRefreshToken {
				config.RefreshToken = "invalid-refresh-token"
			}

			_, err := GetAccessToken(ctx, config)
			if err == nil {
				t.Fatal("Expected error for 401 status")
			}

			if !strings.Contains(err.Error(), tt.expectedErrSubstr) {
				t.Errorf("Error should contain %q, got: %v", tt.expectedErrSubstr, err)
			}
		})
	}
}

// TestGetAccessToken_DefaultExpiresIn tests default expires_in when not provided
func TestGetAccessToken_DefaultExpiresIn(t *testing.T) {
	// Clear token cache
	originalCache := tokenCache
	tokenCache = nil
	defer func() {
		tokenCache = originalCache
	}()

	// Mock server returns response without expires_in
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := OAuthTokenResponse{
			AccessToken: "test-token",
			TokenType:   "Bearer",
			// ExpiresIn is 0 (not set)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	config := &types.APIConfig{
		AuthorizationURL: server.URL,
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		RefreshToken:     "test-refresh-token",
	}

	_, err := GetAccessToken(ctx, config)
	if err != nil {
		t.Fatalf("GetAccessToken() error = %v", err)
	}

	// Verify default expires_in of 3600 seconds (1 hour) was used
	if tokenCache == nil {
		t.Fatal("Token should be cached")
	}

	expectedExpiry := time.Now().Add(3600 * time.Second)
	// Allow 1 minute tolerance for test execution time
	if tokenCache.ExpiresAt.Before(time.Now()) || tokenCache.ExpiresAt.After(expectedExpiry.Add(1*time.Minute)) {
		t.Errorf("ExpiresAt = %v, expected around %v (default 1 hour)", tokenCache.ExpiresAt, expectedExpiry)
	}
}

// TestGetAccessToken_PreservesRefreshToken tests that refresh token is preserved in cache
func TestGetAccessToken_PreservesRefreshToken(t *testing.T) {
	// Set up cache with existing refresh token
	originalCache := tokenCache
	tokenCache = &TokenCache{
		Token:        "old-token",
		RefreshToken: "existing-refresh-token",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired
	}
	defer func() {
		tokenCache = originalCache
	}()

	// Mock server returns new access token but NO refresh token
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := OAuthTokenResponse{
			AccessToken: "new-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
			// No RefreshToken in response
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	config := &types.APIConfig{
		AuthorizationURL: server.URL,
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		RefreshToken:     "config-refresh-token",
	}

	_, err := GetAccessToken(ctx, config)
	if err != nil {
		t.Fatalf("GetAccessToken() error = %v", err)
	}

	// Verify existing refresh token was preserved
	if tokenCache.RefreshToken != "existing-refresh-token" {
		t.Errorf("RefreshToken = %s, want existing-refresh-token (should be preserved)", tokenCache.RefreshToken)
	}
}
