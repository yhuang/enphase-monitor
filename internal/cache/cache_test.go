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
