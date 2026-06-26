// pull.go drives the two end-user PG&E flows: Authorize (one-time token
// bootstrap) and Pull (fetch a date range from the Share My Data API, parse the
// ESPI XML response, and write per-day history records).
package pge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// Authorize exchanges the one-time authorization_code from the SMD click-through
// for a User Access Token and persists it to the token store. Run this once
// before the first Pull (and again only if the refresh token lapses).
func (c *Config) Authorize(ctx context.Context, code, redirectURI string) error {
	if err := c.requireAuth(); err != nil {
		return err
	}
	if code == "" || redirectURI == "" {
		return fmt.Errorf("--pge-code and --pge-redirect-uri are required")
	}
	cl, err := newClient(c)
	if err != nil {
		return err
	}
	if err := cl.exchangeAuthCode(ctx, code, redirectURI); err != nil {
		return err
	}
	return nil
}

// PullResult reports the outcome of a Pull.
type PullResult struct {
	DaysWritten int
	Start       time.Time
	End         time.Time
}

// Pull fetches interval data for the inclusive day range [start, end] from the
// Share My Data API, parses the ESPI XML response with ParseReadings, aggregates
// to daily totals with AggregateReadingsByDay, and writes one pge-<date>.json
// record per day into c.HistoryDir. Zero start/end are filled with defaults
// (yesterday for end; 30 days before end for start). An empty response yields a
// PullResult with DaysWritten 0. loc is the property timezone used for day
// boundaries; pass nil to fall back to America/Los_Angeles.
func (c *Config) Pull(ctx context.Context, start, end time.Time, loc *time.Location) (*PullResult, error) {
	if err := c.requirePull(); err != nil {
		return nil, err
	}

	if loc == nil {
		if l, err := time.LoadLocation("America/Los_Angeles"); err == nil {
			loc = l
		} else {
			loc = time.UTC
		}
	}
	start, end = resolveRange(start, end, loc)

	// PG&E expects Zulu (UTC) RFC 3339 timestamps. published-max is exclusive of
	// the next day, so add 24h to include the whole end day.
	startZ := start.UTC().Format("2006-01-02T15:04:05Z")
	endZ := end.UTC().Add(24 * time.Hour).Format("2006-01-02T15:04:05Z")

	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	token, err := cl.userToken(ctx)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("%s/Subscription/%s/UsagePoint/%s?published-min=%s&published-max=%s",
		c.APIBase, c.SubscriptionID, c.UsagePointID, startZ, endZ)

	resp, err := cl.get(ctx, apiURL, token)
	if err != nil {
		return nil, fmt.Errorf("PG&E API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return &PullResult{Start: start, End: end}, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("PG&E API error %d: %s", resp.StatusCode, body)
	}

	readings, err := ParseReadings(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(readings) == 0 {
		return &PullResult{Start: start, End: end}, nil
	}
	sort.Slice(readings, func(i, j int) bool {
		return readings[i].Start.Before(readings[j].Start)
	})

	days := AggregateReadingsByDay(readings, loc)
	written, err := WriteHistory(c.HistoryDir, days, start, end, loc)
	if err != nil {
		return nil, fmt.Errorf("writing history: %w", err)
	}

	return &PullResult{DaysWritten: written, Start: start, End: end}, nil
}

// resolveRange fills zero start/end with defaults (end = yesterday, start = 30
// days before end) and truncates both to the start of their Pacific calendar day.
func resolveRange(start, end time.Time, loc *time.Location) (time.Time, time.Time) {
	if end.IsZero() {
		now := time.Now().In(loc)
		end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)
	}
	if start.IsZero() {
		start = end.AddDate(0, 0, -30)
	}
	return start, end
}
