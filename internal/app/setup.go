// Package app - setup.go
//
// PURPOSE
// -------
// This file contains application setup and initialization functions.
// Handles configuration loading, OAuth adapter creation, display setup, and mode configuration.
//
// SETUP FUNCTIONS
// ---------------
// These functions extract common initialization logic from main() to improve readability:
//   - CreateOAuthAdapter: Creates OAuth token adapter for aggregator
//   - SetupDisplay: Configures display with colors from config
//   - ConfigureModes: Sets up test mode and cache mode flags
//   - ParseTestDate: Parses and validates test date parameter
package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/display"
	"enphase-monitor/internal/oauth"
	"enphase-monitor/internal/timezone"
)

// CreateOAuthAdapter creates an adapter function that converts between main and aggregator OAuth types
func CreateOAuthAdapter() aggregator.OAuthTokenGetter {
	return func(ctx context.Context, apiConfig *aggregator.APIConfig) (string, error) {
		// Convert aggregator.APIConfig to config.APIConfig
		mainAPIConfig := &config.APIConfig{
			Key:              apiConfig.Key,
			ClientID:         apiConfig.ClientID,
			ClientSecret:     apiConfig.ClientSecret,
			AuthorizationURL: apiConfig.AuthorizationURL,
			RedirectURI:      apiConfig.RedirectURI,
			RefreshToken:     apiConfig.RefreshToken,
			Username:         apiConfig.Username,
			Password:         apiConfig.Password,
		}
		return oauth.GetAccessToken(ctx, mainAPIConfig)
	}
}

// SetupDisplay creates a Display instance with colors from config
func SetupDisplay(cfg *config.Config, reportTZ *time.Location) *display.Display {
	colors := display.GetDefaultColors()
	if cfg.Colors != nil {
		// Convert main.ColorConfig to config.ColorConfig
		colors.Production = cfg.Colors.Production
		colors.Discharge = cfg.Colors.Discharge
		colors.Import = cfg.Colors.Import
		colors.Export = cfg.Colors.Export
		colors.NetImport = cfg.Colors.NetImport
		colors.NetExport = cfg.Colors.NetExport
		colors.Headers = cfg.Colors.Headers
		colors.Charge = cfg.Colors.Charge
		colors.TotalConsumed = cfg.Colors.TotalConsumed
		colors.SecondaryText = cfg.Colors.SecondaryText
		colors.PrimaryText = cfg.Colors.PrimaryText
		colors.Error = cfg.Colors.Error
	}
	return display.NewDisplayWithColorsAndTimezone(colors, reportTZ)
}

// ConfigureModes sets up test mode and cache mode based on flags
func ConfigureModes(testMode, noCache bool) {
	if testMode {
		cache.SetTestMode(true)
		fmt.Println("TEST MODE: Using cache only, no live API calls")
	}
	
	if noCache {
		cache.SetCacheDisabled(true)
		fmt.Println("NO-CACHE MODE: Bypassing cache, making live API calls")
	}
}

// ParseTestDate parses the test date string and returns a time.Time value
// Returns zero value if date string is empty (meaning use today)
func ParseTestDate(dateStr string, reportTZ *time.Location) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, nil
	}
	
	// Parse date using the reporting timezone
	parsed, err := timezone.ParseDateInTimezone(dateStr, reportTZ)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format. Use YYYY-MM-DD (e.g. 2026-01-19): %w", err)
	}
	return parsed, nil
}

// ConvertToAggregatorTypes converts main package config types to aggregator package types
func ConvertToAggregatorTypes(cfg *config.Config) ([]aggregator.SystemConfig, *aggregator.APIConfig) {
	// Convert SystemConfig to aggregator.SystemConfig
	aggSystems := make([]aggregator.SystemConfig, len(cfg.Systems))
	for i, sys := range cfg.Systems {
		aggSystems[i] = aggregator.SystemConfig{
			Name: sys.Name,
			ID:   sys.ID,
		}
	}

	// Convert APIConfig to aggregator.APIConfig
	aggAPIConfig := &aggregator.APIConfig{
		Key:              cfg.API.Key,
		ClientID:         cfg.API.ClientID,
		ClientSecret:     cfg.API.ClientSecret,
		AuthorizationURL: cfg.API.AuthorizationURL,
		RedirectURI:      cfg.API.RedirectURI,
		RefreshToken:     cfg.API.RefreshToken,
		Username:         cfg.API.Username,
		Password:         cfg.API.Password,
	}

	return aggSystems, aggAPIConfig
}

// ExitWithError prints an error message and exits with code 1
func ExitWithError(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}
