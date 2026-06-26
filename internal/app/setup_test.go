package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/credentials"
	"enphase-monitor/internal/types"
)

// TestFormatDateForQueryMode tests date formatting for each query mode.
func TestFormatDateForQueryMode(t *testing.T) {
	date := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		queryMode constants.QueryMode
		want      string
	}{
		{constants.QueryModeDay, "2026-03-15"},
		{constants.QueryModeMonth, "2026-03"},
		{constants.QueryModeYear, "2026"},
	}

	for _, tt := range tests {
		t.Run(tt.queryMode.String(), func(t *testing.T) {
			got := FormatDateForQueryMode(date, tt.queryMode)
			if got != tt.want {
				t.Errorf("FormatDateForQueryMode(%v) = %q, want %q", tt.queryMode, got, tt.want)
			}
		})
	}
}

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
		Credentials: []*types.APIConfig{{
			Name:         "key1",
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		}},
		RefreshIntervalSeconds: 3600,
	}

	tz := time.UTC

	disp := SetupDisplay(cfg, tz)

	if disp == nil {
		t.Error("SetupDisplay() returned nil")
	}
}

func TestSetupDisplay_WithCustomColors(t *testing.T) {
	// VARIANT: Custom colors in configuration
	// This tests the color override path vs the default color path
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "Test System", ID: "12345"},
		},
		Credentials: []*types.APIConfig{{
			Name:         "key1",
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		}},
		RefreshIntervalSeconds: 3600,
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
	if !parsed.Date.IsZero() {
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
	if parsed.Date.IsZero() {
		t.Error("ParseTestDate() returned zero time for valid date")
	}

	// Verify the EXACT parsed value, not just that it's non-zero
	expected := time.Date(2026, 1, 15, 0, 0, 0, 0, tz)
	if !parsed.Date.Equal(expected) {
		t.Errorf("ParseTestDate() = %v, want %v", parsed.Date, expected)
	}
	// Verify query mode is day for YYYY-MM-DD format
	if parsed.QueryMode != constants.QueryModeDay {
		t.Errorf("ParseTestDate() QueryMode = %v, want QueryModeDay", parsed.QueryMode)
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

func TestGetSystems(t *testing.T) {
	cfg := &config.Config{
		Systems: []types.SystemConfig{
			{Name: "System 1", ID: "12345"},
			{Name: "System 2", ID: "67890"},
		},
	}

	systems := GetSystems(cfg)

	if len(systems) != 2 {
		t.Errorf("GetSystems() returned %d systems, want 2", len(systems))
	}
	// Verify the returned slice is a copy (mutating it must not affect cfg).
	systems[0].Name = "mutated"
	if cfg.Systems[0].Name != "System 1" {
		t.Error("GetSystems() returned slice aliases the config; want a copy")
	}
}

func TestGetSystems_EmptySystems(t *testing.T) {
	cfg := &config.Config{Systems: []types.SystemConfig{}}

	if systems := GetSystems(cfg); len(systems) != 0 {
		t.Errorf("GetSystems() returned %d systems, want 0", len(systems))
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
		Credentials: []*types.APIConfig{{
			Name:         "key1",
			Key:          "test-key",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		}},
	}

	tz := time.UTC
	disp := SetupDisplay(cfg, tz)

	// RunOnce with cancelled context should return error (no os.Exit)
	err := RunOnce(ctx, RunConfig{Agg: agg, Pool: credentials.NewPool(cfg.Credentials), Disp: disp, Cfg: cfg, QueryMode: constants.QueryModeDay, ReportTZ: tz})
	if err == nil {
		t.Fatal("RunOnce() with cancelled context: error = nil, want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("RunOnce() error = %v, want context.Canceled", err)
	}
}

// COMBINING PATTERNS:
// -------------------
// This test combines Pattern 1 (Table-Driven) with Pattern 3 (Subtests).
// The table defines all test cases, and t.Run() creates a subtest for each.
//
// TestConfigureModes tests that ConfigureModes correctly propagates flags to cache state.
func TestConfigureModes(t *testing.T) {
	tests := []struct {
		name      string
		noCache   bool
		wantCache bool
	}{
		{name: "defaults", noCache: false, wantCache: false},
		{name: "no cache enabled", noCache: true, wantCache: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(cache.ResetState)
			ConfigureModes(tt.noCache, false)
			if got := cache.CacheDisabled(); got != tt.wantCache {
				t.Errorf("cache.CacheDisabled() = %v, want %v", got, tt.wantCache)
			}
		})
	}
}

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
