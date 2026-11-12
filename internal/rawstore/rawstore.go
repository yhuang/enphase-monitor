// Package rawstore keeps an untouched copy of every external response the program
// ingests (Enphase API, Open-Meteo, PG&E Green Button) under raw/<source>/, with
// one line per file in raw/manifest.jsonl (time, source, file, SHA-256, size,
// request). The processed history/ records can then be re-derived and audited
// against exactly what each source sent, at any time.
//
// Files are written once and never overwritten: a repeat fetch gets its own
// timestamped name. The directory holds account data and is gitignored.
// Archiving is best-effort for callers that already hold the data (a failed write
// must not lose a response) but every failure is returned so it can be reported.
package rawstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Dir is the archive root. Empty (the default) disables archiving, so tests and
// library use never write files; main sets it at startup.
var Dir string

var mu sync.Mutex // serialises manifest appends from concurrent fetches

// entry is one manifest line.
type entry struct {
	FetchedAt string `json:"fetched_at"` // RFC3339, UTC
	Source    string `json:"source"`
	File      string `json:"file"` // path relative to Dir
	SHA256    string `json:"sha256"`
	Bytes     int    `json:"bytes"`
	Request   string `json:"request,omitempty"` // what was asked for, secrets removed
	Note      string `json:"note,omitempty"`    // provenance, e.g. "imported from cache"
}

// Save writes body to Dir/<source>/<name><ext> (name gets a UTC timestamp suffix
// so nothing is overwritten) and appends a manifest line. request describes the
// call; pass it through RedactURL if it is a URL. It returns the file path, and does
// nothing (returning "", nil) when Dir is empty.
func Save(source, name, ext string, body []byte, request string) (string, error) {
	if Dir == "" {
		return "", nil
	}
	return saveAt(source, name, ext, body, request, "", time.Now().UTC())
}

// saveAt is Save with an explicit fetch time and provenance note.
func saveAt(source, name, ext string, body []byte, request, note string, now time.Time) (string, error) {
	return saveExact(source, fmt.Sprintf("%s_%s", name, now.Format("20060102T150405.000000Z")), ext, body, request, note, now)
}

// saveExact writes Dir/<source>/<base><ext> without overwriting and appends a
// manifest line stamped with fetchedAt.
func saveExact(source, base, ext string, body []byte, request, note string, now time.Time) (string, error) {
	sum := sha256.Sum256(body)
	dir := filepath.Join(Dir, source)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating raw dir %q: %w", dir, err)
	}
	path := filepath.Join(dir, base+ext)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("creating raw file %q: %w", path, err)
	}
	_, werr := f.Write(body)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return "", fmt.Errorf("writing raw file %q: %w", path, werr)
	}
	return path, appendManifest(entry{
		FetchedAt: now.Format(time.RFC3339),
		Source:    source,
		File:      filepath.Join(source, base+ext),
		SHA256:    hex.EncodeToString(sum[:]),
		Bytes:     len(body),
		Request:   request,
		Note:      note,
	})
}

// Record adds a manifest line for a file already placed under Dir (used for the
// PG&E download, which is archived as received). path must be inside Dir.
func Record(source, path, request string) error {
	if Dir == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(Dir, path)
	if err != nil {
		rel = path
	}
	sum := sha256.Sum256(data)
	return appendManifest(entry{
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    source,
		File:      rel,
		SHA256:    hex.EncodeToString(sum[:]),
		Bytes:     len(data),
		Request:   request,
	})
}

func appendManifest(e entry) error {
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(filepath.Join(Dir, "manifest.jsonl"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("opening raw manifest: %w", err)
	}
	_, werr := f.Write(append(line, '\n'))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

// RedactURL returns raw with credential query parameters (key, access_token,
// client_secret) removed so it is safe to write to the manifest.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparseable url)"
	}
	q := u.Query()
	for _, k := range []string{"key", "access_token", "client_secret"} {
		q.Del(k)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
