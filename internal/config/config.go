// Package config handles configuration loading, validation, and color customization.
//
// PURPOSE
// -------
// This file manages all application configuration, including:
//   - Loading YAML configuration files
//   - Validating required fields and data types
//   - Converting hex color codes to ANSI escape sequences
//   - Providing default values where appropriate
//
// CONFIGURATION TEMPLATE
// ----------------------
// See config.yaml.example for a complete configuration template with detailed comments.
// For setup instructions, see QUICKSTART.md or README.md
//
// CONFIGURATION STRUCTURE
// -----------------------
// The configuration is defined in config.yaml with three main sections:
//
//  1. API Configuration (api:)
//     - OAuth credentials (key, client_id, client_secret)
//     - OAuth settings (authorization_url, redirect_uri)
//     - Refresh token (obtained from --oauth-setup)
//
//  2. Systems Configuration (systems:)
//     - List of Systems to monitor
//     - Each system requires: name and ID (system ID from Enlighten)
//
//  3. Application Settings
//     - refresh_interval: How often to query API (seconds)
//     - timezone: Optional timezone for reporting/display (e.g., "US/Pacific")
//     - colors: Optional color customization (hex codes or ANSI)
//
// VALIDATION
// ----------
// LoadConfig() performs comprehensive validation:
//   - Ensures all required API fields are present
//   - Validates each system has a name and ID
//   - Checks refresh_interval is positive
//   - Trims whitespace from refresh_token (common copy/paste issue)
//   - Sets default refresh_interval to 3600 (1 hour) if not specified
//
// COLOR CUSTOMIZATION
// -------------------
// Colors can be specified in two formats:
//  1. Hex codes: "#FF5733" (automatically converted to ANSI)
//  2. ANSI codes: "\033[38;5;208m" (used as-is)
//
// Hex codes are converted using convertHexFields() which:
//   - Parses hex string (with or without #)
//   - Converts RGB to ANSI 256-color code
//   - Falls back to closest ANSI color if exact match not available
//
// If colors are not specified, defaults from display.GetDefaultColors() are used.
package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/types"

	"gopkg.in/yaml.v3"
)

// SystemConfig is an alias for types.SystemConfig.
// This maintains backward compatibility while using the shared type definition.
// See internal/types/types.go for the type definition.
type SystemConfig = types.SystemConfig

// APIConfig is an alias for types.APIConfig.
// This maintains backward compatibility while using the shared type definition.
// See internal/types/types.go for the type definition.
type APIConfig = types.APIConfig

// ColorConfig represents color customization settings.
// Note: Reset and Bold are defined as constants in constants.go and cannot be customized.
type ColorConfig struct {
	Production          string `yaml:"production,omitempty"`            // Solar Production
	Discharge           string `yaml:"discharge,omitempty"`             // Battery Discharge
	Import              string `yaml:"import,omitempty"`                // Grid Import
	Export              string `yaml:"export,omitempty"`                // Grid Export
	NetImport           string `yaml:"net_import,omitempty"`            // Foreground color used when Net Flow is in the import direction (positive value)
	NetExport           string `yaml:"net_export,omitempty"`            // Foreground color used when Net Flow is in the export direction (negative value)
	NetImportBackground string `yaml:"net_import_background,omitempty"` // Truecolor background highlight for the Net Flow line in the import direction. Hex values are rendered as 24-bit truecolor (\033[48;2;R;G;Bm) rather than 256-color cube to preserve fidelity for backgrounds.
	NetExportBackground string `yaml:"net_export_background,omitempty"` // Truecolor background highlight for the Net Flow line in the export direction. Hex values rendered as 24-bit truecolor.
	Headers             string `yaml:"headers,omitempty"`               // Report Headers
	Charge              string `yaml:"charge,omitempty"`                // Battery Charge
	TotalConsumed       string `yaml:"total_consumed,omitempty"`        // Total Consumed
	SecondaryText       string `yaml:"secondary_text,omitempty"`        // Secondary Text
	PrimaryText         string `yaml:"primary_text,omitempty"`          // Primary Text
	Error               string `yaml:"error,omitempty"`                 // Error Text
}

