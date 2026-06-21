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

// TestForSystemSpread verifies round-robin assignment across systems.
func TestForSystemSpread(t *testing.T) {
	p := makePool("a", "b")
	want := []string{"a", "b", "a", "b"}
	for i, w := range want {
		if got := p.ForSystem(i).Name; got != w {
			t.Errorf("ForSystem(%d) = %q, want %q", i, got, w)
		}
	}
}

// TestForSystemSkipsCooldown verifies a rate-limited credential is skipped while
// in cooldown and re-used once the window passes.
func TestForSystemSkipsCooldown(t *testing.T) {
	p := makePool("a", "b")
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

// TestForSystemAllCoolingDownFallsBack verifies ForSystem still returns the
// round-robin pick when every credential is in cooldown.
func TestForSystemAllCoolingDown(t *testing.T) {
	p := makePool("a", "b")
	now := time.Now()
	p.now = func() time.Time { return now }
	p.MarkUnavailable(p.creds[0])
	p.MarkUnavailable(p.creds[1])

	if got := p.ForSystem(1).Name; got != "b" {
		t.Errorf("ForSystem(1) all cooling = %q, want b (round-robin fallback)", got)
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
