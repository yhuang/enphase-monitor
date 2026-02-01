// Package config - config_test.go
//
// TEST SETUP
// ----------
// This test suite validates YAML configuration loading and validation.
// Tests create temporary YAML files to avoid polluting the workspace.
//
// TEST PLAN
// ---------
// 1. Valid Configuration Tests
//   - Test complete config with all fields
//   - Test minimal config with required fields only
//   - Test default values (refresh_interval, timezone)
//
// 2. Validation Tests
//   - Test missing required fields (API key, client_id, systems)
//   - Test empty system ID (should fail)
//   - Test invalid YAML syntax
//
// 3. Color Conversion Tests
//   - Test hex color codes (e.g., #FF5733) are converted to ANSI
//   - Test ANSI codes are preserved as-is
//   - Test default colors are applied when not specified
//
// 4. Whitespace Handling Tests
//   - Test leading/trailing whitespace is trimmed
//   - Test refresh_token whitespace is cleaned
//
// TESTING APPROACH
// ----------------
// - Table-driven tests with inline YAML strings
// - Create temporary files with os.WriteFile()
// - Clean up temp files with defer os.Remove()
// - Use validation functions to verify loaded config
//
// YAML PARSING
// ------------
// Uses gopkg.in/yaml.v3 for YAML parsing.
// Struct tags map YAML field names to Go struct fields.
//
// PATTERN USED
// ------------
// - Pattern 1: Table-Driven Tests
// - Pattern 3: Subtests with t.Run()
//
// See TESTING.md for detailed pattern explanations.
package config

import (
	"os"
	"path/filepath"
	"testing"

	"enphase-monitor/internal/constants"
)

// TestLoadConfig tests configuration loading and validation
func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		yamlContent string
		wantErr     bool
		errContains string
		validate    func(*testing.T, *Config)
	}{
		{
			name: "valid config with all fields",
			yamlContent: `
api:
  key: test-key
  client_id: test-client
  client_secret: test-secret
  authorization_url: https://api.enphaseenergy.com/oauth/token
  refresh_token: test-refresh-token
systems:
  - name: System 1
    id: "12345"
  - name: System 2
    id: "67890"
refresh_interval: 3600
timezone: US/Pacific
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.API.Key != "test-key" {
					t.Errorf("API.Key = %s, want test-key", cfg.API.Key)
				}
				if len(cfg.Systems) != 2 {
					t.Errorf("len(Systems) = %d, want 2", len(cfg.Systems))
				}
				if cfg.RefreshInterval != 3600 {
					t.Errorf("RefreshInterval = %d, want 3600", cfg.RefreshInterval)
				}
			},
		},
		{
			name: "missing systems",
			yamlContent: `
api:
  key: test-key
  client_id: test-client
  client_secret: test-secret
systems: []
`,
			wantErr:     true,
			errContains: "no systems configured",
		},
		{
			name: "system without id",
			yamlContent: `
api:
  key: test-key
  client_id: test-client
  client_secret: test-secret
systems:
  - name: System 1
`,
			wantErr:     true,
			errContains: constants.ErrInvalidSystemID,
		},
		{
			name: "missing api config",
			yamlContent: `
systems:
  - name: System 1
    id: "12345"
`,
			wantErr:     true,
			errContains: constants.ErrAPIConfigRequired,
		},
		{
			name: "default refresh interval",
			yamlContent: `
api:
  key: test-key
  client_id: test-client
  client_secret: test-secret
systems:
  - name: System 1
    id: "12345"
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.RefreshInterval != 3600 {
					t.Errorf("RefreshInterval = %d, want 3600 (default)", cfg.RefreshInterval)
				}
			},
		},
		{
			name: "negative refresh interval defaults to 3600",
			yamlContent: `
api:
  key: test-key
  client_id: test-client
  client_secret: test-secret
systems:
  - name: System 1
    id: "12345"
refresh_interval: -1
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.RefreshInterval != 3600 {
					t.Errorf("RefreshInterval = %d, want 3600 (should default)", cfg.RefreshInterval)
				}
			},
		},
		{
			name: "refresh token with whitespace is trimmed",
			yamlContent: `
api:
  key: test-key
  client_id: test-client
  client_secret: test-secret
  refresh_token: "  token-with-spaces  "
systems:
  - name: System 1
    id: "12345"
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.API.RefreshToken != "token-with-spaces" {
					t.Errorf("RefreshToken = %q, want %q (trimmed)", cfg.API.RefreshToken, "token-with-spaces")
				}
			},
		},
		{
			name:        "invalid YAML syntax",
			yamlContent: `this is not valid yaml: [[[`,
			wantErr:     true,
			errContains: "failed to parse config file",
		},
		{
			name: "missing API key",
			yamlContent: `
api:
  client_id: test-client
  client_secret: test-secret
systems:
  - name: System 1
    id: "12345"
`,
			wantErr:     true,
			errContains: "api.key",
		},
		{
			name: "missing client_id",
			yamlContent: `
api:
  key: test-key
  client_secret: test-secret
systems:
  - name: System 1
    id: "12345"
`,
			wantErr:     true,
			errContains: "client_id",
		},
		{
			name: "missing client_secret",
			yamlContent: `
api:
  key: test-key
  client_id: test-client
systems:
  - name: System 1
    id: "12345"
`,
			wantErr:     true,
			errContains: "client_secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.yamlContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create temp config: %v", err)
			}

			// Load config
			cfg, err := LoadConfig(configPath)

			// Check error expectation
			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadConfig() error = nil, want error containing %q", tt.errContains)
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("LoadConfig() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("LoadConfig() unexpected error = %v", err)
				return
			}

			// Run validation if provided
			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

// TestConvertIfHex tests hex color conversion
func TestConvertIfHex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "hex color with hash",
			input:    "#FF0000",
			expected: "\033[38;5;196m", // Red in ANSI 256-color
		},
		{
			name:     "hex color without hash",
			input:    "FF0000",
			expected: "FF0000", // Not recognized without #
		},
		{
			name:     "ANSI escape code",
			input:    "\033[38;5;208m",
			expected: "\033[38;5;208m", // Pass through unchanged
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "short hex color",
			input:    "#F00",
			expected: "\033[38;5;196m", // Red
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertIfHex(tt.input)
			if result != tt.expected {
				t.Errorf("convertIfHex(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestHexToANSI tests hex to ANSI color conversion
func TestHexToANSI(t *testing.T) {
	tests := []struct {
		name     string
		hex      string
		expected string
	}{
		{
			name:     "red",
			hex:      "#FF0000",
			expected: "\033[38;5;196m",
		},
		{
			name:     "green",
			hex:      "#00FF00",
			expected: "\033[38;5;46m",
		},
		{
			name:     "blue",
			hex:      "#0000FF",
			expected: "\033[38;5;21m",
		},
		{
			name:     "white",
			hex:      "#FFFFFF",
			expected: "\033[38;5;231m",
		},
		{
			name:     "black",
			hex:      "#000000",
			expected: "\033[38;5;16m",
		},
		{
			name:     "short format red",
			hex:      "#F00",
			expected: "\033[38;5;196m",
		},
		{
			name:     "invalid hex",
			hex:      "#XYZ",
			expected: "",
		},
		{
			name:     "empty string",
			hex:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hexToANSI(tt.hex)
			if result != tt.expected {
				t.Errorf("hexToANSI(%q) = %q, want %q", tt.hex, result, tt.expected)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
