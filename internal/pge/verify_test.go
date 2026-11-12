package pge

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"enphase-monitor/internal/rawstore"
)

func TestVerifyPGERaw_CleanThenTampered(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("no tzdata")
	}
	root := t.TempDir()
	rawstore.Dir = filepath.Join(root, "raw")
	t.Cleanup(func() { rawstore.Dir = "" })
	archiveDir := filepath.Join(rawstore.Dir, "pge")
	hist := filepath.Join(root, "history")

	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("testdata/pge_real_2025-11-08.xml")
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(archiveDir, "pge-2025-11-08_2025-11-08.xml")
	if err := os.WriteFile(archive, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rawstore.Record("pge", archive, "test"); err != nil {
		t.Fatal(err)
	}

	// Before any history exists the day is reported as missing.
	var out bytes.Buffer
	res, err := VerifyPGERaw(archiveDir, hist, loc, &out)
	if err != nil || res.OK() || len(res.NoRecord) != 1 {
		t.Fatalf("no-history verify = %+v, %v", res, err)
	}

	// Build history from the archive, and it verifies clean.
	if _, _, _, err := ReprocessRaw(archive, hist, "", loc, func(string) {}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if res, err = VerifyPGERaw(archiveDir, hist, loc, &out); err != nil || !res.OK() || res.Identical != 1 {
		t.Fatalf("clean verify = %+v, %v\n%s", res, err, out.String())
	}

	// A hand-edited history record is caught.
	rec := filepath.Join(hist, "pge-2025-11-08.json")
	data, _ := os.ReadFile(rec)
	if err := os.WriteFile(rec, bytes.Replace(data, []byte(`"import_kwh": 29.724`), []byte(`"import_kwh": 99.9`), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if res, err = VerifyPGERaw(archiveDir, hist, loc, &out); err != nil || res.OK() || len(res.Differ) != 1 || !strings.Contains(out.String(), "99.900") {
		t.Fatalf("edited-record verify = %+v, %v\n%s", res, err, out.String())
	}

	// A tampered raw file is caught by the manifest hash.
	if err := os.WriteFile(archive, append(fixture, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if res, _ = VerifyPGERaw(archiveDir, hist, loc, &out); len(res.Manifest.Mismatched) != 1 {
		t.Errorf("tampered raw file not flagged: %+v", res.Manifest)
	}
}

func TestOrderArchives_ByFetchTimeNotName(t *testing.T) {
	// By name, "pge-2025-10-06_..." sorts before "pge-2025-11-08_..."; by fetch time
	// the short range was fetched first, so the full-year archive must come last.
	paths := []string{
		"raw/pge/pge-2025-10-06_2026-10-05.zip",
		"raw/pge/pge-2025-11-08_2025-11-10.zip",
		"raw/pge/pge-2025-10-01_2025-10-05.zip", // not in the manifest: sorts first
	}
	orderArchives(paths, map[string]string{
		"pge-2025-11-08_2025-11-10.zip": "2026-11-12T05:39:47Z",
		"pge-2025-10-06_2026-10-05.zip": "2026-11-12T06:38:00Z",
	})
	want := []string{
		"raw/pge/pge-2025-10-01_2025-10-05.zip",
		"raw/pge/pge-2025-11-08_2025-11-10.zip",
		"raw/pge/pge-2025-10-06_2026-10-05.zip",
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("order = %v, want %v", paths, want)
		}
	}
}
