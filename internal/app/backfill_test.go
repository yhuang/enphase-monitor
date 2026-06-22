package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/aggregator"
	"enphase-monitor/internal/api"
	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/credentials"
	"enphase-monitor/internal/types"
)

// TestBackfillDay_PartialSystemsFails verifies the integrity guard: when one
// system's fetch fails (a network blip), the aggregator skips it and returns a
// partial result without erroring — and backfillDay must turn that into a hard
// error so the day is retried, never written incomplete.
func TestBackfillDay_PartialSystemsFails(t *testing.T) {
	tokenGetter := func(ctx context.Context, _ *types.APIConfig) (string, error) {
		return "tok", nil
	}
	// System "1" returns data; system "2" fails with a non-rate-limit error, which
	// the aggregator warns about and skips.
	agg := aggregator.NewDataAggregatorWithFactory(tokenGetter,
		func(systemID, systemName, apiKey, accessToken string, tz *time.Location) aggregator.CloudClient {
			if systemID == "2" {
				return &mockCloudClient{err: errors.New("network blip")}
			}
			return &mockCloudClient{metrics: &api.LocalMetrics{ProductionToday: 10}}
		})

	cfg := &config.Config{
		Systems: []types.SystemConfig{{Name: "A", ID: "1"}, {Name: "B", ID: "2"}},
		Credentials: []*types.APIConfig{{
			Name: "k", Key: "key", ClientID: "c", ClientSecret: "s", RefreshToken: "r",
		}},
	}
	rc := RunConfig{
		Agg:       agg,
		Pool:      credentials.NewPool(cfg.Credentials),
		Cfg:       cfg,
		QueryMode: constants.QueryModeDay,
		ReportTZ:  time.UTC,
	}

	err := backfillDay(context.Background(), rc, time.Date(2025, 11, 8, 0, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "incomplete day") {
		t.Errorf("backfillDay with a failed system: error = %v, want an 'incomplete day' error", err)
	}
}
