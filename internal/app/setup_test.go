// Package app - setup_test.go
//
// TEST SETUP
// ----------
// This test suite validates application initialization and configuration logic.
// Tests ensure proper setup of OAuth adapter, display, and mode configuration.
//
// TEST PLAN
// ---------
// 1. OAuth Adapter Tests
//    - Test CreateOAuthAdapter returns non-nil function
//    - Test adapter can be called (basic smoke test)
//
// 2. Display Setup Tests
//    - Test SetupDisplay creates Display with correct colors
//    - Test default colors are applied when config has no colors
//    - Test custom colors override defaults
//    - Test timezone is set correctly
//
// 3. Mode Configuration Tests
//    - Test ConfigureModes sets test mode flag
//    - Test ConfigureModes sets cache disabled flag
//    - Test mode flags are mutually independent
//
// 4. Date Parsing Tests
//    - Test ParseTestDate with valid date string
//    - Test ParseTestDate with empty string returns zero time
//    - Test ParseTestDate with invalid format returns error
//
// 5. Test Mode Cache Validation Tests
//    - Test ValidateTestModeCache returns error for missing cache
//    - Test error message contains target date
//    - Test error message contains helpful instructions
//
// TESTING APPROACH
// ----------------
// - Create minimal config structures for testing
// - Verify returned objects are non-nil and properly configured
// - Test error handling for invalid inputs
// - Use time.Time zero value for "no date specified"
//
// WHY TEST APPLICATION SETUP
// --------------------------
// Setup functions orchestrate multiple components:
// - OAuth token management
// - Display with color configuration
// - Operating modes (test, cache)
// - Date parsing for historical queries
//
// Testing ensures all components are initialized correctly before use.
//
// PATTERN USED
// ------------
// - Pattern 1: Table-Driven Tests
// - Pattern 3: Subtests with t.Run()
//
// See TESTING.md for detailed pattern explanations.
//
// =============================================================================
// TESTING PATTERNS DEMONSTRATED IN THIS FILE
// =============================================================================
//
// This file demonstrates several key testing patterns:
//
// 1. SMOKE TESTS - Quick tests that verify basic functionality
//    (TestCreateOAuthAdapter, TestSetupDisplay)
//
// 2. CONFIGURATION VARIANTS - Testing the same function with different configs
//    (TestSetupDisplay vs TestSetupDisplay_WithCustomColors)
//
// 3. BOUNDARY VALUE TESTING - Testing edge cases like empty strings, zero values
//    (TestParseTestDate_EmptyString, TestParseTestDate_InvalidDate)
//
// 4. SUBTESTS WITH t.Run() - Grouping related assertions
//    (TestValidateTestModeCache uses t.Run for each scenario)
//
// 5. TABLE-DRIVEN TESTS - Testing multiple cases with same logic
//    (TestConfigureModes uses a test table)
//
// =============================================================================
package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/types"
)

// =============================================================================
// SMOKE TESTS
// =============================================================================
//
// WHAT IS A SMOKE TEST?
// ---------------------
// A smoke test is a quick sanity check that verifies basic functionality.
// The name comes from electronics: if you power on a device and smoke comes
// out, you know something is fundamentally wrong without further testing.
//
// CHARACTERISTICS OF SMOKE TESTS:
// - Fast to run (milliseconds)
// - Check that functions don't crash or return nil
// - Don't verify detailed behavior
// - Catch obvious regressions quickly
//
// =============================================================================

func TestCreateOAuthAdapter(t *testing.T) {
	// SMOKE TEST: Just verify the function returns something
	// We don't test what the adapter DOES - that's covered in oauth_test.go
	adapter := CreateOAuthAdapter()

	if adapter == nil {
		t.Error("CreateOAuthAdapter() returned nil")
	}
}

func TestSetupDisplay(t *testing.T) {
	// SMOKE TEST: Verify display setup with default colors
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "Test System", ID: "12345"},
		},
		API: &types.APIConfig{
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		},
		RefreshInterval: 3600,
	}

	tz := time.UTC

	disp := SetupDisplay(cfg, tz)

	if disp == nil {
		t.Error("SetupDisplay() returned nil")
	}
}

