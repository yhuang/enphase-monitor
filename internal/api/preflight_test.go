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
	"enphase-monitor/internal/credentials"
	"enphase-monitor/internal/types"
)

// testPool returns a single-credential pool for budget tests.
func testPool(t *testing.T) (*credentials.Pool, string) {
	t.Helper()
	p := credentials.NewPool([]*types.APIConfig{{Name: "test", Key: "k", ClientID: "c", ClientSecret: "s"}})
	dir := t.TempDir()
	t.Setenv("ENPHASE_CACHE_DIR", dir)
	return p, "test"
}

// exhaustBudget records fake API calls until the credential's minute budget reaches zero.
func exhaustBudget(t *testing.T, p *credentials.Pool, name string) {
	t.Helper()
	for p.RemainingMinuteBudget(name) > 0 {
		p.RecordAPICall(name)
	}
}
// preventing on-disk cache entries from a prior test execution from matching
// the URLs constructed by this test (httptest reuses ports across runs).
// Each call returns a different value even within the same test.
func uniqueSysID(tag string) string {
	return fmt.Sprintf("%s_%d", tag, time.Now().UnixNano())
}

// budgetClient returns a client wired to a fresh per-credential budget pool.
func budgetClient(t *testing.T, srvURL, sysID string) (*EnlightenCloudClient, *credentials.Pool, string) {
	t.Helper()
	p, name := testPool(t)
	tz := mustLoadLocation(t, "US/Pacific")
	client := NewEnlightenCloudClientWithBaseURL(srvURL, sysID, "key", "token", tz).WithBudget(p, name)
	return client, p, name
}

// uniqueSysID returns a system ID that is unique within this process run,
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

// captureStderr redirects os.Stderr for the duration of fn and returns everything
// written to it. Restores the original Stderr before returning. Operational warnings
// (including the budget preflight warning) are emitted to stderr, not stdout.
func captureStderr(fn func()) string {
	r, w, _ := os.Pipe()
	orig := os.Stderr
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = orig
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// TestBudgetExhausted_CurrentDate verifies that when the budget is fully
// consumed, a current-day query serves production data from cache and makes no
// additional live API call.
func TestBudgetExhausted_CurrentDate(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := intervalProductionServer(t)
	client, p, credName := budgetClient(t, srv.URL, uniqueSysID("cur"))

	ctx := context.Background()

	// Prime: live call populates cache.
	_, err := client.GetProductionForDate(ctx, time.Time{}, constants.QueryModeDay)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget(t, p, credName)

	// Probe: must use cache, not hit the server again.
	_, err = client.GetProductionForDate(ctx, time.Time{}, constants.QueryModeDay)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (cache used), got %d", hits.Load())
	}
}

// TestBudgetExhausted_SpecificDate verifies that a past-day result is always
// served from immutable cache — even when the budget was already zero before
// the probe call.
func TestBudgetExhausted_SpecificDate(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := intervalProductionServer(t)
	client, p, credName := budgetClient(t, srv.URL, uniqueSysID("past"))
	tz := mustLoadLocation(t, "US/Pacific")

	// A day that is reliably in the past.
	pastDay := time.Now().In(tz).AddDate(0, 0, -3)
	ctx := context.Background()

	// Prime: no cache → live call.
	_, err := client.GetProductionForDate(ctx, pastDay, constants.QueryModeDay)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget(t, p, credName)

	// Probe: Past Period + cache exists → short-circuits to immutable cache,
	// never reaches the budget check.
	_, err = client.GetProductionForDate(ctx, pastDay, constants.QueryModeDay)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (immutable cache), got %d", hits.Load())
	}
}

// TestBudgetExhausted_MonthToDate verifies that a current-month query falls
// back to the lifetime-endpoint cache when the budget is exhausted.
func TestBudgetExhausted_MonthToDate(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	client, p, credName := budgetClient(t, srv.URL, uniqueSysID("mtd"))
	tz := mustLoadLocation(t, "US/Pacific")

	// Any date within the current month works; use today.
	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, time.Now().In(tz), constants.QueryModeMonth)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget(t, p, credName)

	_, err = client.GetProductionForDate(ctx, time.Now().In(tz), constants.QueryModeMonth)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (cache used), got %d", hits.Load())
	}
}

// TestBudgetExhausted_SpecificMonth verifies that a completed past-month result
// is served from immutable cache even with zero budget.
func TestBudgetExhausted_SpecificMonth(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	client, p, credName := budgetClient(t, srv.URL, uniqueSysID("smon"))
	tz := mustLoadLocation(t, "US/Pacific")

	pastMonth := time.Now().In(tz).AddDate(0, -2, 0)
	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, pastMonth, constants.QueryModeMonth)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget(t, p, credName)

	_, err = client.GetProductionForDate(ctx, pastMonth, constants.QueryModeMonth)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (immutable cache), got %d", hits.Load())
	}
}

// TestBudgetExhausted_YearToDate verifies that a current-year query falls back
// to the lifetime-endpoint cache when the budget is exhausted.
func TestBudgetExhausted_YearToDate(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	client, p, credName := budgetClient(t, srv.URL, uniqueSysID("ytd"))
	tz := mustLoadLocation(t, "US/Pacific")

	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, time.Now().In(tz), constants.QueryModeYear)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget(t, p, credName)

	_, err = client.GetProductionForDate(ctx, time.Now().In(tz), constants.QueryModeYear)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (cache used), got %d", hits.Load())
	}
}

