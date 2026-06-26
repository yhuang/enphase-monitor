package credentials

import (
	"testing"
	"time"

	"enphase-monitor/internal/types"
)

func makePool(names ...string) *Pool {
	creds := make([]*types.APIConfig, len(names))
	for i, n := range names {
		creds[i] = &types.APIConfig{Name: n, Key: "k", ClientID: "c", ClientSecret: "s"}
	}
	return NewPool(creds)
}

// TestForSystemSpread verifies that with equal monthly usage, ForSystem assigns
// credentials in pool order (stable sort) and gives each system a distinct key.
func TestForSystemSpread(t *testing.T) {
	p := makePool("a", "b")
	useTempQuotaFile(t, p)

	if got := p.ForSystem(0).Name; got != "a" {
		t.Errorf("ForSystem(0) = %q, want a", got)
	}
	if got := p.ForSystem(1).Name; got != "b" {
		t.Errorf("ForSystem(1) = %q, want b", got)
	}
}

// TestForSystemPrefersLeastUsed verifies that the credential with fewer monthly
// hits is always preferred, so usage equalizes across the pool over time.
func TestForSystemPrefersLeastUsed(t *testing.T) {
	p := makePool("a", "b", "c")
	useTempQuotaFile(t, p)

	// Give "a" more calls than "b", "c".
	for i := 0; i < 5; i++ {
		p.RecordAPICall("a")
	}
	for i := 0; i < 2; i++ {
		p.RecordAPICall("b")
	}
	// Sorted by usage: c(0) < b(2) < a(5)
	if got := p.ForSystem(0).Name; got != "c" {
		t.Errorf("ForSystem(0) = %q, want c (least used)", got)
	}
	if got := p.ForSystem(1).Name; got != "b" {
		t.Errorf("ForSystem(1) = %q, want b (second least)", got)
	}
	if got := p.ForSystem(2).Name; got != "a" {
		t.Errorf("ForSystem(2) = %q, want a (most used)", got)
	}
}

// TestForSystemSkipsCooldown verifies a rate-limited credential is skipped while
// in cooldown and re-used once the window passes.
func TestForSystemSkipsCooldown(t *testing.T) {
	p := makePool("a", "b")
	useTempQuotaFile(t, p)
	now := time.Now()
	p.now = func() time.Time { return now }

	p.MarkUnavailable(p.creds[0]) // "a" cooling down

	if got := p.ForSystem(0).Name; got != "b" {
		t.Errorf("ForSystem(0) with a cooling down = %q, want b", got)
	}

	// After the cooldown window, "a" is eligible again.
	now = now.Add(2 * time.Minute)
	if got := p.ForSystem(0).Name; got != "a" {
		t.Errorf("ForSystem(0) after cooldown = %q, want a", got)
	}
}

// TestForSystemAllCoolingDownFallsBack verifies ForSystem returns the least-used
// credential with monthly budget when every credential is in cooldown.
func TestForSystemAllCoolingDown(t *testing.T) {
	p := makePool("a", "b")
	useTempQuotaFile(t, p)
	now := time.Now()
	p.now = func() time.Time { return now }
	p.MarkUnavailable(p.creds[0])
	p.MarkUnavailable(p.creds[1])

	// Both cooling: fallback returns least-used (equal usage → pool order → "a").
	if got := p.ForSystem(0).Name; got != "a" {
		t.Errorf("ForSystem(0) all cooling = %q, want a (least-used fallback)", got)
	}
}

// TestFailover verifies Failover advances past tried/cooled credentials.
func TestFailover(t *testing.T) {
	p := makePool("a", "b", "c")
	now := time.Now()
	p.now = func() time.Time { return now }

	tried := map[string]bool{"a": true}
	got, ok := p.Failover(tried)
	if !ok || got.Name != "b" {
		t.Fatalf("Failover(tried a) = %v, %v; want b, true", got, ok)
	}

	// b also tried and cooling: only c remains.
	tried["b"] = true
	p.MarkUnavailable(p.creds[1])
	got, ok = p.Failover(tried)
	if !ok || got.Name != "c" {
		t.Fatalf("Failover(tried a,b) = %v, %v; want c, true", got, ok)
	}

	// All tried: no spare.
	tried["c"] = true
	if _, ok := p.Failover(tried); ok {
		t.Error("Failover with all tried returned ok=true, want false")
	}
}

// TestHelpers covers First, Names, ByName, Len.
func TestHelpers(t *testing.T) {
	p := makePool("a", "b")
	if p.Len() != 2 {
		t.Errorf("Len() = %d, want 2", p.Len())
	}
	if p.First().Name != "a" {
		t.Errorf("First() = %q, want a", p.First().Name)
	}
	if got := p.Names(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Names() = %v, want [a b]", got)
	}
	if c, ok := p.ByName("b"); !ok || c.Name != "b" {
		t.Errorf("ByName(b) = %v, %v; want b, true", c, ok)
	}
	if _, ok := p.ByName("nope"); ok {
		t.Error("ByName(nope) ok = true, want false")
	}

	empty := NewPool(nil)
	if empty.First() != nil {
		t.Error("First() on empty pool != nil")
	}
}
