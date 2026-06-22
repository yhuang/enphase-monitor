package aggregator

import (
	"context"
	"errors"
	"testing"
	"time"

	"enphase-monitor/internal/api"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/credentials"
)

// poolOf wraps credential sets in a pool for the aggregator tests.
func poolOf(creds ...*APIConfig) *credentials.Pool {
	return credentials.NewPool(creds)
}

// mustLoadLocation loads a timezone for tests; fails the test if the timezone is invalid.
func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// MockCloudClient implements CloudClient for testing.
type MockCloudClient struct {
	Metrics   *api.LocalMetrics
	CacheUsed bool
	Err       error
}

// GetMetricsFromCloud returns the mock metrics or error.
func (m *MockCloudClient) GetMetricsFromCloud(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (*api.LocalMetrics, bool, error) {
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	if m.Err != nil {
		return nil, false, m.Err
	}
	return m.Metrics, m.CacheUsed, nil
}

// GetEnergyImportForDate returns mock energy import.
func (m *MockCloudClient) GetEnergyImportForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	if m.Metrics != nil {
		return m.Metrics.GridImportToday, nil
	}
	return 0, nil
}

// GetEnergyExportForDate returns mock energy export.
func (m *MockCloudClient) GetEnergyExportForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	if m.Metrics != nil {
		return m.Metrics.GridExportToday, nil
	}
	return 0, nil
}

// GetProductionForDate returns mock production.
func (m *MockCloudClient) GetProductionForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	if m.Metrics != nil {
		return m.Metrics.ProductionToday, nil
	}
	return 0, nil
}

// GetConsumptionForDate returns mock consumption.
func (m *MockCloudClient) GetConsumptionForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (float64, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	if m.Metrics != nil {
		return m.Metrics.ConsumptionToday, nil
	}
	return 0, nil
}

// GetBatteryDataForDate returns mock battery data.
func (m *MockCloudClient) GetBatteryDataForDate(ctx context.Context, testDate time.Time, queryMode constants.QueryMode) (charged float64, discharged float64, soc int, err error) {
	if m.Err != nil {
		return 0, 0, 0, m.Err
	}
	if m.Metrics != nil {
		return m.Metrics.BatteryChargedToday, m.Metrics.BatteryDischargedToday, m.Metrics.BatterySOC, nil
	}
	return 0, 0, 0, nil
}

// TestNewDataAggregator verifies the default constructor.
func TestNewDataAggregator(t *testing.T) {
	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		return "test-token", nil
	}

	agg := NewDataAggregator(mockTokenGetter)

	if agg == nil {
		t.Fatal("NewDataAggregator returned nil")
	}
	if agg.getAccessToken == nil {
		t.Error("getAccessToken should not be nil")
	}
	if agg.createCloudClient == nil {
		t.Error("createCloudClient should not be nil")
	}
}

// TestNewDataAggregatorWithFactory verifies the factory constructor.
func TestNewDataAggregatorWithFactory(t *testing.T) {
	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		return "test-token", nil
	}
	mockFactory := func(systemID, systemName, apiKey, accessToken, credName string, tz *time.Location, budget api.BudgetTracker) CloudClient {
		return &MockCloudClient{}
	}

	agg := NewDataAggregatorWithFactory(mockTokenGetter, mockFactory)

	if agg == nil {
		t.Fatal("NewDataAggregatorWithFactory returned nil")
	}
}

// TestGetAggregatedMetrics_SingleSystem verifies aggregation for a single system.
func TestGetAggregatedMetrics_SingleSystem(t *testing.T) {
	mockClient := &MockCloudClient{
		Metrics: &api.LocalMetrics{
			ProductionToday:        10.5,
			ConsumptionToday:       8.2,
			GridImportToday:        2.0,
			GridExportToday:        4.3,
			BatteryChargedToday:    1.5,
			BatteryDischargedToday: 0.5,
			BatterySOC:             75,
		},
		CacheUsed: false,
	}

	mockFactory := func(systemID, systemName, apiKey, accessToken, credName string, tz *time.Location, budget api.BudgetTracker) CloudClient {
		return mockClient
	}

	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		return "test-token", nil
	}

	agg := NewDataAggregatorWithFactory(mockTokenGetter, mockFactory)

	systems := []SystemConfig{{Name: "Test System", ID: "123"}}
	pool := poolOf(&APIConfig{Name: "key1", Key: "test-key", ClientID: "test-client", ClientSecret: "test-secret"})
	tz := mustLoadLocation(t, "US/Pacific")

	metrics, err := agg.GetAggregatedMetrics(context.Background(), systems, pool, time.Time{}, constants.QueryModeDay, tz)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if metrics.ProductionToday != 10.5 {
		t.Errorf("Expected production 10.5, got %v", metrics.ProductionToday)
	}
	if metrics.ConsumptionToday != 8.2 {
		t.Errorf("Expected consumption 8.2, got %v", metrics.ConsumptionToday)
	}
	if metrics.GridImportToday != 2.0 {
		t.Errorf("Expected grid import 2.0, got %v", metrics.GridImportToday)
	}
	if metrics.GridExportToday != 4.3 {
		t.Errorf("Expected grid export 4.3, got %v", metrics.GridExportToday)
	}
	if len(metrics.Systems) != 1 {
		t.Errorf("Expected 1 system, got %d", len(metrics.Systems))
	}
}

