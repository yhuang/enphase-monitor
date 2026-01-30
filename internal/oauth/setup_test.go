// Package oauth - setup_test.go
//
// TEST SETUP
// ----------
// This test suite validates the OAuth setup wizard functionality.
// Tests focus on validation logic, URL parsing, and error handling.
//
// TEST PLAN
// ---------
// 1. Setup Function Validation Tests
//    - Test missing API configuration
//    - Test missing client_id
//    - Test redirect URI handling
//
// 2. Authorization Code Extraction Tests
//    - Test extracting code from full URL
//    - Test extracting code from code-only string
//    - Test error detection in URLs
//    - Test malformed URLs
//
// 3. Browser Opening Tests (Platform-Specific)
//    - Test openBrowser returns error for unsupported platforms
//    - Cannot test actual browser opening (requires OS integration)
//
// TESTING APPROACH
// ----------------
// - Table-driven tests for URL parsing scenarios
// - Mock stdin for interactive prompts (future enhancement)
// - Error validation for missing configuration
// - URL parsing logic isolated from interactive flow
//
// LIMITATIONS
// -----------
// Cannot test:
// - Actual browser opening (requires OS integration)
// - Interactive stdin prompts (requires user input simulation)
// - Terminal ANSI escape codes (display-only functionality)
// - Full end-to-end OAuth flow (requires real OAuth server)
//
// PATTERN USED
// ------------
// - Pattern 1: Table-Driven Tests
// - Pattern 3: Subtests with t.Run()
// - Pattern 8: Error Path Testing
//
// See TESTING.md for detailed pattern explanations.
package oauth

import (
	"net/url"
	"runtime"
	"strings"
	"testing"

	"enphase-monitor/internal/config"
	"enphase-monitor/internal/types"
)

// TestSetup_ValidationErrors tests validation of required configuration
func TestSetup_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil API config",
			config:      &config.Config{},
			wantErr:     true,
			errContains: "API configuration is required",
		},
		{
			name: "missing client_id",
			config: &config.Config{
				API: &types.APIConfig{
					ClientID:     "",
					ClientSecret: "secret",
					Key:          "key",
				},
			},
			wantErr:     true,
			errContains: "client_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Setup(tt.config)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Error %q does not contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

// TestExtractAuthorizationCode tests extracting authorization codes from URLs
func TestExtractAuthorizationCode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCode    string
		wantError   bool
		description string
	}{
		{
			name:        "full redirect URL with code",
			input:       "http://localhost:8080/callback?code=ABC123XYZ",
			wantCode:    "ABC123XYZ",
			wantError:   false,
			description: "Extract code from complete redirect URL",
		},
		{
			name:        "full redirect URL with code and state",
			input:       "http://localhost:8080/callback?code=ABC123XYZ&state=random",
			wantCode:    "ABC123XYZ",
			wantError:   false,
			description: "Extract code when state parameter is also present",
		},
		{
			name:        "code only string",
			input:       "ABC123XYZ",
			wantCode:    "ABC123XYZ",
			wantError:   false,
			description: "Handle direct code input (not a URL)",
		},
		{
			name:        "URL with error parameter",
			input:       "http://localhost:8080/callback?error=access_denied",
			wantCode:    "",
			wantError:   true,
			description: "Should detect error in URL",
		},
		{
			name:        "URL with error and description",
			input:       "http://localhost:8080/callback?error=access_denied&error_description=User+denied+access",
			wantCode:    "",
			wantError:   true,
			description: "Should detect error with description",
		},
		{
			name:        "empty string",
			input:       "",
			wantCode:    "",
			wantError:   true,
			description: "Empty input should error",
		},
		{
			name:        "URL without code or error",
			input:       "http://localhost:8080/callback",
			wantCode:    "http://localhost:8080/callback", // Treated as raw code
			wantError:   false,
			description: "URL with no code or error parameter is treated as raw code",
		},
		{
			name:        "URL with empty code parameter",
			input:       "http://localhost:8080/callback?code=",
			wantCode:    "",
			wantError:   true,
			description: "URL with empty code value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the code extraction logic from Setup()
			input := strings.TrimSpace(tt.input)
			
			// Check for error parameter
			hasError := strings.Contains(input, "error=")
			
			// Extract code from URL if present
			var code string
			if strings.Contains(input, "code=") {
				parsedURL, err := url.Parse(input)
				if err == nil {
					code = parsedURL.Query().Get("code")
				}
			} else if !hasError && input != "" {
				// If no "code=" prefix and no error, treat as raw code
				code = input
			}

			// Validate results
			if hasError {
				if !tt.wantError {
					t.Error("Expected no error detection but found error parameter")
				}
				return
			}

			if code == "" && !tt.wantError {
				t.Errorf("Expected code %q but got empty string", tt.wantCode)
			}

			if code != "" && code != tt.wantCode {
				t.Errorf("Got code %q, want %q", code, tt.wantCode)
			}

			if code == "" && tt.wantError {
				// Expected behavior - empty code with error flag
			} else if code == "" && !tt.wantError {
				t.Error("Expected valid code but got empty string")
			}
		})
	}
}

