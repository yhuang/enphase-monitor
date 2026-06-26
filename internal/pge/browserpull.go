// browserpull.go is the public entry point for the browser-based PG&E pull: it
// opens a (persistent-profile) Chrome session for the user to sign in,
// auto-drives the Green Button XML export for a date range, then parses the
// captured ESPI XML and writes one history record per day.
//
// This is an alternative to the Share My Data API path (pull.go): it needs no
// client certificate or OAuth registration — just an interactive sign-in — at
// the cost of driving a real browser. The export itself is manual-assisted
// because PG&E's Salesforce Lightning UI renders inside closed shadow roots that
// can't be driven by selector (see download.go); the download is still captured
// automatically. The raw XML is deleted from tmp/ after history records are
// written successfully.
package pge

import (
	"context"
	"fmt"
	"os"
	"time"
)

// dateFormat is the ISO 8601 calendar-day layout used throughout this package.
const dateFormat = "2006-01-02"

// BrowserPullOptions configures a Green Button browser pull.
type BrowserPullOptions struct {
	ProfileDir string         // persistent Chrome profile dir (sign in once, reuse later); required
	HistoryDir string         // directory where pge-YYYY-MM-DD.json day records are written; required
	RawDir     string         // directory the downloaded XML is kept in; required
	From, To   time.Time      // inclusive date range; zero values are filled with defaults
	TZ         *time.Location // report timezone; defaults to Pacific
	Notify     func(string)   // optional status-line sink for the user
}

// BrowserPullResult reports the outcome of a BrowserPull.
type BrowserPullResult struct {
	DaysWritten int       // number of pge-YYYY-MM-DD.json records written to HistoryDir
	LastDay     time.Time // last calendar day that had actual PG&E data within the range; zero if none
	From, To    time.Time // the resolved range
}

// BrowserPull runs the full browser pull: sign-in (interactive, once per profile
// lifetime), a user-driven Green Button XML export, parse, and per-day history
// write.
func BrowserPull(ctx context.Context, opts BrowserPullOptions) (*BrowserPullResult, error) {
	if opts.ProfileDir == "" || opts.HistoryDir == "" || opts.RawDir == "" {
		return nil, fmt.Errorf("ProfileDir, HistoryDir, and RawDir are required")
	}

	loc := opts.TZ
	if loc == nil {
		if l, err := time.LoadLocation("America/Los_Angeles"); err == nil {
			loc = l
		} else {
			loc = time.UTC
		}
	}
	from, to := resolveRange(opts.From, opts.To, loc)
	if to.Before(from) {
		return nil, fmt.Errorf("PG&E pull range is empty: start %s is after end %s",
			from.Format(dateFormat), to.Format(dateFormat))
	}

	bctx, cancel, err := openLoggedInSession(ctx, opts.ProfileDir, greenButtonURL, readyURLMarker, opts.Notify)
	if err != nil {
		return nil, err
	}
	defer cancel()

	downloaded, err := downloadGreenButton(bctx, from, to, opts.RawDir, opts.Notify)
	if err != nil {
		return nil, err
	}

	xmlPath, err := ExtractElectricXML(downloaded)
	if err != nil {
		return nil, fmt.Errorf("extracting electric XML from %q: %w", downloaded, err)
	}

	readings, err := ParseXMLDownload(xmlPath)
	if err != nil {
		return nil, fmt.Errorf("parsing %q: %w", xmlPath, err)
	}
	days := AggregateReadingsByDay(readings, loc) // sorted ascending by Date

	written, err := WriteHistory(opts.HistoryDir, days, from, to, loc)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(xmlPath)

	// Find the last day that had actual data within [from, to]. days is sorted
	// ascending, so we walk backwards to the first match.
	fromDate := from.In(loc).Format(dateFormat)
	toDate := to.In(loc).Format(dateFormat)
	var lastDay time.Time
	for i := len(days) - 1; i >= 0; i-- {
		if days[i].Date >= fromDate && days[i].Date <= toDate {
			t, err := time.ParseInLocation(dateFormat, days[i].Date, loc)
			if err != nil {
				return nil, fmt.Errorf("internal: unparseable date %q from AggregateReadingsByDay: %w", days[i].Date, err)
			}
			lastDay = t
			break
		}
	}

	return &BrowserPullResult{DaysWritten: written, LastDay: lastDay, From: from, To: to}, nil
}
