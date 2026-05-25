package oauth

import (
	"context"
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/types"
)

// TestGetAuthorizationURL tests OAuth authorization URL generation
func TestGetAuthorizationURL(t *testing.T) {
	tests := []struct {
		name     string
		config   *types.APIConfig
		wantErr  bool
		checkURL func(string) bool
	}{
		{
			name: "valid_config",
			config: &types.APIConfig{
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
			config: &types.APIConfig{
				ClientID:    "",
				RedirectURI: "http://localhost:8080/callback",
			},
			wantErr:  true,
			checkURL: nil,
		},
		{
			name: "missing_redirect_uri",
			config: &types.APIConfig{
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
		config  *types.APIConfig
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
			config: &types.APIConfig{
				ClientID:     "test-id",
				ClientSecret: "test-secret",
				RedirectURI:  "http://localhost:8080/callback",
			},
			code:    "test-code",
			wantErr: true,
		},
		{
			name: "missing_client_id",
			config: &types.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientSecret:     "test-secret",
				RedirectURI:      "http://localhost:8080/callback",
			},
			code:    "test-code",
			wantErr: true,
		},
		{
			name: "missing_client_secret",
			config: &types.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientID:         "test-id",
				RedirectURI:      "http://localhost:8080/callback",
			},
			code:    "test-code",
			wantErr: true,
		},
		{
			name: "missing_redirect_uri",
			config: &types.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientID:         "test-id",
				ClientSecret:     "test-secret",
			},
			code:    "test-code",
			wantErr: true,
		},
		{
			name: "empty_code",
			config: &types.APIConfig{
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
					return
				}
				t.Logf("Got expected error: %v", err)
				return
			}
			// Note: Will fail with network error since we don't mock HTTP
			// This tests error validation only
			if err != nil && !strings.Contains(err.Error(), "failed") {
				t.Logf("Got expected error: %v", err)
			}
		})
	}
}

// TestGetAccessToken_Errors tests error handling
func TestGetAccessToken_Errors(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		config  *types.APIConfig
		wantErr bool
	}{
		{
			name:    "nil_config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "missing_authorization_url",
			config: &types.APIConfig{
				ClientID:     "test-id",
				ClientSecret: "test-secret",
			},
			wantErr: true,
		},
		{
			name: "missing_client_id",
			config: &types.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientSecret:     "test-secret",
			},
			wantErr: true,
		},
		{
			name: "missing_client_secret",
			config: &types.APIConfig{
				AuthorizationURL: "https://api.example.com/oauth/token",
				ClientID:         "test-id",
			},
			wantErr: true,
		},
		{
			name: "no_valid_authentication_method",
			config: &types.APIConfig{
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
					return
				}
				t.Logf("Got expected error: %v", err)
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
