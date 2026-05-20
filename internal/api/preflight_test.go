// Package api - preflight_test.go
//
// Tests for budget-exhaustion cache-fallback behaviour across all 8 report
// types, and for the preflight budget-insufficient warning emitted by
// GetMetricsFromCloud.
//
// REPORT TYPES COVERED
// --------------------
//  1. Current date     (today, zero testDate, QueryTypeDay)
//  2. Specific date    (past day,             QueryTypeDay)
//  3. Month-to-date    (current month,        QueryTypeMonth)
//  4. Specific month   (past month,           QueryTypeMonth)
//  5. Year-to-date     (current year,         QueryTypeYear)
//  6. Specific year    (past year,            QueryTypeYear)
//  7. Current true-up  (active period,        QueryTypeTrueUp)
//  8. Past true-up     (completed period,     QueryTypeTrueUp)
//
// TESTING PATTERN
// ---------------
// Each test follows the same two-call pattern:
//  1. Prime call  — budget available → live API call, response saved to cache.
//  2. Exhaust     — RecordAPICall() until RemainingBudget() == 0.
//  3. Probe call  — budget zero → client must serve from cache.
//  4. Assert      — server received exactly 1 hit (the prime), not 2.
//
// For past periods (types 2, 4, 6, 8) the probe call never reaches the budget
// check at all: makeCachedAPIRequest short-circuits to cache the moment it sees
// isPast==true and a valid cache entry. This makes them a useful control group
// that confirms immutable-cache behaviour is budget-free.
package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
)

// uniqueSysID returns a system ID that is unique within this process run,
// preventing on-disk cache entries from a prior test execution from matching
// the URLs constructed by this test (httptest reuses ports across runs).
// Each call returns a different value even within the same test.
func uniqueSysID(tag string) string {
	return fmt.Sprintf("%s_%d", tag, time.Now().UnixNano())
}

// exhaustBudget records fake API calls until RemainingBudget() reaches zero.
func exhaustBudget() {
	for cache.RemainingBudget() > 0 {
		cache.RecordAPICall()
	}
}

// intervalProductionServer returns a mock HTTP server that responds only to
// requests whose path contains "production_meter". It counts how many times it
// was actually hit via the returned atomic counter.
func intervalProductionServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "production_meter") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"intervals":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// lifetimeProductionServer returns a mock HTTP server that responds only to
// requests whose path ends with "/energy_lifetime". It counts hits atomically.
func lifetimeProductionServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/energy_lifetime") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"system_id":12345,"start_date":"2020-01-01","production":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// captureStdout redirects os.Stdout for the duration of fn and returns everything
// written to it. Restores the original Stdout before returning.
func captureStdout(fn func()) string {
	r, w, _ := os.Pipe()
	orig := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = orig
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// =============================================================================
// 1. Current date — QueryTypeDay, today (zero testDate)
// =============================================================================

// TestBudgetExhausted_CurrentDate verifies that when the budget is fully
// consumed, a current-day query serves production data from cache and makes no
// additional live API call.
func TestBudgetExhausted_CurrentDate(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := intervalProductionServer(t)
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srv.URL, uniqueSysID("cur"), "key", "token", tz)

	ctx := context.Background()

	// Prime: live call populates cache.
	_, err := client.GetProductionForDate(ctx, time.Time{}, constants.QueryTypeDay)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget()

	// Probe: must use cache, not hit the server again.
	_, err = client.GetProductionForDate(ctx, time.Time{}, constants.QueryTypeDay)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (cache used), got %d", hits.Load())
	}
}

// =============================================================================
// 2. Specific date — QueryTypeDay, past day
// =============================================================================

// TestBudgetExhausted_SpecificDate verifies that a past-day result is always
// served from immutable cache — even when the budget was already zero before
// the probe call.
func TestBudgetExhausted_SpecificDate(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := intervalProductionServer(t)
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srv.URL, uniqueSysID("past"), "key", "token", tz)

	// A day that is reliably in the past.
	pastDay := time.Now().In(tz).AddDate(0, 0, -3)
	ctx := context.Background()

	// Prime: no cache → live call.
	_, err := client.GetProductionForDate(ctx, pastDay, constants.QueryTypeDay)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget()

	// Probe: past period + cache exists → short-circuits to immutable cache,
	// never reaches the budget check.
	_, err = client.GetProductionForDate(ctx, pastDay, constants.QueryTypeDay)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (immutable cache), got %d", hits.Load())
	}
}

