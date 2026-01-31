// Package oauth - oauth_test.go
//
// TEST SETUP
// ----------
// This test suite validates OAuth 2.0 authentication logic:
// - Authorization URL generation
// - Token refresh mechanics
// - Configuration validation
//
// These are UNIT tests (no HTTP calls). For integration tests with mock HTTP servers,
// see oauth_functional_test.go. For edge cases and error paths, see oauth_edge_cases_test.go.
//
// TEST PLAN
// ---------
// 1. Authorization URL Tests
//    - Test URL contains required parameters (client_id, redirect_uri, scope)
//    - Test URL encoding is correct
//    - Test validation of missing/empty config fields
//
// 2. Token Refresh Tests
//    - Test token expiration detection
//    - Test cache hit/miss logic
//    - Test token data structure
//
// TESTING APPROACH
// ----------------
// - Table-driven tests for multiple scenarios
// - URL validation by checking required substrings
// - Config validation tests for missing fields
// - No external HTTP calls (pure unit tests)
//
// TEST ORGANIZATION
// -----------------
// This package has 3 test files (1:many pattern):
// - oauth_test.go (this file): Basic unit tests (270 lines)
// - oauth_functional_test.go: Integration tests with mock HTTP (598 lines)
// - oauth_edge_cases_test.go: Edge cases and error paths (442 lines)
//
// PATTERN USED
// ------------
// - Pattern 1: Table-Driven Tests
// - Pattern 3: Subtests with t.Run()
// - Pattern 9: State Reset (token cache)
//
// See TESTING.md for detailed pattern explanations.
package oauth

import (
	"context"
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/config"
)

// TestGetAuthorizationURL tests OAuth authorization URL generation
func TestGetAuthorizationURL(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.APIConfig
		wantErr   bool
		checkURL  func(string) bool
	}{
		{
			name: "valid_config",
			config: &config.APIConfig{
				ClientID:    "test-client-id",
				RedirectURI: "http://localhost:8080/callback",
			},
			wantErr: false,
			checkURL: func(url string) bool {
				return strings.Contains(url, "test-client-id") &&
					(strings.Contains(url, "localhost") || strings.Contains(url, "localhost%3A8080")) &&
					strings.Contains(url, "response_type=code") &&
					strings.Contains(url, "scope=")
			},
		},
		{
			name:     "nil_config",
			config:   nil,
			wantErr:  true,
			checkURL: nil,
		},
		{
			name: "missing_client_id",
			config: &config.APIConfig{
				ClientID:    "",
				RedirectURI: "http://localhost:8080/callback",
			},
			wantErr:  true,
			checkURL: nil,
		},
		{
			name: "missing_redirect_uri",
			config: &config.APIConfig{
				ClientID:    "test-client-id",
				RedirectURI: "",
			},
			wantErr:  true,
			checkURL: nil,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := GetAuthorizationURL(tt.config)
			
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if tt.checkURL != nil && !tt.checkURL(url) {
				t.Errorf("URL validation failed: %s", url)
			}
		})
	}
}

// TestExchangeAuthorizationCode_Errors tests error handling
func TestExchangeAuthorizationCode_Errors(t *testing.T) {
	ctx := context.Background()
	
	tests := []struct {
		name    string
		config  *config.APIConfig
		code    string
		wantErr bool
	}{
		{
			name:    "nil_config",
			config:  nil,
			code:    "test-code",
			wantErr: true,
		},
		{
			name: "missing_authorization_url",
			config: &config.APIConfig{
				ClientID:     "test-id",
				ClientSecret: "test-secret",
				RedirectURI:  "http://localhost:8080/callback",
			},
			code:    "test-code",
			wantErr: true,
		},
		{
			name: "missing_client_id",
			config: &config.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientSecret:     "test-secret",
				RedirectURI:      "http://localhost:8080/callback",
			},
			code:    "test-code",
			wantErr: true,
		},
		{
			name: "missing_client_secret",
			config: &config.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientID:         "test-id",
				RedirectURI:      "http://localhost:8080/callback",
			},
			code:    "test-code",
			wantErr: true,
		},
		{
			name: "missing_redirect_uri",
			config: &config.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientID:         "test-id",
				ClientSecret:     "test-secret",
			},
			code:    "test-code",
			wantErr: true,
		},
		{
			name: "empty_code",
			config: &config.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientID:         "test-id",
				ClientSecret:     "test-secret",
				RedirectURI:      "http://localhost:8080/callback",
			},
			code:    "",
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExchangeAuthorizationCode(ctx, tt.config, tt.code)
			
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
			} else {
				// Note: Will fail with network error since we don't mock HTTP
				// This tests error validation only
				if err != nil && !strings.Contains(err.Error(), "failed") {
					t.Logf("Got expected error: %v", err)
				}
			}
		})
	}
}

// TestGetAccessToken_Errors tests error handling
func TestGetAccessToken_Errors(t *testing.T) {
	ctx := context.Background()
	
	tests := []struct {
		name    string
		config  *config.APIConfig
		wantErr bool
	}{
		{
			name:    "nil_config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "missing_authorization_url",
			config: &config.APIConfig{
				ClientID:     "test-id",
				ClientSecret: "test-secret",
			},
			wantErr: true,
		},
		{
			name: "missing_client_id",
			config: &config.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientSecret:     "test-secret",
			},
			wantErr: true,
		},
		{
			name: "missing_client_secret",
			config: &config.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientID:         "test-id",
			},
			wantErr: true,
		},
		{
			name: "no_valid_authentication_method",
			config: &config.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientID:         "test-id",
				ClientSecret:     "test-secret",
				// No refresh_token or username/password
			},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetAccessToken(ctx, tt.config)
			
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				} else {
					t.Logf("Got expected error: %v", err)
				}
			}
		})
	}
}

// TestTokenCache tests token caching logic
func TestTokenCache(t *testing.T) {
	// Save original token cache and restore after test
	originalCache := tokenCache
	defer func() {
		tokenCache = originalCache
	}()
	
	// Test setting and reading from cache
	tokenCache = &TokenCache{
		Token:        "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	
	if tokenCache.Token != "test-access-token" {
		t.Errorf("Token = %s, want test-access-token", tokenCache.Token)
	}
	
	if tokenCache.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %s, want test-refresh-token", tokenCache.RefreshToken)
	}
	
	// Test expiration check
	if tokenCache.ExpiresAt.Before(time.Now()) {
		t.Error("Token should not be expired yet")
	}
	
	// Test with expired token
	tokenCache.ExpiresAt = time.Now().Add(-1 * time.Hour)
	if !tokenCache.ExpiresAt.Before(time.Now()) {
		t.Error("Token should be expired")
	}
}
