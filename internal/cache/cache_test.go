// Package cache - cache_test.go
//
// TEST SETUP
// ----------
// This test suite validates thread-safe cache state management.
// Tests use goroutines to simulate concurrent access patterns.
//
// TEST PLAN
// ---------
// 1. Thread Safety Tests
//    - Test concurrent TestMode access (10 goroutines × 100 iterations)
//    - Test concurrent CacheDisabled access
//    - Test concurrent RateLimitWarningShown access
//    - Run with -race flag to detect data races
//
// 2. State Management Tests
//    - Test SetTestMode/TestMode
//    - Test SetCacheDisabled/CacheDisabled
//    - Test SetRateLimitWarningShown/RateLimitWarningShown
//    - Test ResetState clears all flags
//
// TESTING APPROACH
// ----------------
// - Use sync.WaitGroup to coordinate goroutines
// - Each goroutine performs many rapid set/get operations
// - ResetState() called before each test for isolation
// - Run with `go test -race` to verify no data races
//
// WHY THREAD SAFETY MATTERS
// -------------------------
// The cache state is accessed from multiple parts of the codebase:
// - Test mode flag checked before every API call
// - Cache disabled flag checked in cache lookup
// - Rate limit warning ensures message printed only once
//
// Mutex protection (sync.Mutex) ensures safe concurrent access.
//
// TEST ORGANIZATION
// -----------------
// This package has 3 test files (1:many pattern):
// - cache_test.go (this file): Thread safety tests (161 lines)
// - cache_functions_test.go: Core functionality tests (516 lines)
// - cli_test.go: CLI utilities tests (119 lines)
//
// PATTERN USED
// ------------
// - Pattern 7: Thread Safety Testing (goroutines, sync.WaitGroup)
// - Pattern 10: State Reset (ResetState before each test)
//
// See TESTING.md for detailed pattern explanations.
package cache

import (
	"sync"
	"testing"
)

// TestCacheState_ThreadSafety tests concurrent access to cache state.
// This verifies that the mutex-protected state can be accessed from multiple goroutines
// without race conditions.
func TestCacheState_ThreadSafety(t *testing.T) {
	// Reset to known state
	ResetState()

	const numGoroutines = 10
	const numIterations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // 3 types of operations

	// Concurrent TestMode access
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				SetTestMode(true)
				_ = TestMode()
				SetTestMode(false)
			}
		}()
	}

	// Concurrent CacheDisabled access
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				SetCacheDisabled(true)
				_ = CacheDisabled()
				SetCacheDisabled(false)
			}
		}()
	}

	// Concurrent RateLimitWarningShown access
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				SetRateLimitWarningShown(true)
				_ = RateLimitWarningShown()
				SetRateLimitWarningShown(false)
			}
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// If we get here without deadlock or race detector errors, the test passes
}

// TestResetState verifies that ResetState properly resets all flags.
func TestResetState(t *testing.T) {
	// Set all flags to true
	SetTestMode(true)
	SetCacheDisabled(true)
	SetRateLimitWarningShown(true)

	// Verify they are set
	if !TestMode() {
		t.Error("TestMode should be true before reset")
	}
	if !CacheDisabled() {
		t.Error("CacheDisabled should be true before reset")
	}
	if !RateLimitWarningShown() {
		t.Error("RateLimitWarningShown should be true before reset")
	}

	// Reset state
	ResetState()

	// Verify all flags are false
	if TestMode() {
		t.Error("TestMode should be false after reset")
	}
	if CacheDisabled() {
		t.Error("CacheDisabled should be false after reset")
	}
	if RateLimitWarningShown() {
		t.Error("RateLimitWarningShown should be false after reset")
	}
}

// TestTestModeGetterSetter verifies TestMode getter and setter.
func TestTestModeGetterSetter(t *testing.T) {
	ResetState()

	// Initial state should be false
	if TestMode() {
		t.Error("TestMode should be false initially")
	}

	// Set to true
	SetTestMode(true)
	if !TestMode() {
		t.Error("TestMode should be true after SetTestMode(true)")
	}

	// Set back to false
	SetTestMode(false)
	if TestMode() {
		t.Error("TestMode should be false after SetTestMode(false)")
	}
}

// TestCacheDisabledGetterSetter verifies CacheDisabled getter and setter.
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

// TestRateLimitWarningShownGetterSetter verifies RateLimitWarningShown getter and setter.
func TestRateLimitWarningShownGetterSetter(t *testing.T) {
	ResetState()

	// Initial state should be false
	if RateLimitWarningShown() {
		t.Error("RateLimitWarningShown should be false initially")
	}

	// Set to true
	SetRateLimitWarningShown(true)
	if !RateLimitWarningShown() {
		t.Error("RateLimitWarningShown should be true after SetRateLimitWarningShown(true)")
	}

	// Set back to false
	SetRateLimitWarningShown(false)
	if RateLimitWarningShown() {
		t.Error("RateLimitWarningShown should be false after SetRateLimitWarningShown(false)")
	}
}
