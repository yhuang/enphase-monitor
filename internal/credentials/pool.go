// Package credentials manages a pool of Enphase Cloud API credential sets.
//
// Each Enphase API key is rate-limited to 10 requests/minute and 1000/month. To
// scale beyond a single key the app holds a pool of credential sets and uses
// them two ways:
//
//   - Spread: ForSystem assigns credentials round-robin across systems so each
//     key handles a fraction of a run's calls, keeping every key under its
//     per-minute ceiling.
//   - Failover: when a credential is rate-limited (429) or its token acquisition
//     fails, the caller marks it (MarkUnavailable) and asks for a spare
//     (Failover); the credential stays in a cooldown for one API Budget window so
//     subsequent assignments skip it.
//
// Cooldown state is in-memory for the process lifetime, so it persists across
// Continuous Mode ticks. The pool is not safe for concurrent use; the aggregator
// drives it from a single goroutine.
package credentials

import (
	"time"

	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/types"
)

// Pool holds an ordered set of credential sets plus their cooldown state.
type Pool struct {
	creds         []*types.APIConfig
	cooldownUntil map[string]time.Time // keyed by credential name
	now           func() time.Time     // injectable clock for tests
}

// NewPool creates a pool over the given credential sets, in order.
func NewPool(creds []*types.APIConfig) *Pool {
	return &Pool{
		creds:         creds,
		cooldownUntil: make(map[string]time.Time, len(creds)),
		now:           time.Now,
	}
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

// ForSystem returns the credential to use for the system at the given index.
// The base assignment is round-robin (creds[index % len]) to spread load; if the
// assigned credential is in cooldown, ForSystem returns the next available one
// instead, falling back to the round-robin pick when every credential is cooling
// down. The pool is never empty in practice (validated at load time).
func (p *Pool) ForSystem(index int) *types.APIConfig {
	n := len(p.creds)
	if n == 0 {
		return nil
	}
	base := index % n
	if p.available(p.creds[base]) {
		return p.creds[base]
	}
	// Scan forward from the base pick for the first available credential.
	for offset := 1; offset < n; offset++ {
		c := p.creds[(base+offset)%n]
		if p.available(c) {
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

// Failover returns the next credential that is both available (not in cooldown)
// and not already in tried, for retrying a system whose credential was just
// rate-limited. It returns false when no untried, available credential remains.
func (p *Pool) Failover(tried map[string]bool) (*types.APIConfig, bool) {
	for _, c := range p.creds {
		if tried[c.Name] {
			continue
		}
		if p.available(c) {
			return c, true
		}
	}
	return nil, false
}

// available reports whether a credential is past its cooldown window.
func (p *Pool) available(c *types.APIConfig) bool {
	until, ok := p.cooldownUntil[c.Name]
	if !ok {
		return true
	}
	return !p.now().Before(until)
}
