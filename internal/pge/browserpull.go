// browserpull.go is the public entry point for the browser-based PG&E pull: it
// opens a (persistent-profile) Chrome session for the user to sign in,
// auto-drives the Green Button XML export for a date range, then parses the
// captured ESPI XML and writes one history record per day.
//
// It needs no client certificate or OAuth registration — just an interactive
// sign-in — at the cost of driving a real browser. The export itself is manual-assisted
// because PG&E's Salesforce Lightning UI renders inside closed shadow roots that
// can't be driven by selector (see download.go); the download is still captured
// automatically. The untouched download (ZIP or XML) is archived under raw/pge/
// before extraction so history records can be re-verified against the source;
// only the scratch copies in tmp/ are deleted.
//
// PG&E's export form accepts at most one year per download, so a requested range
// is cut back to one year after its start (clampToOneYear); the caller is told the
// start date to continue from.
package pge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"enphase-monitor/internal/constants"
	"enphase-monitor/internal/rawstore"
)

// dateFormat is the ISO 8601 calendar-day layout used throughout this package.
const dateFormat = constants.DateFormat

// BrowserPullOptions configures a Green Button browser pull.
type BrowserPullOptions struct {
	ProfileDir string         // persistent Chrome profile dir (sign in once, reuse later); required
	HistoryDir string         // directory where pge-YYYY-MM-DD.json day records are written; required
	RawDir     string         // scratch directory the browser downloads into; required
	CompareDir string         // history directory holding enphase-*.json for cross-checks; empty skips the Enphase comparison
	ArchiveDir string         // directory where the untouched download is kept for integrity checks; empty disables archiving
	From, To   time.Time      // inclusive date range; zero values are filled with defaults
	TZ         *time.Location // report timezone; defaults to Pacific
	Notify     func(string)   // optional status-line sink for the user
}

