package credentials

import (
	"os"
	"path/filepath"
	"testing"

	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/types"
)

func useTempQuotaFile(t *testing.T, p *Pool) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("ENPHASE_CACHE_DIR", dir)
	p.quotaFileOverride = filepath.Join(dir, quotaFilename)
	p.loadQuota()
}

func TestForSystemSkipsMonthlyExhausted(t *testing.T) {
	p := makePool("a", "b")
	useTempQuotaFile(t, p)
	p.quota.Monthly["a"] = constants.MaxRequestsPerMonth

	if got := p.ForSystem(0).Name; got != "b" {
		t.Errorf("ForSystem(0) with a monthly-exhausted = %q, want b", got)
	}
}

func TestAllMonthlyExhausted(t *testing.T) {
	p := makePool("a", "b")
	useTempQuotaFile(t, p)
	if p.AllMonthlyExhausted() {
		t.Fatal("fresh pool should not be monthly exhausted")
	}
	p.quota.Monthly["a"] = constants.MaxRequestsPerMonth
	p.quota.Monthly["b"] = constants.MaxRequestsPerMonth
	if !p.AllMonthlyExhausted() {
		t.Error("AllMonthlyExhausted() = false, want true")
	}
}

func TestQuotaSummary(t *testing.T) {
	p := makePool("a", "b")
	useTempQuotaFile(t, p)
	p.quota.Monthly["a"] = 340
	p.quota.Monthly["b"] = constants.MaxRequestsPerMonth

	got := p.QuotaSummary()
	want := "Pool quota: 1,340 / 2,000 this month (67%); 1 key exhausted."
	if got != want {
		t.Errorf("QuotaSummary() = %q, want %q", got, want)
	}
}

func TestQuotaPersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENPHASE_CACHE_DIR", dir)
	path := filepath.Join(dir, quotaFilename)

	p := makePool("a")
	p.quotaFileOverride = path
	p.RecordAPICall("a")

	p2 := makePool("a")
	p2.quotaFileOverride = path
	p2.loadQuota()

	if p2.monthlyCount("a") != 1 {
		t.Errorf("reloaded monthly count = %d, want 1", p2.monthlyCount("a"))
	}
	_ = os.Remove(path)
}

func TestNewPoolInitializesQuotaForCredentials(t *testing.T) {
	creds := []*types.APIConfig{{Name: "x"}, {Name: "y"}}
	p := NewPool(creds)
	useTempQuotaFile(t, p)
	if p.PoolMonthlyCapacity() != 2*constants.MaxRequestsPerMonth {
		t.Errorf("PoolMonthlyCapacity() = %d, want %d", p.PoolMonthlyCapacity(), 2*constants.MaxRequestsPerMonth)
	}
}