// MergeWithDefaults fills in empty color fields with default values.
// Performance: Uses field iterator pattern to eliminate 36 lines of repetitive if-statements.
func (c *ColorConfig) MergeWithDefaults(defaults ColorConfig) {
	// Define field pairs for merging
	fields := []struct {
		dst, src *string
	}{
		{&c.Production, &defaults.Production},
		{&c.Discharge, &defaults.Discharge},
		{&c.Import, &defaults.Import},
		{&c.Export, &defaults.Export},
		{&c.NetImport, &defaults.NetImport},
		{&c.NetExport, &defaults.NetExport},
		{&c.NetImportBackground, &defaults.NetImportBackground},
		{&c.NetExportBackground, &defaults.NetExportBackground},
		{&c.Headers, &defaults.Headers},
		{&c.Charge, &defaults.Charge},
		{&c.TotalConsumed, &defaults.TotalConsumed},
		{&c.SecondaryText, &defaults.SecondaryText},
		{&c.PrimaryText, &defaults.PrimaryText},
		{&c.Error, &defaults.Error},
	}

	for _, f := range fields {
		if *f.dst == "" {
			*f.dst = *f.src
		}
	}
}

// convertHexFields converts hex color codes to ANSI escape codes.
// Foreground fields use the ANSI 256-color cube (compact, terminal-friendly).
// Background fields use 24-bit truecolor (\033[48;2;R;G;Bm) because the
// 6×6×6 cube quantizes coarsely (only 216 colors) — fine for foreground text
// where the eye tolerates approximation, but visibly wrong for solid
// background fills the user is trying to match by hex code.
func (c *ColorConfig) convertHexFields() {
	foregroundFields := []*string{
		&c.Production,
		&c.Discharge,
		&c.Import,
		&c.Export,
		&c.NetImport,
		&c.NetExport,
		&c.Headers,
		&c.Charge,
		&c.TotalConsumed,
		&c.SecondaryText,
		&c.PrimaryText,
		&c.Error,
	}
	for _, field := range foregroundFields {
		*field = convertIfHex(*field)
	}

	backgroundFields := []*string{
		&c.NetImportBackground,
		&c.NetExportBackground,
	}
	for _, field := range backgroundFields {
		*field = convertIfHexBackground(*field)
	}
}

// Config represents the application configuration.
type Config struct {
	API                    *APIConfig     `yaml:"api,omitempty"` // API configuration
	Systems                []SystemConfig `yaml:"systems"`
	RefreshIntervalSeconds int            `yaml:"refresh_interval"`   // How often to query API (seconds)
	Colors                 *ColorConfig   `yaml:"colors,omitempty"`   // Color customization
	Timezone               string         `yaml:"timezone,omitempty"` // Timezone for reporting/display (e.g., "US/Pacific"). If not set, uses system timezone.
}

// LoadConfig reads and parses the configuration file.
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate required fields
	if len(config.Systems) == 0 {
		return nil, fmt.Errorf("no systems configured")
	}

	for i, sys := range config.Systems {
		// System must have id (Cloud API)
		if sys.ID == "" {
			return nil, fmt.Errorf("%s (system %d: %s) for Cloud API", constants.ErrInvalidSystemID, i, sys.Name)
		}
		// API credentials are required for Cloud API
		if config.API == nil {
			return nil, fmt.Errorf("%s for system %d (%s) using Cloud API", constants.ErrAPIConfigRequired, i, sys.Name)
		}
		if config.API.Key == "" || config.API.ClientID == "" || config.API.ClientSecret == "" {
			return nil, fmt.Errorf("api.key, api.client_id, and api.client_secret required for system %d (%s) using Cloud API", i, sys.Name)
		}
	}

	// Default refresh interval to 1 hour if not specified or invalid
	if config.RefreshIntervalSeconds <= 0 {
		config.RefreshIntervalSeconds = 3600
	}

	// Trim whitespace from refresh token (common issue with copy/paste)
	if config.API.RefreshToken != "" {
		config.API.RefreshToken = strings.TrimSpace(config.API.RefreshToken)
	}

	// Convert hex color codes to ANSI codes if needed
	if config.Colors != nil {
		config.Colors.convertHexFields()
	}

	return &config, nil
}

// convertIfHex converts a hex color code to ANSI escape code if needed.
// If a value is already an ANSI code (starts with \033), it is left unchanged.
// If a value is a hex code (starts with #), it is converted to a 256-color
// foreground code via the 6×6×6 cube.
func convertIfHex(hex string) string {
	if hex == "" {
		return hex
	}
	if strings.HasPrefix(hex, "\033") {
		return hex
	}
	if strings.HasPrefix(hex, "#") {
		return hexToANSI(hex)
	}
	return hex
}