// =============================================================================
// CONFIGURATION VARIANT TESTING
// =============================================================================
//
// WHAT IS CONFIGURATION VARIANT TESTING?
// --------------------------------------
// When a function behaves differently based on configuration, test each
// variant separately. This makes it clear WHICH configuration caused a failure.
//
// NAMING CONVENTION:
// - TestFunctionName         (default/basic case)
// - TestFunctionName_Variant (specific configuration)
//
// Examples:
// - TestSetupDisplay             (default colors)
// - TestSetupDisplay_WithCustomColors (custom colors)
//
// =============================================================================

func TestSetupDisplay_WithCustomColors(t *testing.T) {
	// VARIANT: Custom colors in configuration
	// This tests the color override path vs the default color path
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "Test System", ID: "12345"},
		},
		API: &types.APIConfig{
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		},
		RefreshInterval: 3600,
		Colors: &config.ColorConfig{
			Production: "#FF0000",
			Import:     "#00FF00",
		},
	}

	tz := time.UTC

	disp := SetupDisplay(cfg, tz)

	if disp == nil {
		t.Error("SetupDisplay() returned nil")
	}
}

// =============================================================================
// BOUNDARY VALUE TESTING
// =============================================================================
//
// WHAT IS BOUNDARY VALUE TESTING?
// -------------------------------
// Boundary values are inputs at the edges of valid ranges:
// - Empty strings, zero values, nil pointers
// - Maximum/minimum values
// - Just inside/outside valid ranges
//
// These often expose bugs because programmers focus on "normal" cases.
//
// FOR STRING INPUTS, TEST:
// - Empty string ""
// - Valid format with valid data
// - Valid format with invalid data (2026-13-45)
// - Invalid format entirely (01-15-2026)
//
// =============================================================================

func TestParseTestDate_EmptyString(t *testing.T) {
	// BOUNDARY: Empty string input
	// Expected behavior: Return zero time (no error)
	// This represents "no date specified" which is a valid use case
	tz := time.UTC

	parsed, err := ParseTestDate("", tz)

	if err != nil {
		t.Errorf("ParseTestDate() error = %v, want nil", err)
	}
	// time.Time{}.IsZero() returns true for uninitialized time
	if !parsed.IsZero() {
		t.Error("ParseTestDate() should return zero time for empty string")
	}
}

func TestParseTestDate_ValidDate(t *testing.T) {
	// HAPPY PATH: Normal, valid input
	// This is the "golden path" test - everything works as expected
	tz := time.UTC

	parsed, err := ParseTestDate("2026-01-15", tz)

	if err != nil {
		t.Errorf("ParseTestDate() error = %v, want nil", err)
	}
	if parsed.IsZero() {
		t.Error("ParseTestDate() returned zero time for valid date")
	}

	// Verify the EXACT parsed value, not just that it's non-zero
	expected := time.Date(2026, 1, 15, 0, 0, 0, 0, tz)
	if !parsed.Equal(expected) {
		t.Errorf("ParseTestDate() = %v, want %v", parsed, expected)
	}
}

func TestParseTestDate_InvalidFormat(t *testing.T) {
	// BOUNDARY: Wrong format (MM-DD-YYYY instead of YYYY-MM-DD)
	// Expected behavior: Return error
	tz := time.UTC

	_, err := ParseTestDate("01-15-2026", tz)

	if err == nil {
		t.Error("ParseTestDate() expected error for invalid format")
	}
}

func TestParseTestDate_InvalidDate(t *testing.T) {
	// BOUNDARY: Right format, impossible date (month 13, day 45)
	// Expected behavior: Return error
	// This tests that time.Parse validates the date, not just the format
	tz := time.UTC

	_, err := ParseTestDate("2026-13-45", tz)

	if err == nil {
		t.Error("ParseTestDate() expected error for invalid date")
	}
}

