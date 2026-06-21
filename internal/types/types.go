// Package types provides shared type definitions used across multiple packages.
//
// PURPOSE
// -------
// This package breaks circular dependencies by providing common types that can
// be imported by any package. This is a standard Go pattern called "shared types"
// or "domain types".
//
// NAMING CONVENTION
// -----------------
// The package is named "types" rather than "shared" because this is the idiomatic
// Go convention. Examples from the Go ecosystem:
//   - go/types (Go's type checker)
//   - k8s.io/apimachinery/pkg/types (Kubernetes)
//   - google.golang.org/protobuf/types (Protocol Buffers)
//
// The package name describes WHAT it contains (type definitions), not HOW it is
// used (shared). The "internal/" prefix already indicates these are implementation
// details, and the documentation explains the sharing purpose.
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
// PACKAGE-SPECIFIC VS SHARED TYPES
// --------------------------------
// This codebase has TWO different patterns for types.go files:
//
// 1. PACKAGE-SPECIFIC types.go (e.g., internal/aggregator/types.go, internal/api/types.go)
//   - Contains types used ONLY within that package
//   - Example: internal/api/types.go defines LocalMetrics (only used by api package)
//   - Example: internal/aggregator/types.go defines AggregatedMetrics, SystemMetrics
//   - These are standard Go file organization - separating types from implementation
//
// 2. SHARED types package (this file: internal/types/types.go)
//   - Contains types used by MULTIPLE packages
//   - Created specifically to break circular dependencies
//   - Example: SystemConfig is needed by config, aggregator, oauth, and app packages
//   - Without this package, config would import aggregator, aggregator would import
//     config, creating a circular dependency that Go does not allow
//
// The key distinction:
//   - internal/api/types.go      → types for api package only
//   - internal/aggregator/types.go → types for aggregator package only
//   - internal/types/types.go    → types shared across multiple packages
//
// WHEN TO MOVE A TYPE HERE
// ------------------------
// Use this decision process when a package-specific type needs cross-package use:
//
//	Does package B need a type from package A?
//	└─► Does A also need something from B? (circular dependency?)
//	    ├─► NO:  Just import A from B (direct import works)
//	    │        Example: display imports aggregator.AggregatedMetrics
//	    └─► YES: Would create circular dependency
//	        ├─► Can you use an interface? → Define interface in consumer
//	        └─► Need concrete type? → Move to internal/types/
//
// See docs/GO_CONCEPTS.md for detailed flowchart and examples.
//
// TYPES IN THIS PACKAGE
// ---------------------
// The following types are defined here because they are used by multiple packages:
//   - SystemConfig: Used by config, aggregator, app packages
//   - APIConfig: Used by config, aggregator, oauth, app packages
//
// Types that remain in their original packages (package-specific):
//   - ColorConfig: Stays in config (specific to config loading with YAML tags)
//   - Config: Stays in config (the root config struct with all settings)
//   - AggregatedMetrics: Stays in aggregator (specific to aggregation logic)
//   - SystemMetrics: Stays in aggregator (specific to aggregation logic)
//   - LocalMetrics: Stays in api (specific to API response handling)
package types

// SystemConfig represents configuration for a single Enphase system.
// Used by config, aggregator, and app packages.
type SystemConfig struct {
	Name string `yaml:"name"` // Human-readable name for the system
	ID   string `yaml:"id"`   // Enlighten system ID (required for Cloud API)
}

// APIConfig represents API configuration for Enphase Cloud API.
// Used by config, aggregator, oauth, and app packages.
type APIConfig struct {
	Name             string `yaml:"name"`                    // Unique label for this credential set (names a credential for --update-refresh-token and as the token-cache key)
	Key              string `yaml:"key"`                     // API Key
	ClientID         string `yaml:"client_id"`               // OAuth Client ID
	ClientSecret     string `yaml:"client_secret"`           // OAuth Client Secret
	AuthorizationURL string `yaml:"authorization_url"`       // OAuth authorization URL
	RedirectURI      string `yaml:"redirect_uri,omitempty"`  // OAuth redirect URI (for authorization code flow)
	RefreshToken     string `yaml:"refresh_token,omitempty"` // OAuth refresh token (obtained from initial authorization)
	Username         string `yaml:"username,omitempty"`      // Enlighten username (for password grant - Partner/Installer only)
	Password         string `yaml:"password,omitempty"`      // Enlighten password (for password grant - Partner/Installer only)
}
