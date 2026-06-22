// Package cache - cache_test.go
//
// TEST SETUP
// ----------
// This test suite validates cache state management functions.
// Tests verify getter/setter behavior and state reset functionality.
//
// TEST PLAN
// ---------
// 1. State Management Tests
//   - Test SetValidationMode/ValidationMode
//   - Test SetCacheDisabled/CacheDisabled
//   - Test ResetState clears all flags
//
// TESTING APPROACH
// ----------------
// - ResetState() called before each test for isolation
// - Verify initial state is false for all flags
// - Verify setters correctly update state
//
// TEST ORGANIZATION
// -----------------
// This package has 3 test files (1:many pattern):
// - cache_test.go (this file): State management tests
// - cache_functions_test.go: Core functionality tests
// - cli_test.go: CLI utilities tests
//
// PATTERN USED
// ------------
// - Pattern 9: State Reset (ResetState before each test)
//
// See TESTING.md for detailed pattern explanations.
package cache

import (
	"testing"
)

// =============================================================================
// PATTERN 9: STATE RESET
// =============================================================================
//
// WHAT IS THE STATE RESET PATTERN?
// --------------------------------
// When testing code that uses package-level variables (global state), tests
// can interfere with each other. If Test A sets a flag to true, Test B might
// fail because it expected the flag to be false.
//
// The State Reset pattern solves this by:
// 1. Providing a ResetState() function that clears all state
// 2. Calling ResetState() at the START of each test (not the end)
//
// WHY RESET AT THE START, NOT THE END?
// -------------------------------------
// If you reset at the end and a test crashes/panics before cleanup,
// the next test gets dirty state. Resetting at the START guarantees
// each test begins with clean state regardless of what happened before.
//
// WHEN TO USE THIS PATTERN
// ------------------------
// - Testing code with package-level variables
// - Testing singletons or cached state
// - Any test that modifies shared mutable state
//
// =============================================================================

// TestResetState verifies that ResetState properly resets all flags.
//
// WALKTHROUGH: Testing the Reset Function Itself
// -----------------------------------------------
// This test verifies our cleanup mechanism actually works. It's the
// foundation for all other tests - if ResetState() is broken, no
// other test can be trusted.
//
// The testing strategy is:
// 1. Set ALL flags to non-default values (true)
// 2. Verify they were actually set (sanity check)
// 3. Call ResetState()
// 4. Verify ALL flags are back to defaults (false)
func TestResetState(t *testing.T) {
	// STEP 1: Dirty the state intentionally
	// We set all flags to true (the non-default value)
	SetValidationMode(true)
	SetCacheDisabled(true)

	// STEP 2: Sanity check - verify our setters worked
	// This catches bugs where setters silently fail
	if !ValidationMode() {
		t.Error("ValidationMode should be true before reset")
	}
	if !CacheDisabled() {
		t.Error("CacheDisabled should be true before reset")
	}

	// STEP 3: Call the function under test
	ResetState()

	// STEP 4: Verify all flags are back to false (default)
	// Each assertion checks one piece of state
	if ValidationMode() {
		t.Error("ValidationMode should be false after reset")
	}
	if CacheDisabled() {
		t.Error("CacheDisabled should be false after reset")
	}
}

// =============================================================================
// GETTER/SETTER TESTS WITH STATE RESET
// =============================================================================
//
// WALKTHROUGH: Testing Getter/Setter Pairs
// -----------------------------------------
// These tests validate that each getter/setter pair works correctly.
// They all follow the same structure:
//
// 1. ResetState() - ensure clean starting point (Pattern 9)
// 2. Verify initial value is false (the default)
// 3. Set to true, verify getter returns true
// 4. Set to false, verify getter returns false
//
// WHY TEST BOTH DIRECTIONS?
// -------------------------
// A buggy setter might only work one way. For example:
//   func SetValidationMode(enabled bool) { validationMode = true }  // Bug: ignores parameter
// This would pass "set to true" but fail "set to false".
//
// =============================================================================

// TestValidationModeGetterSetter verifies ValidationMode getter and setter.
func TestValidationModeGetterSetter(t *testing.T) {
	// PATTERN 9: Always reset at the start of each test
	// This ensures previous tests don't affect this one
	ResetState()

	// STEP 1: Verify initial state (after reset)
	// The default for all boolean flags should be false
	if ValidationMode() {
		t.Error("ValidationMode should be false initially")
	}

	// STEP 2: Test setting to true
	SetValidationMode(true)
	if !ValidationMode() {
		t.Error("ValidationMode should be true after SetValidationMode(true)")
	}

	// STEP 3: Test setting back to false
	// This catches bugs where the setter ignores the parameter
	SetValidationMode(false)
	if ValidationMode() {
		t.Error("ValidationMode should be false after SetValidationMode(false)")
	}
}

// TestCacheDisabledGetterSetter verifies CacheDisabled getter and setter.
//
// Note: This test is structurally identical to TestValidationModeGetterSetter.
// In a larger codebase, you might use a table-driven test to reduce
// duplication. Here, explicit tests are clearer for a small number of flags.
func TestCacheDisabledGetterSetter(t *testing.T) {
	ResetState()

	// Initial state should be false
	if CacheDisabled() {
		t.Error("CacheDisabled should be false initially")
	}

	// Set to true
	SetCacheDisabled(true)
	if !CacheDisabled() {
		t.Error("CacheDisabled should be true after SetCacheDisabled(true)")
	}

	// Set back to false
	SetCacheDisabled(false)
	if CacheDisabled() {
		t.Error("CacheDisabled should be false after SetCacheDisabled(false)")
	}
}