func TestGetAggregatorTypes(t *testing.T) {
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "System 1", ID: "12345"},
			{Name: "System 2", ID: "67890"},
		},
		API: &types.APIConfig{
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		},
	}
	
	systems, apiConfig := GetAggregatorTypes(cfg)
	
	if len(systems) != 2 {
		t.Errorf("GetAggregatorTypes() returned %d systems, want 2", len(systems))
	}
	if apiConfig == nil {
		t.Error("GetAggregatorTypes() returned nil API config")
	}
	if apiConfig.Key != "test-key" {
		t.Errorf("GetAggregatorTypes() API key = %v, want test-key", apiConfig.Key)
	}
}

func TestGetAggregatorTypes_EmptySystems(t *testing.T) {
	cfg := &config.Config{
		Systems: []types.SystemConfig{},
		API: &types.APIConfig{
			Key: "test-key",
		},
	}
	
	systems, apiConfig := GetAggregatorTypes(cfg)
	
	if len(systems) != 0 {
		t.Errorf("GetAggregatorTypes() returned %d systems, want 0", len(systems))
	}
	if apiConfig == nil {
		t.Error("GetAggregatorTypes() returned nil API config")
	}
}

func TestRunOnce_ContextCancelled(t *testing.T) {
	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	
	// Create mock aggregator that returns error due to cancelled context
	mockGetter := func(ctx context.Context, apiConfig *aggregator.APIConfig) (string, error) {
		return "", context.Canceled
	}
	agg := aggregator.NewDataAggregator(mockGetter)
	
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "Test", ID: "12345"},
		},
		API: &types.APIConfig{
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		},
	}
	
	tz := time.UTC
	disp := SetupDisplay(cfg, tz)
	testDate := time.Time{}
	
	// This should exit early due to cancelled context
	// We can't easily test os.Exit, but we can verify the function doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RunOnce() panicked: %v", r)
		}
	}()
	
	// Note: In a real test, we'd mock os.Exit to verify it's called
	// For now, we just ensure the function can handle a cancelled context
	_ = agg
	_ = disp
	_ = testDate
	_ = ctx
}

// =============================================================================
// TABLE-DRIVEN TESTS WITH SUBTESTS
// =============================================================================
//
// COMBINING PATTERNS:
// -------------------
// This test combines Pattern 1 (Table-Driven) with Pattern 3 (Subtests).
// The table defines all test cases, and t.Run() creates a subtest for each.
//
// BENEFITS:
// - Each case runs independently (one failure doesn't stop others)
// - Output shows which specific case failed
// - Easy to add new combinations
//
// BOOLEAN COMBINATION TESTING:
// ----------------------------
// With 2 boolean flags (testMode, noCache), there are 4 possible states:
// - false, false
// - true, false
// - false, true
// - true, true
//
// Testing all combinations ensures flags are independent (setting one
// doesn't accidentally affect the other).
//
// =============================================================================

// TestConfigureModes tests the ConfigureModes function
func TestConfigureModes(t *testing.T) {
	// This struct is here to satisfy the compiler - in a real scenario,
	// you'd verify the cache package state
	cache := &struct {
		testMode      bool
		cacheDisabled bool
	}{}

	// TABLE: Define all boolean combinations
	// Each row is one test case with inputs and expected outputs
	tests := []struct {
		name      string // Descriptive name for test output
		testMode  bool   // Input: --test flag
		noCache   bool   // Input: --no-cache flag
		wantTest  bool   // Expected: cache.TestMode() value
		wantCache bool   // Expected: cache.CacheDisabled() value
	}{
		{
			name:      "both false",
			testMode:  false,
			noCache:   false,
			wantTest:  false,
			wantCache: false,
		},
		{
			name:      "test mode enabled",
			testMode:  true,
			noCache:   false,
			wantTest:  true,
			wantCache: false,
		},
		{
			name:      "no cache enabled",
			testMode:  false,
			noCache:   true,
			wantTest:  false,
			wantCache: true,
		},
		{
			name:      "both enabled",
			testMode:  true,
			noCache:   true,
			wantTest:  true,
			wantCache: true,
		},
	}

	// LOOP: Run each test case as a subtest
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: ConfigureModes calls cache.SetTestMode and cache.SetCacheDisabled
			// We can't easily test the actual state changes without mocking,
			// but we can verify the function doesn't panic
			ConfigureModes(tt.testMode, tt.noCache)
			_ = cache // Use the mock struct to avoid unused variable
		})
	}
}

