package credentials

import "testing"

func TestNamesWithPrefix(t *testing.T) {
	p := makePool("enphase-monitor-01", "other", "enphase-monitor-02")
	got := p.NamesWithPrefix("enphase-monitor-")
	want := []string{"enphase-monitor-01", "enphase-monitor-02"}
	if len(got) != len(want) {
		t.Fatalf("NamesWithPrefix() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("NamesWithPrefix()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHasMonthlyBaseline(t *testing.T) {
	p := makePool("a", "b")
	useTempQuotaFile(t, p)

	if p.HasMonthlyBaseline("") {
		t.Fatal("empty monthly map should not have baseline")
	}

	p.quota.Monthly["a"] = 10
	if p.HasMonthlyBaseline("") {
		t.Fatal("partial monthly map should not have baseline")
	}

	p.quota.Monthly["b"] = 0
	if !p.HasMonthlyBaseline("") {
		t.Error("all keys present (including zero) should have baseline")
	}

	if p.HasMonthlyBaseline("enphase-") {
		t.Error("prefix with no matches should not have baseline")
	}
}

func TestApplyPortalMonthlyUsage(t *testing.T) {
	p := makePool("a", "b")
	useTempQuotaFile(t, p)

	p.ApplyPortalMonthlyUsage(map[string]int{"a": 500, "b": 1000})
	if p.monthlyCount("a") != 500 {
		t.Errorf("monthly a = %d, want 500", p.monthlyCount("a"))
	}
	if p.monthlyCount("b") != 1000 {
		t.Errorf("monthly b = %d, want 1000", p.monthlyCount("b"))
	}
	if p.MonthlyExhaustedCount() != 1 {
		t.Errorf("MonthlyExhaustedCount() = %d, want 1", p.MonthlyExhaustedCount())
	}
}