// convertIfHexBackground is the background counterpart of convertIfHex.
// Hex values are rendered as 24-bit truecolor backgrounds
// (\033[48;2;R;G;Bm) rather than the 256-color cube, because backgrounds
// fill solid areas where quantization is visually obvious.
func convertIfHexBackground(hex string) string {
	if hex == "" {
		return hex
	}
	if strings.HasPrefix(hex, "\033") {
		return hex
	}
	if strings.HasPrefix(hex, "#") {
		return hexToANSIBackground(hex)
	}
	return hex
}

// hexToANSIBackground converts a hex color code to a 24-bit truecolor ANSI
// background escape (\033[48;2;R;G;Bm). Returns "" on malformed input —
// MergeWithDefaults will then refill the field from the package defaults.
func hexToANSIBackground(hex string) string {
	r, g, b, ok := parseHexRGB(hex)
	if !ok {
		return ""
	}
	return "\033[48;2;" +
		strconv.FormatInt(r, 10) + ";" +
		strconv.FormatInt(g, 10) + ";" +
		strconv.FormatInt(b, 10) + "m"
}

// parseHexRGB parses a "#RRGGBB" or "#RGB" string into 0–255 RGB components.
// Returns ok=false if the input has the wrong length or non-hex digits.
func parseHexRGB(hex string) (r, g, b int64, ok bool) {
	hex = strings.TrimPrefix(hex, "#")
	hex = strings.ToUpper(hex)
	if len(hex) != 6 && len(hex) != 3 {
		return 0, 0, 0, false
	}
	if len(hex) == 6 {
		var err error
		r, err = strconv.ParseInt(hex[0:2], constants.HexBase, 64)
		if err != nil {
			return 0, 0, 0, false
		}
		g, err = strconv.ParseInt(hex[2:4], constants.HexBase, 64)
		if err != nil {
			return 0, 0, 0, false
		}
		b, err = strconv.ParseInt(hex[4:6], constants.HexBase, 64)
		if err != nil {
			return 0, 0, 0, false
		}
		return r, g, b, true
	}
	// len(hex) == 3 — short form #RGB
	rVal, err1 := strconv.ParseInt(string(hex[0]), constants.HexBase, 64)
	gVal, err2 := strconv.ParseInt(string(hex[1]), constants.HexBase, 64)
	bVal, err3 := strconv.ParseInt(string(hex[2]), constants.HexBase, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return rVal*16 + rVal, gVal*16 + gVal, bVal*16 + bVal, true
}

// hexToANSI converts a hex color code (e.g., "#FF5733") to ANSI 256-color escape code
// Uses the 6x6x6 color cube (216 colors) from the ANSI 256-color palette
func hexToANSI(hex string) string {
	r, g, b, ok := parseHexRGB(hex)
	if !ok {
		return ""
	}

	// Convert RGB (0-255) to ANSI 256-color code (16-231 for 6x6x6 cube)
	// ANSI 256-color palette: base + redMultiplier*r + levels*g + b
	// Where r, g, b are in range 0-5 (6 levels each)
	// Map 0-255 to 0-5: use value * 5 / 255 for better accuracy
	r6 := int(math.Round((float64(r) * float64(constants.ANSIColorCubeLevels-1)) / float64(constants.RGBMaxValue)))
	g6 := int(math.Round((float64(g) * float64(constants.ANSIColorCubeLevels-1)) / float64(constants.RGBMaxValue)))
	b6 := int(math.Round((float64(b) * float64(constants.ANSIColorCubeLevels-1)) / float64(constants.RGBMaxValue)))

	// Clamp to 0-5 range using Go 1.21 built-in min/max
	r6 = max(0, min(constants.ANSIColorCubeLevels-1, r6))
	g6 = max(0, min(constants.ANSIColorCubeLevels-1, g6))
	b6 = max(0, min(constants.ANSIColorCubeLevels-1, b6))

	// Calculate ANSI color code (16-231 for color cube)
	ansiCode := constants.ANSIColorCubeBase + constants.ANSIColorCubeRedMultiplier*r6 + constants.ANSIColorCubeLevels*g6 + b6

	// Return ANSI escape code (strconv.Itoa is faster than fmt.Sprintf for single int)
	return "\033[38;5;" + strconv.Itoa(ansiCode) + "m"
}
