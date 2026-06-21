package config

import (
	"os"
	"path/filepath"
	"strings"
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
				if len(cfg.Systems) != 2 {
					t.Errorf("len(Systems) = %d, want 2", len(cfg.Systems))
				}
				if cfg.RefreshIntervalSeconds != 3600 {
					t.Errorf("RefreshIntervalSeconds = %d, want 3600", cfg.RefreshIntervalSeconds)
				}
				if cfg.RefreshIntervalClampedFromSeconds != 0 {
					t.Errorf("RefreshIntervalClampedFromSeconds = %d, want 0 (no clamp for valid interval)", cfg.RefreshIntervalClampedFromSeconds)
				}
			},
		},
		{
			name: "missing systems",
			yamlContent: `
systems: []
`,
			wantErr:     true,
			errContains: "no systems configured",
		},
		{
			name: "system without id",
			yamlContent: `
systems:
  - name: System 1
`,
			wantErr:     true,
			errContains: constants.ErrInvalidSystemID,
		},
		{
			name: "default refresh interval",
			yamlContent: `
systems:
  - name: System 1
    id: "12345"
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.RefreshIntervalSeconds != 3600 {
					t.Errorf("RefreshIntervalSeconds = %d, want 3600 (default)", cfg.RefreshIntervalSeconds)
				}
			},
		},
		{
			name: "negative refresh interval defaults to 3600",
			yamlContent: `
systems:
  - name: System 1
    id: "12345"
refresh_interval: -1
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.RefreshIntervalSeconds != 3600 {
					t.Errorf("RefreshIntervalSeconds = %d, want 3600 (should default)", cfg.RefreshIntervalSeconds)
				}
			},
		},
		{
			name: "below-minimum refresh interval is clamped to the 60s floor",
			yamlContent: `
systems:
  - name: System 1
    id: "12345"
refresh_interval: 30
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.RefreshIntervalSeconds != constants.APIBudgetWindowSeconds {
					t.Errorf("RefreshIntervalSeconds = %d, want %d (clamped to floor)", cfg.RefreshIntervalSeconds, constants.APIBudgetWindowSeconds)
				}
				if cfg.RefreshIntervalClampedFromSeconds != 30 {
					t.Errorf("RefreshIntervalClampedFromSeconds = %d, want 30 (original pre-clamp value)", cfg.RefreshIntervalClampedFromSeconds)
				}
			},
		},
		{
			name:        "invalid YAML syntax",
			yamlContent: `this is not valid yaml: [[[`,
			wantErr:     true,
			errContains: "failed to parse config file",
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
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
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

// TestMergeWithDefaults tests that empty fields are filled from defaults.
func TestMergeWithDefaults(t *testing.T) {
	defaults := ColorConfig{
		Production:    "default-prod",
		Discharge:     "default-discharge",
		Import:        "default-import",
		Export:        "default-export",
		NetImport:     "default-netimport",
		NetExport:     "default-netexport",
		Headers:       "default-headers",
		Charge:        "default-charge",
		TotalConsumed: "default-total",
		SecondaryText: "default-secondary",
		PrimaryText:   "default-primary",
		Error:         "default-error",
	}

	t.Run("all empty fields filled from defaults", func(t *testing.T) {
		c := ColorConfig{}
		c.MergeWithDefaults(defaults)
		if c.Production != "default-prod" {
			t.Errorf("Production = %q, want %q", c.Production, "default-prod")
		}
		if c.Import != "default-import" {
			t.Errorf("Import = %q, want %q", c.Import, "default-import")
		}
		if c.Error != "default-error" {
			t.Errorf("Error = %q, want %q", c.Error, "default-error")
		}
	})

	t.Run("existing fields are not overwritten", func(t *testing.T) {
		c := ColorConfig{
			Production: "my-prod",
			Import:     "my-import",
		}
		c.MergeWithDefaults(defaults)
		if c.Production != "my-prod" {
			t.Errorf("Production = %q, want %q (should not be overwritten)", c.Production, "my-prod")
		}
		if c.Import != "my-import" {
			t.Errorf("Import = %q, want %q (should not be overwritten)", c.Import, "my-import")
		}
		// Other empty fields should still get defaults
		if c.Discharge != "default-discharge" {
			t.Errorf("Discharge = %q, want %q", c.Discharge, "default-discharge")
		}
	})
}

// TestConvertHexFields tests that hex color codes in all fields are converted to ANSI.
func TestConvertHexFields(t *testing.T) {
	c := ColorConfig{
		Production: "#FF0000",
		Import:     "#00FF00",
		Export:     "not-hex", // non-hex should pass through
	}
	c.convertHexFields()

	// After conversion, hex fields should no longer start with #
	if len(c.Production) > 0 && c.Production[0] == '#' {
		t.Errorf("convertHexFields() Production still starts with #: %q", c.Production)
	}
	if len(c.Import) > 0 && c.Import[0] == '#' {
		t.Errorf("convertHexFields() Import still starts with #: %q", c.Import)
	}
	// Non-hex should be unchanged
	if c.Export != "not-hex" {
		t.Errorf("convertHexFields() Export = %q, want %q", c.Export, "not-hex")
	}
}
