// Package types provides shared type definitions used across multiple packages.
//
// PURPOSE
// -------
// This package breaks circular dependencies by providing common types that can
// be imported by any package. This is a standard Go pattern called "shared types"
// or "domain types".
//
// GO FILE ORGANIZATION
// --------------------
// In Go, types are often separated into dedicated files or packages when:
//  1. Multiple packages need the same types (circular dependency avoidance)
//  2. Types represent core domain concepts (clean architecture)
//  3. The types file would otherwise become too large
//
// This pattern is called "type extraction" or "shared types package".
//
// TYPES IN THIS PACKAGE
// ---------------------
// The following types are defined here because they are used by multiple packages:
//   - SystemConfig: Used by config, aggregator, app packages
//   - APIConfig: Used by config, aggregator, oauth, app packages
//
// Types that remain in their original packages:
//   - ColorConfig: Stays in config (specific to config loading with YAML tags)
//   - Config: Stays in config (the root config struct with all settings)
//   - AggregatedMetrics: Stays in aggregator (specific to aggregation logic)
//   - SystemMetrics: Stays in aggregator (specific to aggregation logic)
package types

// SystemConfig represents configuration for a single Enphase system.
// Used by config, aggregator, and app packages.
type SystemConfig struct {
	Name string `yaml:"name"`        // Human-readable name for the system
	ID   string `yaml:"id"`          // Enlighten system ID (required for Cloud API)
}

// APIConfig represents API configuration for Enphase Cloud API.
// Used by config, aggregator, oauth, and app packages.
type APIConfig struct {
	Key              string `yaml:"key"`                     // API Key
	ClientID         string `yaml:"client_id"`               // OAuth Client ID
	ClientSecret     string `yaml:"client_secret"`           // OAuth Client Secret
	AuthorizationURL string `yaml:"authorization_url"`       // OAuth authorization URL
	RedirectURI      string `yaml:"redirect_uri,omitempty"`  // OAuth redirect URI (for authorization code flow)
	RefreshToken     string `yaml:"refresh_token,omitempty"` // OAuth refresh token (obtained from initial authorization)
	Username         string `yaml:"username,omitempty"`      // Enlighten username (for password grant - Partner/Installer only)
	Password         string `yaml:"password,omitempty"`      // Enlighten password (for password grant - Partner/Installer only)
}
