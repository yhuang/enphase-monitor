package rawstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImportResult counts what ImportEnphaseCache did.
type ImportResult struct {
	Imported int // new files written to raw/enphase
	Existing int // already imported on an earlier run
	Skipped  int // cache files that are not Enphase API responses, or not status 200
}

// cacheFile is the on-disk shape of an Enphase API cache entry (internal/cache).
type cacheFile struct {
	StatusCode  int       `json:"status_code"`
	Body        []byte    `json:"body"` // base64 in JSON: the response exactly as received
	CachedAt    time.Time `json:"cached_at"`
	QueriedDate string    `json:"queried_date"`
	Endpoint    string    `json:"endpoint"`
	SystemID    string    `json:"system_id"`
}

// ImportEnphaseCache copies the raw Enphase response bodies held in cacheDir into
// Dir/enphase, using each entry's cached_at as the fetch time and marking the
// manifest line "imported from cache". It is idempotent: file names are derived
// from the entry (system, endpoint, date, fetch time), so a second run finds them
// and writes nothing. It makes no network calls.
func ImportEnphaseCache(cacheDir string) (ImportResult, error) {
	var res ImportResult
	if Dir == "" {
		return res, fmt.Errorf("raw archive is disabled (rawstore.Dir is empty)")
	}
	files, err := filepath.Glob(filepath.Join(cacheDir, "*.json"))
	if err != nil {
		return res, err
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return res, err
		}
		var c cacheFile
		if json.Unmarshal(data, &c) != nil || c.Endpoint == "" || c.StatusCode != 200 || len(c.Body) == 0 || c.CachedAt.IsZero() {
			res.Skipped++
			continue
		}
		fetched := c.CachedAt.UTC()
		name := fmt.Sprintf("%s_%s_%s", c.SystemID, strings.ReplaceAll(c.Endpoint, "/", "_"), c.QueriedDate)
		base := fmt.Sprintf("%s_%s", name, fetched.Format("20060102T150405.000000Z"))
		if _, err := os.Stat(filepath.Join(Dir, "enphase", base+".json")); err == nil {
			res.Existing++
			continue
		}
		req := fmt.Sprintf("system=%s endpoint=%s date=%s", c.SystemID, c.Endpoint, c.QueriedDate)
		if _, err := saveExact("enphase", base, ".json", c.Body, req, "imported from cache", fetched); err != nil {
			return res, err
		}
		res.Imported++
	}
	return res, nil
}