// TestGetAggregatedMetrics_MultipleSystems verifies aggregation for multiple systems.
func TestGetAggregatedMetrics_MultipleSystems(t *testing.T) {
	callCount := 0
	mockFactory := func(systemID, systemName, apiKey, accessToken, credName string, tz *time.Location, budget api.BudgetTracker) CloudClient {
		callCount++
		// Return different metrics for each system
		if systemID == "123" {
			return &MockCloudClient{
				Metrics: &api.LocalMetrics{
					ProductionToday:  10.0,
					ConsumptionToday: 8.0,
					GridImportToday:  3.0,
					GridExportToday:  5.0,
				},
				CacheUsed: false,
			}
		}
		return &MockCloudClient{
			Metrics: &api.LocalMetrics{
				ProductionToday:  5.0,
				ConsumptionToday: 4.0,
				GridImportToday:  1.0,
				GridExportToday:  2.0,
			},
			CacheUsed: true,
		}
	}

	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		return "test-token", nil
	}

	agg := NewDataAggregatorWithFactory(mockTokenGetter, mockFactory)

	systems := []SystemConfig{
		{Name: "System 1", ID: "123"},
		{Name: "System 2", ID: "456"},
	}
	pool := poolOf(&APIConfig{Name: "key1", Key: "test-key", ClientID: "test-client", ClientSecret: "test-secret"})
	tz := mustLoadLocation(t, "US/Pacific")

	metrics, err := agg.GetAggregatedMetrics(context.Background(), systems, pool, time.Time{}, constants.QueryModeDay, tz)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify aggregated totals
	expectedProduction := 15.0 // 10.0 + 5.0
	if metrics.ProductionToday != expectedProduction {
		t.Errorf("Expected production %v, got %v", expectedProduction, metrics.ProductionToday)
	}

	expectedConsumption := 12.0 // 8.0 + 4.0
	if metrics.ConsumptionToday != expectedConsumption {
		t.Errorf("Expected consumption %v, got %v", expectedConsumption, metrics.ConsumptionToday)
	}

	expectedImport := 4.0 // 3.0 + 1.0
	if metrics.GridImportToday != expectedImport {
		t.Errorf("Expected grid import %v, got %v", expectedImport, metrics.GridImportToday)
	}

	expectedExport := 7.0 // 5.0 + 2.0
	if metrics.GridExportToday != expectedExport {
		t.Errorf("Expected grid export %v, got %v", expectedExport, metrics.GridExportToday)
	}

	// Net flow = import - export = 4.0 - 7.0 = -3.0 (net export)
	expectedNetFlow := -3.0
	if metrics.NetFlowToday != expectedNetFlow {
		t.Errorf("Expected net flow %v, got %v", expectedNetFlow, metrics.NetFlowToday)
	}

	// Verify individual systems
	if len(metrics.Systems) != 2 {
		t.Errorf("Expected 2 systems, got %d", len(metrics.Systems))
	}

	// Verify cache status (any system using cache means CacheUsed=true)
	if !metrics.CacheUsed {
		t.Error("CacheUsed should be true when any system uses cache")
	}

	// Verify factory was called twice
	if callCount != 2 {
		t.Errorf("Expected factory to be called 2 times, got %d", callCount)
	}
}

// TestGetAggregatedMetrics_MissingAPIConfig verifies error handling for missing API config.
func TestGetAggregatedMetrics_MissingAPIConfig(t *testing.T) {
	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		return "test-token", nil
	}

	agg := NewDataAggregator(mockTokenGetter)

	systems := []SystemConfig{{Name: "Test System", ID: "123"}}
	tz := mustLoadLocation(t, "US/Pacific")

	_, err := agg.GetAggregatedMetrics(context.Background(), systems, poolOf(), time.Time{}, constants.QueryModeDay, tz)

	if err == nil {
		t.Error("Expected error for empty credential pool")
	}
}

