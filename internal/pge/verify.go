// verify.go re-derives PG&E history records from the archived raw downloads and
// diffs them against history/, so processing drift (a parser fix not yet applied,
// a hand-edited record, a stale pull) shows up locally without any network access.
package pge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"enphase-monitor/internal/history"
	"enphase-monitor/internal/rawstore"
)

// VerifyResult summarises VerifyPGERaw.
type VerifyResult struct {
	Archives  int      // raw downloads re-derived
	Days      int      // day records compared
	Identical int      // days whose re-derived record is byte-identical to history
	Differ    []string // dates whose history record differs from the re-derived one
	NoRecord  []string // dates re-derived but absent from history
	Manifest  rawstore.VerifyReport
}

// OK reports whether the manifest is clean and every day matches.
func (r VerifyResult) OK() bool {
	return r.Manifest.OK() && len(r.Differ) == 0 && len(r.NoRecord) == 0
}

// VerifyPGERaw checks Dir/pge against the raw manifest (hashes, missing, unlisted
// files), then re-derives every archived download into a scratch directory with
// the current parser and compares each day with historyDir. Archives are read
// only (ReprocessRaw works on a copy). When archives overlap, a day is judged
// against the most recently fetched archive covering it (by manifest fetch time),
// which is what the last pull would have written. Progress and findings go to out.
func VerifyPGERaw(archiveDir, historyDir string, loc *time.Location, out io.Writer) (VerifyResult, error) {
	var res VerifyResult
	var err error
	if res.Manifest, err = rawstore.VerifySource("pge"); err != nil {
		return res, err
	}
	rawstore.PrintReport(out, "pge", res.Manifest)

	archives, err := filepath.Glob(filepath.Join(archiveDir, "pge-*"))
	if err != nil {
		return res, err
	}
	entries, err := rawstore.Entries("pge")
	if err != nil {
		return res, err
	}
	fetched := make(map[string]string, len(entries))
	for _, e := range entries {
		fetched[filepath.Base(e.File)] = e.FetchedAt
	}
	orderArchives(archives, fetched)

	scratch, err := os.MkdirTemp("", "pge-verify-")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(scratch)

	for _, a := range archives {
		if !archiveNameRE.MatchString(filepath.Base(a)) {
			continue
		}
		if _, _, _, err := ReprocessRaw(a, scratch, "", loc, func(string) {}); err != nil {
			return res, fmt.Errorf("re-deriving %s: %w", filepath.Base(a), err)
		}
		res.Archives++
	}

	days, err := filepath.Glob(history.PGE.RecordPath(scratch, "*"))
	if err != nil {
		return res, err
	}
	sort.Strings(days)
	for _, p := range days {
		name := filepath.Base(p)
		want, err := os.ReadFile(p)
		if err != nil {
			return res, err
		}
		date := name[len("pge-") : len(name)-len(".json")]
		got, err := os.ReadFile(filepath.Join(historyDir, name))
		res.Days++
		switch {
		case os.IsNotExist(err):
			res.NoRecord = append(res.NoRecord, date)
			fmt.Fprintf(out, "✗ %s: re-derived from raw but missing in %s\n", date, historyDir)
		case err != nil:
			return res, err
		case !bytes.Equal(got, want):
			res.Differ = append(res.Differ, date)
			fmt.Fprintf(out, "≠ %s: history record differs from the one re-derived from raw%s\n", date, summaryDiff(got, want))
		default:
			res.Identical++
		}
	}
	return res, nil
}

// summaryDiff renders the daily totals of the stored and re-derived records for a
// quick eyeball of how far apart they are; "" if either cannot be read.
func summaryDiff(stored, derived []byte) string {
	s, d := dayTotals(stored), dayTotals(derived)
	if s == nil || d == nil {
		return ""
	}
	return fmt.Sprintf(" (import %.3f vs %.3f kWh, export %.3f vs %.3f kWh)", s[0], d[0], s[1], d[1])
}

// dayTotals returns {import, export} kWh from a pge day record, or nil.
func dayTotals(data []byte) []float64 {
	var r struct {
		Import float64 `json:"import_kwh"`
		Export float64 `json:"export_kwh"`
	}
	if json.Unmarshal(data, &r) != nil {
		return nil
	}
	return []float64{r.Import, r.Export}
}

// orderArchives sorts archive paths oldest-fetch first, so that when ranges
// overlap the later-fetched archive overwrites the earlier one in the scratch
// directory, as the later pull overwrote history. fetchedAt maps an archive's base
// name to its manifest fetch time (UTC RFC 3339, so it sorts as text); archives
// without a manifest line sort first, and ties fall back to the file name.
func orderArchives(paths []string, fetchedAt map[string]string) {
	sort.SliceStable(paths, func(i, j int) bool {
		fi, fj := fetchedAt[filepath.Base(paths[i])], fetchedAt[filepath.Base(paths[j])]
		if fi != fj {
			return fi < fj
		}
		return paths[i] < paths[j]
	})
}
