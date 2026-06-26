package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"enphase-monitor/internal/cache"
	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/types"
)

const quotaFilename = "monthly-quota.json"

// quotaFile persists per-credential monthly call counts on disk.
// Monthly values are seeded from the developer portal (--init / --refresh-quota)
// and incremented on each live API call (RecordAPICall).
type quotaFile struct {
	Month   string         `json:"month"`
	Monthly map[string]int `json:"monthly"`
}

func (p *Pool) loadQuota() {
	p.quota = quotaFile{
		Month:   p.currentMonth(),
		Monthly: make(map[string]int, len(p.creds)),
	}
	data, err := os.ReadFile(p.quotaPath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "WARNING: could not read quota file %s, starting with empty quota: %v\n", p.quotaPath(), err)
			return
		}
		// Legacy filename from before the monthly-quota.json rename.
		legacy := filepath.Join(filepath.Dir(p.quotaPath()), "quota.json")
		data, err = os.ReadFile(legacy)
		if err != nil {
			return
		}
	}
	var loaded quotaFile
	if err := json.Unmarshal(data, &loaded); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not parse quota file %s, starting with empty quota: %v\n", p.quotaPath(), err)
		return
	}
	if loaded.Monthly == nil {
		loaded.Monthly = make(map[string]int)
	}
	if loaded.Month != p.currentMonth() {
		loaded.Month = p.currentMonth()
		loaded.Monthly = make(map[string]int, len(p.creds))
	}
	p.quota = loaded
}

func (p *Pool) saveQuota() {
	if err := os.MkdirAll(filepath.Dir(p.quotaPath()), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not create quota directory: %v\n", err)
		return
	}
	data, err := json.MarshalIndent(p.quota, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not encode quota: %v\n", err)
		return
	}
	data = append(data, '\n')
	// Quota tracking guards the 1000/month cap; a silent save failure would let
	// counts reset on restart and the cap be exceeded unnoticed, so warn loudly.
	if err := os.WriteFile(p.quotaPath(), data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not save quota to %s: %v\n", p.quotaPath(), err)
	}
}

func (p *Pool) quotaPath() string {
	if p.quotaFileOverride != "" {
		return p.quotaFileOverride
	}
	return filepath.Join(cache.GetCacheDir(), quotaFilename)
}

func (p *Pool) currentMonth() string {
	return p.now().Format("2006-01")
}

func (p *Pool) ensureMonthCurrent() {
	if p.quota.Month == p.currentMonth() {
		return
	}
	p.quota.Month = p.currentMonth()
	p.quota.Monthly = make(map[string]int, len(p.creds))
	p.saveQuota()
}

// monthlyCount returns calls recorded this calendar month.
func (p *Pool) monthlyCount(name string) int {
	p.ensureMonthCurrent()
	return p.quota.Monthly[name]
}

// hasMonthlyBudget reports whether the credential still has monthly quota left.
func (p *Pool) hasMonthlyBudget(c *types.APIConfig) bool {
	if c == nil {
		return false
	}
	return p.monthlyCount(c.Name) < constants.MaxRequestsPerMonth
}

// RecordAPICall implements api.BudgetTracker. It rewrites the whole quota file
// on every call to keep counts durable across restarts; the file is small and
// runs make few calls, so the cost is negligible even during backfill.
func (p *Pool) RecordAPICall(credentialName string) {
	p.ensureMonthCurrent()
	p.quota.Monthly[credentialName]++
	p.saveQuota()
}

// AllMonthlyExhausted reports whether every credential in the pool has spent
// its monthly API budget.
func (p *Pool) AllMonthlyExhausted() bool {
	if len(p.creds) == 0 {
		return false
	}
	for _, c := range p.creds {
		if p.hasMonthlyBudget(c) {
			return false
		}
	}
	return true
}

// MonthlyExhaustedCount returns how many credentials have no monthly budget left.
func (p *Pool) MonthlyExhaustedCount() int {
	n := 0
	for _, c := range p.creds {
		if !p.hasMonthlyBudget(c) {
			n++
		}
	}
	return n
}

// PoolMonthlyUsed returns total live API calls recorded this month across the pool.
func (p *Pool) PoolMonthlyUsed() int {
	total := 0
	for _, c := range p.creds {
		total += p.monthlyCount(c.Name)
	}
	return total
}

// PoolMonthlyCapacity returns the combined monthly budget (keys × 1000).
func (p *Pool) PoolMonthlyCapacity() int {
	return len(p.creds) * constants.MaxRequestsPerMonth
}

// QuotaSummary returns a human-readable pool quota line for logging.
func (p *Pool) QuotaSummary() string {
	used := p.PoolMonthlyUsed()
	capacity := p.PoolMonthlyCapacity()
	pct := 0
	if capacity > 0 {
		pct = used * 100 / capacity
	}
	exhausted := p.MonthlyExhaustedCount()
	keyWord := "keys"
	if exhausted == 1 {
		keyWord = "key"
	}
	return fmt.Sprintf("Pool quota: %s / %s this month (%d%%); %d %s exhausted.",
		formatWithCommas(used), formatWithCommas(capacity), pct, exhausted, keyWord)
}

func formatWithCommas(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ApplyPortalMonthlyUsage replaces per-credential monthly counts with values
// scraped from the Enphase developer portal. Keys are credential names. Live
// calls after this baseline continue to increment via RecordAPICall.
func (p *Pool) ApplyPortalMonthlyUsage(usage map[string]int) {
	p.ensureMonthCurrent()
	for name, used := range usage {
		if used < 0 {
			continue
		}
		p.quota.Monthly[name] = used
	}
	p.saveQuota()
}

// NamesWithPrefix returns credential names in pool order matching prefix.
func (p *Pool) NamesWithPrefix(prefix string) []string {
	names := make([]string, 0, len(p.creds))
	for _, c := range p.creds {
		if prefix == "" || strings.HasPrefix(c.Name, prefix) {
			names = append(names, c.Name)
		}
	}
	return names
}

// HasMonthlyBaseline reports whether every credential matching namePrefix has a
// monthly usage entry for the current calendar month (including zero usage).
func (p *Pool) HasMonthlyBaseline(namePrefix string) bool {
	p.ensureMonthCurrent()
	matched := 0
	for _, c := range p.creds {
		if namePrefix != "" && !strings.HasPrefix(c.Name, namePrefix) {
			continue
		}
		matched++
		if _, ok := p.quota.Monthly[c.Name]; !ok {
			return false
		}
	}
	return matched > 0
}