// =============================================================================
// 3. Month-to-date — QueryTypeMonth, current month
// =============================================================================

// TestBudgetExhausted_MonthToDate verifies that a current-month query falls
// back to the lifetime-endpoint cache when the budget is exhausted.
func TestBudgetExhausted_MonthToDate(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srv.URL, uniqueSysID("mtd"), "key", "token", tz)

	// Any date within the current month works; use today.
	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, time.Now().In(tz), constants.QueryTypeMonth)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget()

	_, err = client.GetProductionForDate(ctx, time.Now().In(tz), constants.QueryTypeMonth)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (cache used), got %d", hits.Load())
	}
}

// =============================================================================
// 4. Specific month — QueryTypeMonth, past month
// =============================================================================

// TestBudgetExhausted_SpecificMonth verifies that a completed past-month result
// is served from immutable cache even with zero budget.
func TestBudgetExhausted_SpecificMonth(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srv.URL, uniqueSysID("smon"), "key", "token", tz)

	pastMonth := time.Now().In(tz).AddDate(0, -2, 0)
	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, pastMonth, constants.QueryTypeMonth)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget()

	_, err = client.GetProductionForDate(ctx, pastMonth, constants.QueryTypeMonth)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (immutable cache), got %d", hits.Load())
	}
}

// =============================================================================
// 5. Year-to-date — QueryTypeYear, current year
// =============================================================================

// TestBudgetExhausted_YearToDate verifies that a current-year query falls back
// to the lifetime-endpoint cache when the budget is exhausted.
func TestBudgetExhausted_YearToDate(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srv.URL, uniqueSysID("ytd"), "key", "token", tz)

	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, time.Now().In(tz), constants.QueryTypeYear)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget()

	_, err = client.GetProductionForDate(ctx, time.Now().In(tz), constants.QueryTypeYear)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (cache used), got %d", hits.Load())
	}
}

// =============================================================================
// 6. Specific year — QueryTypeYear, past year
// =============================================================================

// TestBudgetExhausted_SpecificYear verifies that a completed past-year result
// is served from immutable cache even with zero budget.
func TestBudgetExhausted_SpecificYear(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srv.URL, uniqueSysID("syr"), "key", "token", tz)

	pastYear := time.Now().In(tz).AddDate(-2, 0, 0)
	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, pastYear, constants.QueryTypeYear)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget()

	_, err = client.GetProductionForDate(ctx, pastYear, constants.QueryTypeYear)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (immutable cache), got %d", hits.Load())
	}
}

// =============================================================================
// 7. Current true-up — QueryTypeTrueUp, active period
// =============================================================================

// TestBudgetExhausted_CurrentTrueUp verifies that an active true-up query
// falls back to the lifetime-endpoint cache when the budget is exhausted.
// IsPastPeriod always returns false for QueryTypeTrueUp; cacheMaxAge also
// treats it as current when trueUpStart + 1 year is still in the future.
func TestBudgetExhausted_CurrentTrueUp(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srv.URL, uniqueSysID("ctu"), "key", "token", tz)

	// A true-up that started 6 months ago is still active (end = 6 months from now).
	activeTrueUpStart := time.Now().In(tz).AddDate(0, -6, 0)
	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, activeTrueUpStart, constants.QueryTypeTrueUp)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget()

	_, err = client.GetProductionForDate(ctx, activeTrueUpStart, constants.QueryTypeTrueUp)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (cache used), got %d", hits.Load())
	}
}

// =============================================================================
// 8. Past true-up — QueryTypeTrueUp, completed period
// =============================================================================

// TestBudgetExhausted_PastTrueUp verifies that a completed true-up year is
// served from immutable cache even with zero budget. cacheMaxAge detects a
// past true-up when trueUpStart + 1 year is before now and sets maxAge = 0
// (never-expires sentinel).
func TestBudgetExhausted_PastTrueUp(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srv.URL, uniqueSysID("ptu"), "key", "token", tz)

	// A true-up that started 2 years ago ended 1 year ago → it is in the past.
	pastTrueUpStart := time.Now().In(tz).AddDate(-2, 0, 0)
	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, pastTrueUpStart, constants.QueryTypeTrueUp)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget()

	_, err = client.GetProductionForDate(ctx, pastTrueUpStart, constants.QueryTypeTrueUp)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (immutable cache), got %d", hits.Load())
	}
}

// =============================================================================
// 9. No-cache + exhausted budget → RateLimitError
// =============================================================================

