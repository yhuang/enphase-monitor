package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	tokenCache = make(map[string]*TokenCache)

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
	tokenCache = make(map[string]*TokenCache)

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
	tokenCache = map[string]*TokenCache{"test_client": {
		Token:        "cached_token",
		RefreshToken: "cached_refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour), // Valid for 1 hour
	}}

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
	tokenCache = make(map[string]*TokenCache)
}

// TestGetAccessToken_CacheExpired tests expired cache token
func TestGetAccessToken_CacheExpired(t *testing.T) {
	// Set up cache with expired token
	tokenCache = map[string]*TokenCache{"test_client": {
		Token:        "expired_token",
		RefreshToken: "expired_refresh",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}}

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
	tokenCache = make(map[string]*TokenCache)
}

// TestGetAccessToken_ContextCancellation tests context cancellation
func TestGetAccessToken_ContextCancellation(t *testing.T) {
	tokenCache = make(map[string]*TokenCache)

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

// TestAuthorize_NilAPI tests that Authorize returns an error when the credential is nil.
func TestAuthorize_NilAPI(t *testing.T) {
	_, err := Authorize(context.Background(), nil)
	if err == nil {
		t.Fatal("Authorize() with nil API: want error, got nil")
	}
	if !strings.Contains(err.Error(), "API configuration is required") {
		t.Errorf("Authorize() with nil API: got %q, want 'API configuration is required'", err.Error())
	}
}

// TestAuthorize_EmptyClientID tests that Authorize returns an error when ClientID is empty.
func TestAuthorize_EmptyClientID(t *testing.T) {
	_, err := Authorize(context.Background(), &types.APIConfig{ClientID: ""})
	if err == nil {
		t.Fatal("Authorize() with empty ClientID: want error, got nil")
	}
	if !strings.Contains(err.Error(), "client_id is required") {
		t.Errorf("Authorize() with empty ClientID: got %q, want 'client_id is required'", err.Error())
	}
}

// TestAuthorize_AuthorizationError tests the manual-paste fallback's error path.
// A non-loopback redirect URI forces the paste flow (no local listener); the
// pasted URL contains "error=", which causes Authorize to return early without
// token exchange. openBrowser is stubbed so no real browser launches.
func TestAuthorize_AuthorizationError(t *testing.T) {
	orig := openBrowser
	t.Cleanup(func() { openBrowser = orig })
	openBrowser = func(string) error { return nil }

	// Redirect os.Stdin so Authorize can read from it without blocking.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	// Write an error redirect URL and close so reads don't block.
	_, _ = w.WriteString("https://example.com/callback?error=access_denied\n")
	w.Close()

	// A non-loopback redirect URI forces the manual-paste fallback.
	api := &types.APIConfig{
		ClientID:    "test_client_id",
		RedirectURI: "https://example.com/callback",
	}

	_, setupErr := Authorize(context.Background(), api)
	if setupErr == nil {
		t.Fatal("Authorize() with error redirect: want error, got nil")
	}
	if !strings.Contains(setupErr.Error(), "authorization failed") {
		t.Errorf("Authorize() with error redirect: got %q, want 'authorization failed'", setupErr.Error())
	}
}
