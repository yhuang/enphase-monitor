// Package credentials manages a pool of Enphase Cloud API credential sets.
//
// Each Enphase API key is rate-limited to 10 requests/minute and 1000/month. To
// scale beyond a single key the app holds a pool of credential sets and uses
// them two ways:
//
//   - Spread: ForSystem assigns credentials round-robin across systems so each
//     key handles a fraction of a run's calls, keeping every key under its
//     per-minute ceiling. Keys at either limit are skipped proactively.
//   - Failover: when a credential is rate-limited (429) or its token acquisition
//     fails, the caller marks it (MarkUnavailable) and asks for a spare
//     (Failover); the credential stays in a cooldown for one API Budget window so
//     subsequent assignments skip it.
//
// Per-credential quota state (minute and monthly windows) lives in the pool and
// is persisted to cache/monthly-quota.json. Monthly counts are seeded from the Enphase
// developer portal during --init (or refreshed with --refresh-quota); each live
// API call increments the running total via RecordAPICall. Cooldown state is
// in-memory for the process lifetime.
//
// A Pool is not safe for concurrent use: its cooldown and quota maps are mutated
// without synchronization. The aggregator drives it from a single goroutine
// (systems are fetched sequentially); parallelizing that would require a mutex.
package credentials

import (
	"time"

	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/types"
)

// Pool holds an ordered set of credential sets plus cooldown and quota state.
type Pool struct {
	creds             []*types.APIConfig
	cooldownUntil     map[string]time.Time // keyed by credential name
	now               func() time.Time     // injectable clock for tests
	rotation          int                  // round-robin base offset; advanced by Rotate
	quota             quotaFile
	quotaFileOverride string // non-empty in tests
}

// NewPool creates a pool over the given credential sets, in order.
func NewPool(creds []*types.APIConfig) *Pool {
	p := &Pool{
		creds:         creds,
		cooldownUntil: make(map[string]time.Time, len(creds)),
		now:           time.Now,
	}
	p.loadQuota()
	return p
}

// Len reports the number of credential sets in the pool.
func (p *Pool) Len() int { return len(p.creds) }

// First returns the first credential set, or nil if the pool is empty. It is
// used by out-of-band single calls (e.g. --init) that don't need rotation.
func (p *Pool) First() *types.APIConfig {
	if len(p.creds) == 0 {
		return nil
	}
	return p.creds[0]
}

// Names returns the credential names in order, for error messages and credential selection.
func (p *Pool) Names() []string {
	names := make([]string, len(p.creds))
	for i, c := range p.creds {
		names[i] = c.Name
	}
	return names
}

// ByName returns the credential set with the given name.
func (p *Pool) ByName(name string) (*types.APIConfig, bool) {
	for _, c := range p.creds {
		if c.Name == name {
			return c, true
		}
	}
	return nil, false
}

// Rotate advances the round-robin base by step credentials, so the next batch of
// ForSystem calls starts at a fresh credential. Backfill calls this once per
// fetched day (step = number of systems) to spread load proactively across the
// pool: consecutive days use disjoint credentials, keeping each key under its
// per-minute limit instead of reusing the same few keys until they 429. A
// non-positive step or an empty pool is a no-op. (Continuous Mode never rotates,
// so each system keeps a stable credential and its cached token across ticks.)
func (p *Pool) Rotate(step int) {
	if step <= 0 || len(p.creds) == 0 {
		return
	}
	p.rotation = (p.rotation + step) % len(p.creds)
}

// ForSystem returns the credential to use for the system at the given index.
// The base assignment is round-robin (creds[(rotation+index) % len]) to spread
// load; if the assigned credential is unavailable (cooldown or quota), ForSystem
// returns the next selectable one instead. When every credential is minute- or
// cooldown-limited but at least one still has monthly budget, it falls back to
// the round-robin pick so the client can serve cache without a live call.
func (p *Pool) ForSystem(index int) *types.APIConfig {
	n := len(p.creds)
	if n == 0 {
		return nil
	}
	base := (p.rotation + index) % n
	if p.selectable(p.creds[base]) {
		return p.creds[base]
	}
	for offset := 1; offset < n; offset++ {
		c := p.creds[(base+offset)%n]
		if p.selectable(c) {
			return c
		}
	}
	if p.hasMonthlyBudget(p.creds[base]) {
		return p.creds[base]
	}
	for offset := 1; offset < n; offset++ {
		c := p.creds[(base+offset)%n]
		if p.hasMonthlyBudget(c) {
			return c
		}
	}
	return p.creds[base]
}

// MarkUnavailable puts a credential into cooldown for one API Budget window,
// after which it becomes eligible again. It is used both when a credential is
// rate-limited (429) and when its token acquisition fails, so a bad or throttled
// credential is skipped for the rest of the run instead of being retried.
func (p *Pool) MarkUnavailable(c *types.APIConfig) {
	if c == nil {
		return
	}
	p.cooldownUntil[c.Name] = p.now().Add(constants.APIBudgetWindowSeconds * time.Second)
}

// Failover returns the next credential that is both selectable (not in cooldown
// and with minute + monthly budget) and not already in tried, for retrying a
// system whose credential was just rate-limited. It returns false when no
// untried, selectable credential remains.
func (p *Pool) Failover(tried map[string]bool) (*types.APIConfig, bool) {
	for _, c := range p.creds {
		if tried[c.Name] {
			continue
		}
		if p.selectable(c) {
			return c, true
		}
	}
	return nil, false
}

// selectable reports whether a credential may make a live API call now.
func (p *Pool) selectable(c *types.APIConfig) bool {
	return p.pastCooldown(c) && p.hasMinuteBudget(c) && p.hasMonthlyBudget(c)
}

// pastCooldown reports whether a credential is past its reactive cooldown window.
func (p *Pool) pastCooldown(c *types.APIConfig) bool {
	until, ok := p.cooldownUntil[c.Name]
	if !ok {
		return true
	}
	return !p.now().Before(until)
}