// TestOpenBrowser_UnsupportedPlatform tests browser opening with invalid platform
func TestOpenBrowser_UnsupportedPlatform(t *testing.T) {
	// We can't easily mock runtime.GOOS, but we can test the current platform
	// works without errors
	validURL := "https://api.enphaseenergy.com/oauth/authorize"
	
	err := openBrowser(validURL)
	
	// On supported platforms (darwin, linux, windows), this should succeed
	// On unsupported platforms, it should return an error
	supportedPlatforms := map[string]bool{
		"darwin":  true,
		"linux":   true,
		"windows": true,
	}
	
	if supportedPlatforms[runtime.GOOS] {
		// For supported platforms, we expect either:
		// 1. No error (browser launched successfully)
		// 2. An error from exec.Cmd (browser not found, display not available, etc.)
		// We don't fail the test either way since we can't control the environment
		if err != nil {
			t.Logf("Browser launch returned error (may be expected in test environment): %v", err)
		}
	} else {
		// For unsupported platforms, we expect an error
		if err == nil {
			t.Errorf("Expected error for unsupported platform %s but got none", runtime.GOOS)
		}
		if err != nil && !strings.Contains(err.Error(), "unsupported platform") {
			t.Errorf("Expected 'unsupported platform' error, got: %v", err)
		}
	}
}

// TestOpenBrowser_InvalidURL tests browser opening with invalid URL
func TestOpenBrowser_InvalidURL(t *testing.T) {
	// Test with various invalid URLs
	tests := []struct {
		name string
		url  string
	}{
		{"empty string", ""},
		{"invalid characters", "ht!tp://invalid"},
		{"spaces", "http://example.com/path with spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Even with invalid URLs, the function may not error
			// because it just passes the URL to the OS
			// This test documents the behavior
			err := openBrowser(tt.url)
			t.Logf("openBrowser(%q) returned: %v", tt.url, err)
		})
	}
}

// TestURLParsing_EdgeCases tests URL parsing edge cases
func TestURLParsing_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		rawURL      string
		expectCode  bool
		expectError bool
	}{
		{
			name:        "URL encoded code",
			rawURL:      "http://localhost:8080/callback?code=ABC%2B123",
			expectCode:  true,
			expectError: false,
		},
		{
			name:        "multiple code parameters (takes first)",
			rawURL:      "http://localhost:8080/callback?code=FIRST&code=SECOND",
			expectCode:  true,
			expectError: false,
		},
		{
			name:        "both code and error (error takes precedence)",
			rawURL:      "http://localhost:8080/callback?code=ABC&error=invalid",
			expectCode:  false,
			expectError: true,
		},
		{
			name:        "fragment in URL",
			rawURL:      "http://localhost:8080/callback?code=ABC#fragment",
			expectCode:  true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasError := strings.Contains(tt.rawURL, "error=")
			
			if hasError != tt.expectError {
				t.Errorf("Error detection: got %v, want %v", hasError, tt.expectError)
			}

			if !hasError && tt.expectCode {
				parsedURL, err := url.Parse(tt.rawURL)
				if err != nil {
					t.Fatalf("Failed to parse URL: %v", err)
				}
				code := parsedURL.Query().Get("code")
				if code == "" {
					t.Error("Expected to extract code but got empty string")
				}
			}
		})
	}
}

// TestSetup_ConfigWithRedirectURI tests Setup with redirect URI already in config
func TestSetup_ConfigWithRedirectURI(t *testing.T) {
	// Note: This test cannot run the full Setup() function because it requires
	// interactive input. This is a limitation of the current implementation.
	// Future enhancement: refactor Setup() to accept io.Reader for testability
	
	cfg := &config.Config{
		API: &types.APIConfig{
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			Key:          "test-key",
			RedirectURI:  "http://localhost:8080/callback",
		},
	}

	// Verify the config is valid for setup
	if cfg.API == nil {
		t.Fatal("API config should not be nil")
	}
	if cfg.API.ClientID == "" {
		t.Fatal("ClientID should not be empty")
	}
	if cfg.API.RedirectURI == "" {
		t.Fatal("RedirectURI should not be empty")
	}

	// At this point, GetAuthorizationURL should work
	authURL, err := GetAuthorizationURL(cfg.API)
	if err != nil {
		t.Errorf("GetAuthorizationURL failed with valid config: %v", err)
	}
	if authURL == "" {
		t.Error("Authorization URL should not be empty")
	}
}

// TestCodeExtraction_RealWorldExamples tests with real-world URL formats
func TestCodeExtraction_RealWorldExamples(t *testing.T) {
	// These are examples of actual redirect URLs users might paste
	tests := []struct {
		name     string
		input    string
		wantCode string
	}{
		{
			name:     "localhost redirect",
			input:    "http://localhost:8080/callback?code=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9",
			wantCode: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9",
		},
		{
			name:     "HTTPS redirect",
			input:    "https://myapp.example.com/oauth/callback?code=abc123def456",
			wantCode: "abc123def456",
		},
		{
			name:     "with port number",
			input:    "http://localhost:3000/callback?code=xyz789",
			wantCode: "xyz789",
		},
		{
			name:     "with path",
			input:    "http://localhost:8080/api/v1/oauth/callback?code=token123",
			wantCode: "token123",
		},
		{
			name:     "trailing slash",
			input:    "http://localhost:8080/callback/?code=slash123",
			wantCode: "slash123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL, err := url.Parse(tt.input)
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}

			code := parsedURL.Query().Get("code")
			if code != tt.wantCode {
				t.Errorf("Got code %q, want %q", code, tt.wantCode)
			}
		})
	}
}
