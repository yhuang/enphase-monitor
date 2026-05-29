package urlbuilder

import (
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/constants"
)

func TestBuildTelemetryURL(t *testing.T) {
	tests := []struct {
		name     string
		systemID string
		endpoint string
		apiKey   string
		dayStart time.Time
		dayEnd   time.Time
		want     struct {
			containsBaseURL  bool
			containsSystemID bool
			containsEndpoint bool
			containsKey      bool
			containsStartAt  bool
			containsEndAt    bool
		}
	}{
		{
			name:     "production endpoint",
			systemID: "12345",
			endpoint: "telemetry/production_meter",
			apiKey:   "test-api-key",
			dayStart: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			dayEnd:   time.Date(2026, 1, 15, 23, 59, 59, 0, time.UTC),
			want: struct {
				containsBaseURL  bool
				containsSystemID bool
				containsEndpoint bool
				containsKey      bool
				containsStartAt  bool
				containsEndAt    bool
			}{
				containsBaseURL:  true,
				containsSystemID: true,
				containsEndpoint: true,
				containsKey:      true,
				containsStartAt:  true,
				containsEndAt:    true,
			},
		},
		{
			name:     "battery endpoint",
			systemID: "67890",
			endpoint: "telemetry/battery",
			apiKey:   "another-key",
			dayStart: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
			dayEnd:   time.Date(2026, 1, 20, 23, 59, 59, 0, time.UTC),
			want: struct {
				containsBaseURL  bool
				containsSystemID bool
				containsEndpoint bool
				containsKey      bool
				containsStartAt  bool
				containsEndAt    bool
			}{
				containsBaseURL:  true,
				containsSystemID: true,
				containsEndpoint: true,
				containsKey:      true,
				containsStartAt:  true,
				containsEndAt:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildTelemetryURL(constants.EnphaseAPIv4SystemsURL, tt.systemID, tt.endpoint, tt.apiKey, tt.dayStart, tt.dayEnd)

			// Verify URL contains expected components
			if tt.want.containsBaseURL && !strings.Contains(got, constants.EnphaseAPIv4SystemsURL) {
				t.Errorf("BuildTelemetryURL() missing base URL, got = %v", got)
			}
			if tt.want.containsSystemID && !strings.Contains(got, tt.systemID) {
				t.Errorf("BuildTelemetryURL() missing system ID, got = %v", got)
			}
			if tt.want.containsEndpoint && !strings.Contains(got, tt.endpoint) {
				t.Errorf("BuildTelemetryURL() missing endpoint, got = %v", got)
			}
			if tt.want.containsKey && !strings.Contains(got, "key="+tt.apiKey) {
				t.Errorf("BuildTelemetryURL() missing API key parameter, got = %v", got)
			}
			if tt.want.containsStartAt && !strings.Contains(got, "start_at=") {
				t.Errorf("BuildTelemetryURL() missing start_at parameter, got = %v", got)
			}
			if tt.want.containsEndAt && !strings.Contains(got, "end_at=") {
				t.Errorf("BuildTelemetryURL() missing end_at parameter, got = %v", got)
			}

			// Verify URL format
			expectedPrefix := constants.EnphaseAPIv4SystemsURL + "/" + tt.systemID + "/" + tt.endpoint
			if !strings.HasPrefix(got, expectedPrefix) {
				t.Errorf("BuildTelemetryURL() wrong prefix, got = %v, want prefix = %v", got, expectedPrefix)
			}
		})
	}
}

func TestBuildTelemetryURL_TimestampConversion(t *testing.T) {
	systemID := "test-system"
	endpoint := "telemetry/production_meter"
	apiKey := "test-key"
	dayStart := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	dayEnd := time.Date(2026, 1, 15, 23, 59, 59, 0, time.UTC)

	url := BuildTelemetryURL(constants.EnphaseAPIv4SystemsURL, systemID, endpoint, apiKey, dayStart, dayEnd)

	// Verify Unix timestamps are included
	expectedStartUnix := dayStart.Unix()
	expectedEndUnix := dayEnd.Unix()

	// Verify timestamps are numeric
	if !strings.Contains(url, "start_at=") || !strings.Contains(url, "end_at=") {
		t.Errorf("BuildTelemetryURL() missing timestamp parameters, got = %v", url)
	}
	if strings.Contains(url, "start_at=0") && expectedStartUnix != 0 {
		t.Errorf("BuildTelemetryURL() wrong start timestamp, expected non-zero")
	}
	if strings.Contains(url, "end_at=0") && expectedEndUnix != 0 {
		t.Errorf("BuildTelemetryURL() wrong end timestamp, expected non-zero")
	}
}

func TestBuildTelemetryURL_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		systemID string
		endpoint string
		apiKey   string
	}{
		{
			name:     "system ID with numbers",
			systemID: "123456789",
			endpoint: "telemetry/production_meter",
			apiKey:   "api-key-123",
		},
		{
			name:     "complex endpoint path",
			systemID: "system-id",
			endpoint: "energy_import_telemetry",
			apiKey:   "key-with-dashes",
		},
	}

	dayStart := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	dayEnd := time.Date(2026, 1, 15, 23, 59, 59, 0, time.UTC)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := BuildTelemetryURL(constants.EnphaseAPIv4SystemsURL, tt.systemID, tt.endpoint, tt.apiKey, dayStart, dayEnd)

			// Verify all components are present
			if !strings.Contains(url, tt.systemID) {
				t.Errorf("BuildTelemetryURL() missing system ID: %v", tt.systemID)
			}
			if !strings.Contains(url, tt.endpoint) {
				t.Errorf("BuildTelemetryURL() missing endpoint: %v", tt.endpoint)
			}
			if !strings.Contains(url, tt.apiKey) {
				t.Errorf("BuildTelemetryURL() missing API key: %v", tt.apiKey)
			}
		})
	}
}

func TestBuildLifetimeURL(t *testing.T) {
	got := BuildLifetimeURL(constants.EnphaseAPIv4SystemsURL, "12345", "energy_lifetime", "test-key", "2026-01-15")

	expectedPrefix := constants.EnphaseAPIv4SystemsURL + "/12345/energy_lifetime"
	if !strings.HasPrefix(got, expectedPrefix) {
		t.Errorf("BuildLifetimeURL() wrong prefix, got = %v, want prefix = %v", got, expectedPrefix)
	}
	if !strings.Contains(got, "key=test-key") {
		t.Errorf("BuildLifetimeURL() missing API key, got = %v", got)
	}
	if !strings.Contains(got, "start_date=2026-01-15") {
		t.Errorf("BuildLifetimeURL() missing start_date parameter, got = %v", got)
	}
	// Lifetime URLs use start_date, never the interval start_at/end_at params.
	if strings.Contains(got, "start_at=") || strings.Contains(got, "end_at=") {
		t.Errorf("BuildLifetimeURL() should not contain interval params, got = %v", got)
	}
}

func TestBuildLifetimeURL_CustomBase(t *testing.T) {
	got := BuildLifetimeURL("http://test.local", "sys-1", "consumption_lifetime", "k", "2025-12-31")
	if !strings.HasPrefix(got, "http://test.local/sys-1/consumption_lifetime") {
		t.Errorf("BuildLifetimeURL() did not honor custom base, got = %v", got)
	}
}