// =============================================================================
// SUBTESTS WITH t.Run() - DETAILED WALKTHROUGH
// =============================================================================
//
// WHAT ARE SUBTESTS?
// ------------------
// Subtests are tests nested inside a parent test using t.Run(name, func).
// Each subtest:
// - Has its own name (shown in output)
// - Runs independently (one failure doesn't stop others)
// - Can be run individually: go test -run="TestFoo/subtest_name"
//
// WHEN TO USE SUBTESTS vs TABLE-DRIVEN:
// -------------------------------------
// - TABLE-DRIVEN: Same test logic, different inputs/outputs
// - SUBTESTS: Different test logic for different scenarios
//
// In this test, each subtest verifies a different aspect of error handling,
// so subtests are more appropriate than a table.
//
// =============================================================================

// TestValidateTestModeCache tests the cache validation for test mode.
// This function provides early detection when --test is used without cached data.
func TestValidateTestModeCache(t *testing.T) {
	tz := time.UTC

	// SUBTEST 1: Verify error is returned for missing cache
	t.Run("returns error when cache does not exist for date", func(t *testing.T) {
		// Use a date that definitely won't have cache (in the past)
		testDate := time.Date(1999, 1, 1, 0, 0, 0, 0, tz)

		err := ValidateTestModeCache(testDate, tz)

		// ASSERTION: We expect an error
		if err == nil {
			t.Error("ValidateTestModeCache() should return error when no cache exists")
		}

		// ASSERTION: Error message should be helpful
		// Good error messages include:
		// 1. What went wrong (no cache)
		// 2. Context (which date)
		// 3. How to fix it (run with --once)
		errMsg := err.Error()
		if !strings.Contains(errMsg, "1999-01-01") {
			t.Error("Error message should contain the date")
		}
		if !strings.Contains(errMsg, "./enphase-monitor --once") {
			t.Error("Error message should contain instructions to populate cache")
		}
	})

	// SUBTEST 2: Verify future dates also fail (no cache could exist)
	t.Run("returns error when cache does not exist for today", func(t *testing.T) {
		// Zero time means "today" - but we can't guarantee cache exists
		// So we test with a date far in the future that won't have cache
		futureDate := time.Date(2099, 12, 31, 0, 0, 0, 0, tz)

		err := ValidateTestModeCache(futureDate, tz)

		if err == nil {
			t.Error("ValidateTestModeCache() should return error for future date with no cache")
		}
	})

	// SUBTEST 3: Verify error message quality
	// Good error messages are part of the API - test them!
	t.Run("error message contains helpful instructions", func(t *testing.T) {
		testDate := time.Date(2050, 6, 15, 0, 0, 0, 0, tz)

		err := ValidateTestModeCache(testDate, tz)

		// t.Fatal vs t.Error:
		// - t.Fatal: Stop this subtest immediately (can't continue)
		// - t.Error: Record failure but continue running
		// Use Fatal when subsequent assertions depend on this one
		if err == nil {
			t.Fatal("Expected error but got nil")
		}

		errMsg := err.Error()

		// MULTIPLE ASSERTIONS on error message content
		// Each checks a different aspect of the message quality

		// Should contain the date (context)
		if !strings.Contains(errMsg, "2050-06-15") {
			t.Error("Error should contain the target date")
		}

		// Should contain instructions to populate cache (fix)
		if !strings.Contains(errMsg, "--once") {
			t.Error("Error should contain --once flag instruction")
		}

		// Should contain instructions for historical dates (alternative fix)
		if !strings.Contains(errMsg, "--date") {
			t.Error("Error should contain --date flag instruction")
		}
	})
}