// TestBudgetExhausted_NoCache_ReturnsRateLimitError verifies that a current-
// period query returns a RateLimitError (not a panic or misleading zero) when
// the budget is zero and no cache entry exists for the endpoint.
func TestBudgetExhausted_NoCache_ReturnsRateLimitError(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := intervalProductionServer(t)
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srv.URL, uniqueSysID("nc"), "key", "token", tz)

	// Exhaust budget BEFORE any priming call so the cache is empty.
	exhaustBudget()

	ctx := context.Background()
	_, err := client.GetProductionForDate(ctx, time.Time{}, constants.QueryTypeDay)

	if err == nil {
		t.Fatal("expected error when budget exhausted and no cache, got nil")
	}
	if !constants.IsRateLimitError(err) {
		t.Errorf("expected RateLimitError, got: %v", err)
	}
	// Server must not have been hit — budget was zero from the start.
	if hits.Load() != 0 {
		t.Errorf("expected 0 server hits (budget exhausted), got %d", hits.Load())
	}
}

// =============================================================================
// 10. Preflight warning — emitted for current periods, suppressed for past
// =============================================================================

// TestPreflightWarning_CurrentPeriod verifies that GetMetricsFromCloud prints
// a budget-insufficient warning to stdout when the remaining budget is less
// than the query cost for a current period.
func TestPreflightWarning_CurrentPeriod(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()
	cache.SetDebugMode(true)
	defer cache.SetDebugMode(false)

	// Full mock that serves all five day-query endpoints so GetMetricsFromCloud
	// can complete its prime call successfully.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.Contains(p, "energy_import_telemetry"):
			fmt.Fprintf(w, `{"intervals":[[]]}`)
		case strings.Contains(p, "energy_export_telemetry"):
			fmt.Fprintf(w, `{"intervals":[[]]}`)
		case strings.Contains(p, "production_meter"):
			fmt.Fprintf(w, `{"intervals":[]}`)
		case strings.Contains(p, "telemetry/battery"):
			fmt.Fprintf(w, `{"intervals":[]}`)
		case strings.Contains(p, "consumption_meter"):
			fmt.Fprintf(w, `{"intervals":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srv.URL, uniqueSysID("warn"), "key", "token", tz)
	ctx := context.Background()

	// Prime: populates cache for all five endpoints.
	_, _, err := client.GetMetricsFromCloud(ctx, time.Time{}, constants.QueryTypeDay)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}

	// Leave exactly 1 call in the budget (< 5 needed for QueryTypeDay+battery).
	for cache.RemainingBudget() > 1 {
		cache.RecordAPICall()
	}

	// Probe: preflight should warn because remaining (1) < needed (5).
	output := captureStdout(func() {
		_, _, _ = client.GetMetricsFromCloud(ctx, time.Time{}, constants.QueryTypeDay)
	})

	if !strings.Contains(output, "Insufficient API budget") {
		t.Errorf("expected preflight warning in stdout, got: %q", output)
	}
}

// TestPreflightWarning_PastPeriod verifies that no preflight warning is emitted
// for past periods — their data is always served from immutable cache and never
// consumes budget.
func TestPreflightWarning_PastPeriod(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	// Mock that serves the interval endpoint for the past day prime call.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.Contains(p, "energy_import_telemetry"):
			fmt.Fprintf(w, `{"intervals":[[]]}`)
		case strings.Contains(p, "energy_export_telemetry"):
			fmt.Fprintf(w, `{"intervals":[[]]}`)
		case strings.Contains(p, "production_meter"):
			fmt.Fprintf(w, `{"intervals":[]}`)
		case strings.Contains(p, "telemetry/battery"):
			fmt.Fprintf(w, `{"intervals":[]}`)
		case strings.Contains(p, "consumption_meter"):
			fmt.Fprintf(w, `{"intervals":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srv.URL, uniqueSysID("pwrn"), "key", "token", tz)
	pastDay := time.Now().In(tz).AddDate(0, 0, -3)
	ctx := context.Background()

	// Prime: populates cache for the past day.
	_, _, err := client.GetMetricsFromCloud(ctx, pastDay, constants.QueryTypeDay)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}

	// Exhaust budget completely.
	exhaustBudget()

	// Probe: past period → no preflight warning expected.
	output := captureStdout(func() {
		_, _, _ = client.GetMetricsFromCloud(ctx, pastDay, constants.QueryTypeDay)
	})

	if strings.Contains(output, "Insufficient API budget") {
		t.Errorf("unexpected preflight warning for past period: %q", output)
	}
}
