package rawstore

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// VerifyReport is the outcome of VerifySource.
type VerifyReport struct {
	Checked    int      // manifest lines whose file was hashed
	Mismatched []string // files whose SHA-256 differs from the manifest (altered or corrupted)
	Missing    []string // manifest lines whose file is gone
	Unlisted   []string // files in the source directory that no manifest line mentions
}

// OK reports whether every listed file is present and unaltered and no file is unlisted.
func (r VerifyReport) OK() bool {
	return len(r.Mismatched) == 0 && len(r.Missing) == 0 && len(r.Unlisted) == 0
}

// VerifySource re-hashes every file the manifest lists for source and compares it
// with the recorded SHA-256, and finds files in Dir/<source> the manifest does not
// list. It reads only local files. A source with no manifest lines and no files
// yields an empty (OK) report.
func VerifySource(source string) (VerifyReport, error) {
	var rep VerifyReport
	if Dir == "" {
		return rep, fmt.Errorf("raw archive is disabled (rawstore.Dir is empty)")
	}
	listed := map[string]bool{}
	f, err := os.Open(filepath.Join(Dir, "manifest.jsonl"))
	if err != nil && !os.IsNotExist(err) {
		return rep, err
	}
	if err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for n := 1; sc.Scan(); n++ {
			var e entry
			if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
				return rep, fmt.Errorf("manifest line %d is not valid JSON: %w", n, err)
			}
			if e.Source != source {
				continue
			}
			listed[e.File] = true
			data, err := os.ReadFile(filepath.Join(Dir, e.File))
			if err != nil {
				rep.Missing = append(rep.Missing, e.File)
				continue
			}
			rep.Checked++
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != e.SHA256 {
				rep.Mismatched = append(rep.Mismatched, e.File)
			}
		}
		if err := sc.Err(); err != nil {
			return rep, err
		}
	}

	entries, err := os.ReadDir(filepath.Join(Dir, source))
	if err != nil && !os.IsNotExist(err) {
		return rep, err
	}
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		if rel := filepath.Join(source, de.Name()); !listed[rel] {
			rep.Unlisted = append(rep.Unlisted, rel)
		}
	}
	sort.Strings(rep.Mismatched)
	sort.Strings(rep.Missing)
	sort.Strings(rep.Unlisted)
	return rep, nil
}

// Entry is one manifest line, exported for callers that inspect what a run added.
type Entry = entry

// ManifestLen returns the number of lines in the raw manifest (0 if none yet).
func ManifestLen() (int, error) {
	if Dir == "" {
		return 0, fmt.Errorf("raw archive is disabled (rawstore.Dir is empty)")
	}
	f, err := os.Open(filepath.Join(Dir, "manifest.jsonl"))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		n++
	}
	return n, sc.Err()
}

// EntriesSince returns the manifest lines after the first n (as counted by an
// earlier ManifestLen), i.e. what was archived since that point.
func EntriesSince(n int) ([]Entry, error) {
	if Dir == "" {
		return nil, fmt.Errorf("raw archive is disabled (rawstore.Dir is empty)")
	}
	f, err := os.Open(filepath.Join(Dir, "manifest.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for i := 0; sc.Scan(); i++ {
		if i < n {
			continue
		}
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("manifest line %d is not valid JSON: %w", i+1, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// VerifyEntry re-hashes the file e names and compares it with e's SHA-256 and size.
func VerifyEntry(e Entry) error {
	data, err := os.ReadFile(filepath.Join(Dir, e.File))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != e.SHA256 || len(data) != e.Bytes {
		return fmt.Errorf("%s: contents do not match the manifest hash/size", e.File)
	}
	return nil
}

// PrintReport writes the findings of a VerifySource report for source to out.
func PrintReport(out io.Writer, source string, r VerifyReport) {
	for _, f := range r.Mismatched {
		fmt.Fprintf(out, "✗ %s: hash differs from manifest (altered or corrupted)\n", f)
	}
	for _, f := range r.Missing {
		fmt.Fprintf(out, "✗ %s: listed in manifest but missing\n", f)
	}
	for _, f := range r.Unlisted {
		fmt.Fprintf(out, "? %s: not listed in manifest\n", f)
	}
	fmt.Fprintf(out, "%s manifest: %d file(s) hashed, %d altered, %d missing, %d unlisted\n",
		source, r.Checked, len(r.Mismatched), len(r.Missing), len(r.Unlisted))
}

// Entries returns every manifest line for source, in file order.
func Entries(source string) ([]Entry, error) {
	all, err := EntriesSince(0)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range all {
		if e.Source == source {
			out = append(out, e)
		}
	}
	return out, nil
}
