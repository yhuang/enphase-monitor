// Package api - query_cost_test.go
//
// Tests for QueryCost — verifies the API call count returned for every
// combination of query mode and hasBattery flag.
package api

import (
	"testing"

	"enphase-monitor/internal/constants"
)

func TestQueryCost(t *testing.T) {
	tests := []struct {
		name       string
		queryMode  constants.QueryMode
		hasBattery bool
		want       int
	}{
		// Day queries: base 4 calls; battery adds 1 when present.
		{"day with battery", constants.QueryModeDay, true, 5},
		{"day without battery", constants.QueryModeDay, false, 4},

		// Month queries: battery endpoint is never called regardless of hasBattery.
		{"month with battery", constants.QueryModeMonth, true, 4},
		{"month without battery", constants.QueryModeMonth, false, 4},

		// Year queries: same as month.
		{"year with battery", constants.QueryModeYear, true, 4},
		{"year without battery", constants.QueryModeYear, false, 4},

		// True-up queries: same as month/year.
		{"true-up with battery", constants.QueryModeTrueUp, true, 4},
		{"true-up without battery", constants.QueryModeTrueUp, false, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := QueryCost(tc.queryMode, tc.hasBattery)
			if got != tc.want {
				t.Errorf("QueryCost(%v, hasBattery=%v) = %d, want %d",
					tc.queryMode, tc.hasBattery, got, tc.want)
			}
		})
	}
}

// TestQueryCost_FitsWithinBudget verifies that a single-system day query with
// battery still fits within the per-window budget when the budget is full.
func TestQueryCost_FitsWithinBudget(t *testing.T) {
	cost := QueryCost(constants.QueryModeDay, true)
	if cost > 10 {
		t.Errorf("QueryCost(day, battery) = %d exceeds MaxRequestsPerWindow 10", cost)
	}
}

// TestQueryCost_TwoSystemsFitBudget verifies that two systems each making a
// day+battery query sum to exactly MaxRequestsPerWindow (the documented
// architectural constraint: 2 systems × 5 metrics = 10 calls).
func TestQueryCost_TwoSystemsFitBudget(t *testing.T) {
	const systems = 2
	total := systems * QueryCost(constants.QueryModeDay, true)
	if total != 10 {
		t.Errorf("2 systems × QueryCost(day, battery) = %d, want 10 (exactly at budget)", total)
	}
}
