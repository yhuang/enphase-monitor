// Package cache - api_budget_test.go
//
// Tests for the sliding-window API Budget counter: RecordAPICall, RemainingBudget,
// and the recentAPICallStampsLocked pruning logic.
//
// Each test calls ResetState() first (Pattern 9) so the api_calls file is
// always clean at the start — prior test runs cannot leave stale timestamps.
package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRemainingBudget_FreshState verifies that a clean slate reports the full
// budget of MaxRequestsPerWindow.
func TestRemainingBudget_FreshState(t *testing.T) {
	useTempCacheDir(t)
	ResetState()
	defer ResetState()

	got := RemainingBudget()
	if got != MaxRequestsPerWindow {
		t.Errorf("RemainingBudget() = %d, want %d (full budget on clean state)", got, MaxRequestsPerWindow)
	}
}

// TestRemainingBudget_AfterCalls verifies that each RecordAPICall reduces the
// remaining budget by exactly one.
func TestRemainingBudget_AfterCalls(t *testing.T) {
	useTempCacheDir(t)
	ResetState()
	defer ResetState()

	for i := 1; i <= MaxRequestsPerWindow; i++ {
		RecordAPICall()
		want := MaxRequestsPerWindow - i
		got := RemainingBudget()
		if got != want {
			t.Errorf("after %d RecordAPICall(): RemainingBudget() = %d, want %d", i, got, want)
		}
	}
}

// TestRemainingBudget_Exhausted verifies that the budget cannot go negative —
// it floors at zero even when more calls than MaxRequestsPerWindow are recorded.
func TestRemainingBudget_Exhausted(t *testing.T) {
	useTempCacheDir(t)
	ResetState()
	defer ResetState()

	for i := 0; i < MaxRequestsPerWindow+3; i++ {
		RecordAPICall()
	}

	got := RemainingBudget()
	if got != 0 {
		t.Errorf("RemainingBudget() = %d after exhausting budget, want 0", got)
	}
}

// TestRemainingBudget_OldEntriesPruned verifies that timestamps older than
// MinRequestInterval are not counted. We write a fake api_calls file with two
// old entries and two fresh ones; only the two fresh ones should consume budget.
func TestRemainingBudget_OldEntriesPruned(t *testing.T) {
	dir := useTempCacheDir(t)
	ResetState()
	defer ResetState()

	old := time.Now().Add(-(MinRequestInterval + 5*time.Second))
	fresh1 := time.Now().Add(-10 * time.Second)
	fresh2 := time.Now().Add(-5 * time.Second)

	lines := []string{
		old.Format(time.RFC3339Nano),
		old.Add(-time.Minute).Format(time.RFC3339Nano),
		fresh1.Format(time.RFC3339Nano),
		fresh2.Format(time.RFC3339Nano),
	}
	content := strings.Join(lines, "\n") + "\n"
	path := filepath.Join(dir, apiCallsFilename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write api_calls: %v", err)
	}

	// Two old entries are outside the window; two fresh entries consume budget.
	want := MaxRequestsPerWindow - 2
	got := RemainingBudget()
	if got != want {
		t.Errorf("RemainingBudget() = %d, want %d (old entries should be pruned)", got, want)
	}
}

// TestRecordAPICall_PrunesOldEntries verifies that RecordAPICall writes only
// the in-window timestamps (old entries are dropped on the next write).
func TestRecordAPICall_PrunesOldEntries(t *testing.T) {
	dir := useTempCacheDir(t)
	ResetState()
	defer ResetState()

	// Seed file with one entry that is already expired.
	old := time.Now().Add(-(MinRequestInterval + time.Second))
	path := filepath.Join(dir, apiCallsFilename)
	if err := os.WriteFile(path, []byte(old.Format(time.RFC3339Nano)+"\n"), 0644); err != nil {
		t.Fatalf("failed to seed api_calls: %v", err)
	}

	RecordAPICall() // should add 1 fresh entry and drop the old one

	// File should now have exactly one entry (the fresh call).
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read api_calls: %v", err)
	}
	lines := []string{}
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) != 1 {
		t.Errorf("api_calls has %d line(s) after RecordAPICall(), want 1 (old entry pruned)", len(lines))
	}
}

// TestClearAPICalls verifies that ClearAPICalls removes all recorded calls and
// restores the full budget.
func TestClearAPICalls(t *testing.T) {
	useTempCacheDir(t)
	ResetState()
	defer ResetState()

	for i := 0; i < 5; i++ {
		RecordAPICall()
	}
	if RemainingBudget() != MaxRequestsPerWindow-5 {
		t.Fatalf("precondition: expected %d remaining after 5 calls", MaxRequestsPerWindow-5)
	}

	ClearAPICalls()

	got := RemainingBudget()
	if got != MaxRequestsPerWindow {
		t.Errorf("RemainingBudget() = %d after ClearAPICalls(), want %d", got, MaxRequestsPerWindow)
	}
}

// TestLastAPICallTime_NoCallsYet verifies that LastAPICallTime returns (zero, false)
// on a clean slate.
func TestLastAPICallTime_NoCallsYet(t *testing.T) {
	useTempCacheDir(t)
	ResetState()
	defer ResetState()

	ts, ok := LastAPICallTime()
	if ok {
		t.Errorf("LastAPICallTime() ok = true, want false when no calls recorded")
	}
	if !ts.IsZero() {
		t.Errorf("LastAPICallTime() ts = %v, want zero", ts)
	}
}

// TestLastAPICallTime_AfterCalls verifies that LastAPICallTime returns (latest, true)
// after calls are recorded.
func TestLastAPICallTime_AfterCalls(t *testing.T) {
	useTempCacheDir(t)
	ResetState()
	defer ResetState()

	before := time.Now()
	RecordAPICall()
	after := time.Now()

	ts, ok := LastAPICallTime()
	if !ok {
		t.Fatal("LastAPICallTime() ok = false, want true after a call")
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("LastAPICallTime() = %v, want between %v and %v", ts, before, after)
	}
}
