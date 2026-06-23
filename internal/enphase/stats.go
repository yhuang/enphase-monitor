// stats.go reads per-application monthly API hit totals from the Enphase
// developer portal stats page (developer-v4.enphase.com/buyer/stats), which
// exposes no public API. Used by --init (baseline seed) and --refresh-quota
// (out-of-band resync). After seeding, live calls increment usage via
// credentials.Pool.RecordAPICall.
package enphase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"enphase-monitor/internal/constants"
)

const statsPageURL = DefaultBaseURL + "/buyer/stats"

// statsWaitTimeout bounds login and per-app UI waits.
const statsWaitTimeout = 5 * time.Minute

// hitsTotalPattern matches the portal summary line, e.g. "206 Hits (hits)".
var hitsTotalPattern = regexp.MustCompile(`(?i)(\d[\d,]*)\s+Hits\s*\(\s*hits\s*\)`)

// MonthlyHitStat is one application's usage for the requested calendar month.
type MonthlyHitStat struct {
	Name      string
	Used      int
	Limit     int
	Remaining int
}

// FetchMonthlyHits logs into the portal via Chrome, reads each application's
// hit total for the calendar month containing ref, and returns the stats in
// name order. Names must match the portal dropdown labels (credentials.yaml
// `name` fields). notify receives phase status lines; progress, when non-nil,
// is called as each application is processed (same contract as ProgressFunc).
func FetchMonthlyHits(ctx context.Context, appNames []string, ref time.Time, notify func(string), progress ProgressFunc) ([]MonthlyHitStat, error) {
	if len(appNames) == 0 {
		return nil, errors.New("no application names to sync")
	}
	scraper := NewStatsScraper(ctx)
	defer scraper.Close()

	report(notify, "Opening Chrome — log in to the Enphase developer portal if prompted. Stats sync continues automatically once the stats page loads…")

	if err := scraper.openStatsPage(); err != nil {
		return nil, err
	}
	if err := scraper.waitForStatsReady(); err != nil {
		return nil, err
	}
	report(notify, "Signed in — reading monthly stats for each application…")

	from, until := calendarMonthRange(ref)
	stats := make([]MonthlyHitStat, 0, len(appNames))
	for i, name := range appNames {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if progress != nil {
			progress(i+1, len(appNames), name)
		}
		used, err := scraper.readAppMonthlyHits(name, from, until)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		stats = append(stats, MonthlyHitStat{
			Name:      name,
			Used:      used,
			Limit:     constants.MaxRequestsPerMonth,
			Remaining: remainingMonthly(used),
		})
	}
	return stats, nil
}

// parseHitsTotal extracts the total hit count from portal page text.
func parseHitsTotal(text string) (int, bool) {
	m := hitsTotalPattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	if err != nil {
		return 0, false
	}
	return n, true
}

// calendarMonthRange returns the portal date strings (MM/DD/YYYY) for the first
// day of ref's calendar month through ref itself (inclusive).
func calendarMonthRange(ref time.Time) (from, until string) {
	ref = ref.In(time.Local)
	start := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
	const portalDate = "01/02/2006"
	return start.Format(portalDate), ref.Format(portalDate)
}

func remainingMonthly(used int) int {
	rem := constants.MaxRequestsPerMonth - used
	if rem < 0 {
		return 0
	}
	return rem
}
