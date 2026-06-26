package enphase

import (
	"testing"
	"time"

	"enphase-monitor/internal/constants"
)

func TestParseHitsTotal(t *testing.T) {
	tests := []struct {
		text string
		want int
		ok   bool
	}{
		{"206 Hits (hits)", 206, true},
		{"1,340 Hits (hits)", 1340, true},
		{"show last 24 hours\n206 Hits (hits)\nGraphs", 206, true},
		{"no hits here", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseHitsTotal(tt.text)
		if ok != tt.ok || got != tt.want {
			t.Errorf("parseHitsTotal(%q) = (%d, %v), want (%d, %v)", tt.text, got, ok, tt.want, tt.ok)
		}
	}
}

func TestFirstOfMonth(t *testing.T) {
	ref := time.Date(2026, 6, 21, 15, 0, 0, 0, time.Local)
	from := firstOfMonth(ref)
	if from != "06/01/2026" {
		t.Errorf("firstOfMonth = %q, want 06/01/2026", from)
	}
}

func TestRemainingMonthly(t *testing.T) {
	if got := remainingMonthly(340); got != constants.MaxRequestsPerMonth-340 {
		t.Errorf("remainingMonthly(340) = %d, want %d", got, constants.MaxRequestsPerMonth-340)
	}
	if got := remainingMonthly(constants.MaxRequestsPerMonth + 5); got != 0 {
		t.Errorf("remainingMonthly(over limit) = %d, want 0", got)
	}
}