// TestGetAggregatedMetrics_MissingAPIKey verifies error handling for missing API key.
func TestGetAggregatedMetrics_MissingAPIKey(t *testing.T) {
	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		return "test-token", nil
	}

	agg := NewDataAggregator(mockTokenGetter)

	systems := []SystemConfig{{Name: "Test System", ID: "123"}}
	pool := poolOf(&APIConfig{Name: "key1", ClientID: "test-client", ClientSecret: "test-secret"}) // No Key
	tz := mustLoadLocation(t, "US/Pacific")

	_, err := agg.GetAggregatedMetrics(context.Background(), systems, pool, time.Time{}, constants.QueryModeDay, tz)

	if err == nil {
		t.Error("Expected error for missing API key")
	}
}

// TestGetAggregatedMetrics_TokenError verifies error handling for token retrieval failure.
func TestGetAggregatedMetrics_TokenError(t *testing.T) {
	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		return "", errors.New("token retrieval failed")
	}

	agg := NewDataAggregator(mockTokenGetter)

	systems := []SystemConfig{{Name: "Test System", ID: "123"}}
	pool := poolOf(&APIConfig{Name: "key1", Key: "test-key", ClientID: "test-client", ClientSecret: "test-secret"})
	tz := mustLoadLocation(t, "US/Pacific")

	_, err := agg.GetAggregatedMetrics(context.Background(), systems, pool, time.Time{}, constants.QueryModeDay, tz)

	if err == nil {
		t.Error("Expected error for token retrieval failure")
	}
}

// TestGetAggregatedMetrics_FailoverOn429 verifies a rate-limited credential fails
// over to a spare credential and the system still succeeds.
func TestGetAggregatedMetrics_FailoverOn429(t *testing.T) {
	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		return "test-token", nil
	}
	// The first credential ("k1") is throttled; the spare ("k2") returns metrics.
	mockFactory := func(systemID, systemName, apiKey, accessToken, credName string, tz *time.Location, budget api.BudgetTracker) CloudClient {
		if apiKey == "k1" {
			return &MockCloudClient{Err: errors.New(constants.RateLimitError)}
		}
		return &MockCloudClient{Metrics: &api.LocalMetrics{ProductionToday: 9.0}}
	}

	agg := NewDataAggregatorWithFactory(mockTokenGetter, mockFactory)

	systems := []SystemConfig{{Name: "Test System", ID: "123"}}
	pool := poolOf(
		&APIConfig{Name: "key1", Key: "k1", ClientID: "c1", ClientSecret: "s"},
		&APIConfig{Name: "key2", Key: "k2", ClientID: "c2", ClientSecret: "s"},
	)
	tz := mustLoadLocation(t, "US/Pacific")

	metrics, err := agg.GetAggregatedMetrics(context.Background(), systems, pool, time.Time{}, constants.QueryModeDay, tz)
	if err != nil {
		t.Fatalf("Unexpected error after failover: %v", err)
	}
	if metrics.ProductionToday != 9.0 {
		t.Errorf("ProductionToday = %v, want 9.0 (from spare credential)", metrics.ProductionToday)
	}
}

// TestGetAggregatedMetrics_TokenFailover verifies that when token acquisition
// fails for one credential, the system fails over to a spare and still succeeds.
func TestGetAggregatedMetrics_TokenFailover(t *testing.T) {
	// key1's token fetch fails (e.g. expired refresh token / Enphase 500); key2 works.
	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		if apiConfig.Name == "key1" {
			return "", errors.New("token request failed with status 500")
		}
		return "test-token", nil
	}
	mockFactory := func(systemID, systemName, apiKey, accessToken, credName string, tz *time.Location, budget api.BudgetTracker) CloudClient {
		return &MockCloudClient{Metrics: &api.LocalMetrics{ProductionToday: 7.0}}
	}

	agg := NewDataAggregatorWithFactory(mockTokenGetter, mockFactory)

	systems := []SystemConfig{{Name: "Test System", ID: "123"}}
	pool := poolOf(
		&APIConfig{Name: "key1", Key: "k1", ClientID: "c1", ClientSecret: "s"},
		&APIConfig{Name: "key2", Key: "k2", ClientID: "c2", ClientSecret: "s"},
	)
	tz := mustLoadLocation(t, "US/Pacific")

	metrics, err := agg.GetAggregatedMetrics(context.Background(), systems, pool, time.Time{}, constants.QueryModeDay, tz)
	if err != nil {
		t.Fatalf("Unexpected error after token failover: %v", err)
	}
	if metrics.ProductionToday != 7.0 {
		t.Errorf("ProductionToday = %v, want 7.0 (from spare credential)", metrics.ProductionToday)
	}
}

