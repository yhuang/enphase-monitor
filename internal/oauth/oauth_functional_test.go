package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"enphase-monitor/internal/types"
)

// TestGetAccessToken_RetriesOn5xx verifies transient 5xx responses are retried
// and eventually succeed.
func TestGetAccessToken_RetriesOn5xx(t *testing.T) {
	originalCache := tokenCache
	tokenCache = make(map[string]*TokenCache)
	origBackoff := tokenRetryBackoff
	tokenRetryBackoff = time.Millisecond
	defer func() { tokenCache = originalCache; tokenRetryBackoff = origBackoff }()

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError) // fail the first two
			return
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "after-retry", ExpiresIn: 3600})
	}))
	defer server.Close()

	cfg := &types.APIConfig{Key: "k", AuthorizationURL: server.URL, ClientID: "retry-client", ClientSecret: "s", RefreshToken: "rt"}
	tok, err := GetAccessToken(context.Background(), cfg)
	if err != nil {
		t.Fatalf("GetAccessToken() error = %v", err)
	}
	if tok != "after-retry" {
		t.Errorf("token = %q, want after-retry", tok)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("server hits = %d, want 3 (2 retries then success)", got)
	}
}

// TestGetAccessToken_GivesUpAfterMax5xx verifies retries are bounded.
func TestGetAccessToken_GivesUpAfterMax5xx(t *testing.T) {
	originalCache := tokenCache
	tokenCache = make(map[string]*TokenCache)
	origBackoff := tokenRetryBackoff
	tokenRetryBackoff = time.Millisecond
	defer func() { tokenCache = originalCache; tokenRetryBackoff = origBackoff }()

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadGateway) // always 502
	}))
	defer server.Close()

	cfg := &types.APIConfig{Key: "k", AuthorizationURL: server.URL, ClientID: "giveup-client", ClientSecret: "s", RefreshToken: "rt"}
	if _, err := GetAccessToken(context.Background(), cfg); err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if got := atomic.LoadInt32(&hits); got != tokenMaxAttempts {
		t.Errorf("server hits = %d, want %d (no more than the attempt cap)", got, tokenMaxAttempts)
	}
}

// TestGetAccessToken_DoesNotRetry4xx verifies client errors are not retried.
func TestGetAccessToken_DoesNotRetry4xx(t *testing.T) {
	originalCache := tokenCache
	tokenCache = make(map[string]*TokenCache)
	defer func() { tokenCache = originalCache }()

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusUnauthorized) // 401
	}))
	defer server.Close()

	cfg := &types.APIConfig{Key: "k", AuthorizationURL: server.URL, ClientID: "no-retry-client", ClientSecret: "s", RefreshToken: "rt"}
	if _, err := GetAccessToken(context.Background(), cfg); err == nil {
		t.Fatal("expected error for 401")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("server hits = %d, want 1 (4xx must not be retried)", got)
	}
}

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
		resp := TokenResponse{
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
	tokenCache = make(map[string]*TokenCache)
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
		resp := TokenResponse{
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
	cached := tokenCache["test-client-id"]
	if cached == nil {
		t.Fatal("Token should be cached")
	}
	if cached.Token != "new-access-token" {
		t.Errorf("Cached token = %s, want new-access-token", cached.Token)
	}

	// Verify expiration time is reasonable (should be ~1 hour from now)
	expectedExpiry := time.Now().Add(3600 * time.Second)
	if cached.ExpiresAt.Before(time.Now()) || cached.ExpiresAt.After(expectedExpiry.Add(1*time.Minute)) {
		t.Errorf("ExpiresAt = %v, expected around %v", cached.ExpiresAt, expectedExpiry)
	}
}

// TestGetAccessToken_WithPasswordGrant tests token acquisition using password grant
func TestGetAccessToken_WithPasswordGrant(t *testing.T) {
	// Clear token cache before test
	originalCache := tokenCache
	tokenCache = make(map[string]*TokenCache)
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
		resp := TokenResponse{
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
	tokenCache = map[string]*TokenCache{"test-client-id": {
		Token:        "cached-token",
		RefreshToken: "cached-refresh",
		ExpiresAt:    time.Now().Add(30 * time.Minute), // Valid for 30 more minutes
	}}
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
	tokenCache = map[string]*TokenCache{"test-client-id": {
		Token:        "expiring-token",
		RefreshToken: "test-refresh",
		ExpiresAt:    time.Now().Add(3 * time.Minute), // Expires in 3 minutes (< 5 min buffer)
	}}
	defer func() {
		tokenCache = originalCache
	}()

	// Mock server should be called to refresh
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true

		resp := TokenResponse{
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
	tokenCache = make(map[string]*TokenCache)
	defer func() {
		tokenCache = originalCache
	}()

	// Mock server returns response without access_token
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TokenResponse{
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
	tokenCache = make(map[string]*TokenCache)
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
			expectedErrSubstr: "--update-refresh-tokens",
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
	tokenCache = make(map[string]*TokenCache)
	defer func() {
		tokenCache = originalCache
	}()

	// Mock server returns response without expires_in
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TokenResponse{
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
	cached := tokenCache["test-client-id"]
	if cached == nil {
		t.Fatal("Token should be cached")
	}

	expectedExpiry := time.Now().Add(3600 * time.Second)
	// Allow 1 minute tolerance for test execution time
	if cached.ExpiresAt.Before(time.Now()) || cached.ExpiresAt.After(expectedExpiry.Add(1*time.Minute)) {
		t.Errorf("ExpiresAt = %v, expected around %v (default 1 hour)", cached.ExpiresAt, expectedExpiry)
	}
}

// TestGetAccessToken_PreservesRefreshToken tests that refresh token is preserved in cache
func TestGetAccessToken_PreservesRefreshToken(t *testing.T) {
	// Set up cache with existing refresh token
	originalCache := tokenCache
	tokenCache = map[string]*TokenCache{"test-client-id": {
		Token:        "old-token",
		RefreshToken: "existing-refresh-token",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired
	}}
	defer func() {
		tokenCache = originalCache
	}()

	// Mock server returns new access token but NO refresh token
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TokenResponse{
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
	if cached := tokenCache["test-client-id"]; cached.RefreshToken != "existing-refresh-token" {
		t.Errorf("RefreshToken = %s, want existing-refresh-token (should be preserved)", cached.RefreshToken)
	}
}
