// Package cli - flags_test.go
//
// TEST SETUP
// ----------
// This test suite validates command-line flag parsing.
// Uses flag package to parse arguments and verify flag values.
//
// TEST PLAN
// ---------
// 1. Default Value Tests
//   - Test --config defaults to "config.yaml"
//   - Test boolean flags default to false
//   - Test empty string flags remain empty
//
// 2. Flag Parsing Tests
//   - Test --continuous flag sets Continuous to true
//   - Test --test flag sets Validation to true
//   - Test --date flag parses date string
//   - Test --config flag overrides default path
//   - Test --no-cache flag sets NoCache to true
//
// 3. Cache Command Tests
//   - Test --clear-cache sets ClearCache to true
//   - Test --clear-all-cache sets ClearAllCache to true
//
// TESTING APPROACH
// ----------------
// - Reset flag.CommandLine before each test (isolation)
// - Mock os.Args to simulate command-line arguments
// - Call ParseFlags() and verify returned struct
// - Restore os.Args after test (defer)
//
// WHY RESET FLAGS
// ---------------
// flag.CommandLine is package-level state that persists between tests.
// Resetting ensures:
// - Tests don't affect each other
// - Clean slate for each test
// - Tests can run in any order
// - Parallel execution is possible
//
// PATTERN USED
// ------------
// - Pattern 1: Table-Driven Tests
// - Pattern 9: State Reset (resetFlags before each test)
//
// See docs/TESTING.md for detailed pattern explanations.
package cli

import (
	"flag"
	"os"
	"testing"
)

// resetFlags is a test helper that resets the flag.CommandLine state.
// This ensures tests are isolated and don't affect each other.
func resetFlags(t *testing.T) {
	t.Helper()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
}

func TestParseFlags_Defaults(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd"}

	flags := ParseFlags()

	if flags.ConfigFile != "config.yaml" {
		t.Errorf("ParseFlags() ConfigFile = %v, want config.yaml", flags.ConfigFile)
	}
	if flags.Continuous {
		t.Errorf("ParseFlags() Continuous = %v, want false", flags.Continuous)
	}
	if flags.Validation {
		t.Errorf("ParseFlags() Validation = %v, want false", flags.Validation)
	}
}

func TestParseFlags_ConfigFile(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--config", "custom.yaml"}

	flags := ParseFlags()

	if flags.ConfigFile != "custom.yaml" {
		t.Errorf("ParseFlags() ConfigFile = %v, want custom.yaml", flags.ConfigFile)
	}
}

func TestParseFlags_Continuous(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--continuous"}

	flags := ParseFlags()

	if !flags.Continuous {
		t.Error("Continuous should be true")
	}
}

func TestParseFlags_Validation(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--test"}

	flags := ParseFlags()

	if !flags.Validation {
		t.Error("Validation should be true")
	}
}

func TestParseFlags_TestDate(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--date", "2026-01-15"}

	flags := ParseFlags()

	if flags.TestDate != "2026-01-15" {
		t.Errorf("TestDate = %v, want 2026-01-15", flags.TestDate)
	}
}

func TestParseFlags_MultipleFlags(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--config", "test.yaml", "--continuous", "--test", "--date", "2026-01-20"}

	flags := ParseFlags()

	if flags.ConfigFile != "test.yaml" {
		t.Errorf("ConfigFile = %v, want test.yaml", flags.ConfigFile)
	}
	if !flags.Continuous {
		t.Error("Continuous should be true")
	}
	if !flags.Validation {
		t.Error("Validation should be true")
	}
	if flags.TestDate != "2026-01-20" {
		t.Errorf("TestDate = %v, want 2026-01-20", flags.TestDate)
	}
}

func TestParseFlags_OAuthSetup(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--oauth-setup"}

	flags := ParseFlags()

	if !flags.OAuthSetup {
		t.Error("OAuthSetup should be true")
	}
}

func TestParseFlags_ClearCache(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--clear-cache"}

	flags := ParseFlags()

	if !flags.ClearCache {
		t.Error("ClearCache should be true")
	}
}

func TestParseFlags_ClearAllCache(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--clear-all-cache"}

	flags := ParseFlags()

	if !flags.ClearAllCache {
		t.Error("ClearAllCache should be true")
	}
}

func TestParseFlags_NoCache(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--no-cache"}

	flags := ParseFlags()

	if !flags.NoCache {
		t.Error("NoCache should be true")
	}
}

func TestParseFlags_CacheManagementFlags(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--clear-cache", "--no-cache"}

	flags := ParseFlags()

	if !flags.ClearCache {
		t.Error("ClearCache should be true")
	}
	if !flags.NoCache {
		t.Error("NoCache should be true")
	}
}

func TestParseFlags_AllCacheFlags(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--clear-all-cache"}

	flags := ParseFlags()

	if !flags.ClearAllCache {
		t.Error("ClearAllCache should be true")
	}
}

func TestParseFlags_ComplexCombination(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{
		"cmd",
		"--config", "production.yaml",
		"--continuous",
		"--date", "2026-01-20",
		"--test",
		"--no-cache",
	}

	flags := ParseFlags()

	if flags.ConfigFile != "production.yaml" {
		t.Errorf("ConfigFile = %v, want production.yaml", flags.ConfigFile)
	}
	if !flags.Continuous {
		t.Error("Continuous should be true")
	}
	if flags.TestDate != "2026-01-20" {
		t.Errorf("TestDate = %v, want 2026-01-20", flags.TestDate)
	}
	if !flags.Validation {
		t.Error("Validation should be true")
	}
	if !flags.NoCache {
		t.Error("NoCache should be true")
	}
}