// BrowserPullResult reports the outcome of a BrowserPull.
type BrowserPullResult struct {
	DaysWritten int       // number of pge-YYYY-MM-DD.json records written to HistoryDir
	LastDay     time.Time // last calendar day that had actual PG&E data within the range; zero if none
	From, To    time.Time // the resolved range (To is the clamped end when Clamped is true)
	Clamped     bool      // true when the requested end was cut back to fit PG&E's one-year export limit
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

	// PG&E's export form rejects ranges over one year, so cap the end at one year
	// after the start (inclusive) and tell the user how to continue.
	to, clamped := clampToOneYear(from, to)
	if clamped {
		next := to.AddDate(0, 0, 1).Format(dateFormat)
		report(opts.Notify, fmt.Sprintf(
			"PG&E exports are limited to one year: pulling %s to %s. Run again with --start-date %s to continue.",
			from.Format(dateFormat), to.Format(dateFormat), next))
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

	// Archive the download byte-for-byte before extraction deletes the ZIP, so
	// the parsed history can always be re-checked against what PG&E sent.
	if opts.ArchiveDir != "" {
		archived, err := archiveRawDownload(downloaded, opts.ArchiveDir, from, to, loc)
		if err != nil {
			return nil, err
		}
		if err := rawstore.Record("pge", archived, fmt.Sprintf("green button export %s..%s", from.In(loc).Format(dateFormat), to.In(loc).Format(dateFormat))); err != nil {
			report(opts.Notify, fmt.Sprintf("WARNING: could not add %s to the raw manifest: %v", archived, err))
		}
		report(opts.Notify, fmt.Sprintf("Raw PG&E download kept at %s", archived))
	}

	days, written, err := ingestDownload(downloaded, opts.HistoryDir, opts.CompareDir, from, to, loc, opts.Notify)
	if err != nil {
		return nil, err
	}

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

	return &BrowserPullResult{DaysWritten: written, LastDay: lastDay, From: from, To: to, Clamped: clamped}, nil
}

// ingestDownload turns a Green Button download (ZIP or XML) into history
// records for [from, to]: extract, parse, aggregate, sanity-check (warnings go to
// notify), write. It consumes path: the ZIP and extracted XML are deleted, so
// callers must archive or copy first. It returns the aggregated days (sorted
// ascending) and the number of records written.
func ingestDownload(path, historyDir, compareDir string, from, to time.Time, loc *time.Location, notify func(string)) ([]DayUsage, int, error) {
	xmlPath, err := ExtractElectricXML(path)
	if err != nil {
		return nil, 0, fmt.Errorf("extracting electric XML from %q: %w", path, err)
	}
	defer os.Remove(xmlPath)

	readings, err := ParseXMLDownload(xmlPath)
	if err != nil {
		return nil, 0, fmt.Errorf("parsing %q: %w", xmlPath, err)
	}
	days, err := AggregateReadingsByDay(readings, loc)
	if err != nil {
		return nil, 0, fmt.Errorf("aggregating %q: %w", xmlPath, err)
	}
	for _, w := range CheckDays(days, compareDir, from, to, loc) {
		report(notify, "WARNING: "+w)
	}
	written, err := WriteHistory(historyDir, days, from, to, loc)
	if err != nil {
		return nil, 0, err
	}
	return days, written, nil
}

// archiveNameRE matches the names archiveRawDownload produces.
var archiveNameRE = regexp.MustCompile(`^pge-(\d{4}-\d{2}-\d{2})_(\d{4}-\d{2}-\d{2})(?:-\d+)?\.(?:zip|xml)$`)

// ReprocessRaw rebuilds history records from a previously archived raw download
// (raw/pge/pge-<from>_<to>.zip) without touching the network, so a parser fix can
// be applied to data already collected. The date range comes from the file name.
// The archive itself is never modified: it is copied to a scratch directory first.
func ReprocessRaw(archivePath, historyDir, compareDir string, loc *time.Location, notify func(string)) (days int, from, to time.Time, err error) {
	m := archiveNameRE.FindStringSubmatch(filepath.Base(archivePath))
	if m == nil {
		return 0, from, to, fmt.Errorf("%q is not named pge-<from>_<to>.zip|xml, so its date range is unknown", filepath.Base(archivePath))
	}
	if from, err = time.ParseInLocation(dateFormat, m[1], loc); err != nil {
		return 0, from, to, err
	}
	if to, err = time.ParseInLocation(dateFormat, m[2], loc); err != nil {
		return 0, from, to, err
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return 0, from, to, err
	}
	scratch, err := os.MkdirTemp("", "pge-reprocess-")
	if err != nil {
		return 0, from, to, err
	}
	defer os.RemoveAll(scratch)
	copyPath := filepath.Join(scratch, filepath.Base(archivePath))
	if err := os.WriteFile(copyPath, data, 0o600); err != nil {
		return 0, from, to, err
	}
	_, written, err := ingestDownload(copyPath, historyDir, compareDir, from, to, loc, notify)
	return written, from, to, err
}

// archiveRawDownload copies the file at src into dir as
// pge-<from>_<to>.<ext> (ext is "zip" or "xml", detected from content) and
// returns the new path. An existing archive for the same range is kept and the
// new copy gets a numeric suffix, so a re-pull never overwrites earlier raw data.
func archiveRawDownload(src, dir string, from, to time.Time, loc *time.Location) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("reading download %q for archiving: %w", src, err)
	}
	ext := "xml"
	if isZipXML(data) {
		ext = "zip"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating raw archive dir %q: %w", dir, err)
	}
	base := fmt.Sprintf("pge-%s_%s", from.In(loc).Format(dateFormat), to.In(loc).Format(dateFormat))
	for n := 0; ; n++ {
		name := base + "." + ext
		if n > 0 {
			name = fmt.Sprintf("%s-%d.%s", base, n, ext)
		}
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("creating raw archive %q: %w", name, err)
		}
		_, werr := f.Write(data)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return "", fmt.Errorf("writing raw archive %q: %w", name, werr)
		}
		return filepath.Join(dir, name), nil
	}
}

// clampToOneYear caps to at one year after from, inclusive (2025-10-01 → 2026-09-30).
// PG&E's Green Button form rejects longer ranges ("Please select a date range
// that does not exceed one year"), so the pull is cut back instead of failing.
// It reports whether the end was changed.
func clampToOneYear(from, to time.Time) (time.Time, bool) {
	limit := from.AddDate(1, 0, -1)
	if to.After(limit) {
		return limit, true
	}
	return to, false
}

// resolveRange fills zero start/end with defaults (end = yesterday, start = 30
// days before end) in loc.
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

// report sends msg to notify when it is non-nil.
func report(notify func(string), msg string) {
	if notify != nil {
		notify(msg)
	}
}