// TestGetAggregatedMetrics_AllCredentialsTokenFail verifies a fatal error is
// returned once every credential's token acquisition fails.
func TestGetAggregatedMetrics_AllCredentialsTokenFail(t *testing.T) {
	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		return "", errors.New("token request failed with status 500")
	}
	mockFactory := func(systemID, systemName, apiKey, accessToken, credName string, tz *time.Location, budget api.BudgetTracker) CloudClient {
		return &MockCloudClient{Metrics: &api.LocalMetrics{}}
	}

	agg := NewDataAggregatorWithFactory(mockTokenGetter, mockFactory)

	systems := []SystemConfig{{Name: "Test System", ID: "123"}}
	pool := poolOf(
		&APIConfig{Name: "key1", Key: "k1", ClientID: "c1", ClientSecret: "s"},
		&APIConfig{Name: "key2", Key: "k2", ClientID: "c2", ClientSecret: "s"},
	)
	tz := mustLoadLocation(t, "US/Pacific")

	if _, err := agg.GetAggregatedMetrics(context.Background(), systems, pool, time.Time{}, constants.QueryModeDay, tz); err == nil {
		t.Error("err = nil, want a fatal token error after all credentials exhausted")
	}
}

// TestGetAggregatedMetrics_AllCredentialsRateLimited verifies a rate-limit error is
// surfaced once every credential is exhausted for a system.
func TestGetAggregatedMetrics_AllCredentialsRateLimited(t *testing.T) {
	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		return "test-token", nil
	}
	mockFactory := func(systemID, systemName, apiKey, accessToken, credName string, tz *time.Location, budget api.BudgetTracker) CloudClient {
		return &MockCloudClient{Err: errors.New(constants.RateLimitError)}
	}

	agg := NewDataAggregatorWithFactory(mockTokenGetter, mockFactory)

	systems := []SystemConfig{{Name: "Test System", ID: "123"}}
	pool := poolOf(
		&APIConfig{Name: "key1", Key: "k1", ClientID: "c1", ClientSecret: "s"},
		&APIConfig{Name: "key2", Key: "k2", ClientID: "c2", ClientSecret: "s"},
	)
	tz := mustLoadLocation(t, "US/Pacific")

	_, err := agg.GetAggregatedMetrics(context.Background(), systems, pool, time.Time{}, constants.QueryModeDay, tz)
	if err == nil || !constants.IsRateLimitError(err) {
		t.Errorf("err = %v, want a rate-limit error after all credentials exhausted", err)
	}
}

// TestGetAggregatedMetrics_AllMonthlyExhausted verifies a clear error when every
// credential has spent its monthly API budget.
func TestGetAggregatedMetrics_AllMonthlyExhausted(t *testing.T) {
	pool := poolOf(
		&APIConfig{Name: "key1", Key: "k1", ClientID: "c1", ClientSecret: "s"},
		&APIConfig{Name: "key2", Key: "k2", ClientID: "c2", ClientSecret: "s"},
	)
	dir := t.TempDir()
	t.Setenv("ENPHASE_CACHE_DIR", dir)
	pool = credentials.NewPool([]*APIConfig{
		{Name: "key1", Key: "k1", ClientID: "c1", ClientSecret: "s"},
		{Name: "key2", Key: "k2", ClientID: "c2", ClientSecret: "s"},
	})
	for _, name := range pool.Names() {
		for i := 0; i < constants.MaxRequestsPerMonth; i++ {
			pool.RecordAPICall(name)
		}
	}

	agg := NewDataAggregator(func(context.Context, *APIConfig) (string, error) { return "token", nil })
	systems := []SystemConfig{{Name: "Test System", ID: "123"}}
	tz := mustLoadLocation(t, "US/Pacific")

	_, err := agg.GetAggregatedMetrics(context.Background(), systems, pool, time.Time{}, constants.QueryModeDay, tz)
	if err == nil || !constants.IsPoolMonthlyQuotaExhaustedError(err) {
		t.Errorf("err = %v, want pool monthly quota exhausted", err)
	}
}

// TestGetAggregatedMetrics_ContextCancellation verifies context cancellation is handled.
func TestGetAggregatedMetrics_ContextCancellation(t *testing.T) {
	mockTokenGetter := func(ctx context.Context, apiConfig *APIConfig) (string, error) {
		return "test-token", nil
	}

	agg := NewDataAggregator(mockTokenGetter)

	systems := []SystemConfig{{Name: "Test System", ID: "123"}}
	pool := poolOf(&APIConfig{Name: "key1", Key: "test-key", ClientID: "test-client", ClientSecret: "test-secret"})
	tz := mustLoadLocation(t, "US/Pacific")

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := agg.GetAggregatedMetrics(ctx, systems, pool, time.Time{}, constants.QueryModeDay, tz)

	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}