// TestBudgetExhausted_SpecificYear verifies that a completed past-year result
// is served from immutable cache even with zero budget.
func TestBudgetExhausted_SpecificYear(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	client, p, credName := budgetClient(t, srv.URL, uniqueSysID("syr"))
	tz := mustLoadLocation(t, "US/Pacific")

	pastYear := time.Now().In(tz).AddDate(-2, 0, 0)
	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, pastYear, constants.QueryModeYear)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget(t, p, credName)

	_, err = client.GetProductionForDate(ctx, pastYear, constants.QueryModeYear)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (immutable cache), got %d", hits.Load())
	}
}

// TestBudgetExhausted_CurrentTrueUp verifies that a Current Period True-Up Mode query
// falls back to the lifetime-endpoint cache when the budget is exhausted.
// IsPastPeriod always returns false for QueryModeTrueUp; cacheMaxAge also
// treats it as current when trueUpStart + 1 year is still in the future.
func TestBudgetExhausted_CurrentTrueUp(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	client, p, credName := budgetClient(t, srv.URL, uniqueSysID("ctu"))
	tz := mustLoadLocation(t, "US/Pacific")

	// A true-up that started 6 months ago is still active (end = 6 months from now).
	activeTrueUpStart := time.Now().In(tz).AddDate(0, -6, 0)
	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, activeTrueUpStart, constants.QueryModeTrueUp)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget(t, p, credName)

	_, err = client.GetProductionForDate(ctx, activeTrueUpStart, constants.QueryModeTrueUp)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (cache used), got %d", hits.Load())
	}
}

// TestBudgetExhausted_PastTrueUp verifies that a Past True-Up Period is
// served from immutable cache even with zero budget. cacheMaxAge detects a
// Past Period True-Up when trueUpStart + 1 year is before now and sets
// maxAge = 0 (never-expires sentinel).
func TestBudgetExhausted_PastTrueUp(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := lifetimeProductionServer(t)
	client, p, credName := budgetClient(t, srv.URL, uniqueSysID("ptu"))
	tz := mustLoadLocation(t, "US/Pacific")

	// A true-up that started 2 years ago ended 1 year ago → it is in the past.
	pastTrueUpStart := time.Now().In(tz).AddDate(-2, 0, 0)
	ctx := context.Background()

	_, err := client.GetProductionForDate(ctx, pastTrueUpStart, constants.QueryModeTrueUp)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("prime: expected 1 server hit, got %d", hits.Load())
	}

	exhaustBudget(t, p, credName)

	_, err = client.GetProductionForDate(ctx, pastTrueUpStart, constants.QueryModeTrueUp)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("probe: expected 1 total server hit (immutable cache), got %d", hits.Load())
	}
}

// TestBudgetExhausted_NoCache_ReturnsRateLimitError verifies that a current-
// period query returns a RateLimitError (not a panic or misleading zero) when
// the budget is zero and no cache entry exists for the endpoint.
func TestBudgetExhausted_NoCache_ReturnsRateLimitError(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	srv, hits := intervalProductionServer(t)
	client, p, credName := budgetClient(t, srv.URL, uniqueSysID("nc"))

	// Exhaust budget BEFORE any priming call so the cache is empty.
	exhaustBudget(t, p, credName)

	ctx := context.Background()
	_, err := client.GetProductionForDate(ctx, time.Time{}, constants.QueryModeDay)

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

// TestPreflightWarning_CurrentPeriod verifies that GetMetricsFromCloud prints
// a budget-insufficient warning to stdout when the remaining budget is less
// than the query cost for a Current Period.
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

	client, p, credName := budgetClient(t, srv.URL, uniqueSysID("warn"))
	ctx := context.Background()

	// Prime: populates cache for all five endpoints.
	_, _, err := client.GetMetricsFromCloud(ctx, time.Time{}, constants.QueryModeDay)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}

	// Leave exactly 1 call in the budget (< 5 needed for QueryModeDay+battery).
	for p.RemainingMinuteBudget(credName) > 1 {
		p.RecordAPICall(credName)
	}

	// Probe: preflight should warn because remaining (1) < needed (5).
	output := captureStderr(func() {
		_, _, _ = client.GetMetricsFromCloud(ctx, time.Time{}, constants.QueryModeDay)
	})

	if !strings.Contains(output, "Insufficient API budget") {
		t.Errorf("expected preflight warning in stdout, got: %q", output)
	}
}

// TestPreflightWarning_PastPeriod verifies that no preflight warning is emitted
// for Past Periods — their data is always served from immutable cache and never
// consumes budget.
func TestPreflightWarning_PastPeriod(t *testing.T) {
	cache.ResetState()
	defer cache.ResetState()

	// Mock that serves the Interval Data endpoint for the past day prime call.
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

	client, p, credName := budgetClient(t, srv.URL, uniqueSysID("pwrn"))
	tz := mustLoadLocation(t, "US/Pacific")
	pastDay := time.Now().In(tz).AddDate(0, 0, -3)
	ctx := context.Background()

	// Prime: populates cache for the past day.
	_, _, err := client.GetMetricsFromCloud(ctx, pastDay, constants.QueryModeDay)
	if err != nil {
		t.Fatalf("prime call: %v", err)
	}

	// Exhaust budget completely.
	exhaustBudget(t, p, credName)

	// Probe: Past Period → no preflight warning expected.
	output := captureStderr(func() {
		_, _, _ = client.GetMetricsFromCloud(ctx, pastDay, constants.QueryModeDay)
	})

	if strings.Contains(output, "Insufficient API budget") {
		t.Errorf("unexpected preflight warning for Past Period: %q", output)
	}
}
