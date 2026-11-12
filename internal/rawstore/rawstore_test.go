package rawstore

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveKeepsBytesAndManifest(t *testing.T) {
	Dir = filepath.Join(t.TempDir(), "raw")
	body := []byte(`{"a":1}`)
	p1, err := Save("enphase", "energy", ".json", body, "https://x/y?a=1")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := Save("enphase", "energy", ".json", body, "")
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatal("second save reused the first file name")
	}
	if got, _ := os.ReadFile(p1); string(got) != string(body) {
		t.Errorf("stored bytes = %q", got)
	}
	f, _ := os.Open(filepath.Join(Dir, "manifest.jsonl"))
	defer f.Close()
	n := 0
	for sc := bufio.NewScanner(f); sc.Scan(); n++ {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil || e.Source != "enphase" || len(e.SHA256) != 64 || e.Bytes != len(body) {
			t.Errorf("bad manifest line %q: %v", sc.Text(), err)
		}
	}
	if n != 2 {
		t.Errorf("manifest has %d lines, want 2", n)
	}
}

func TestRedactURL(t *testing.T) {
	got := RedactURL("https://api.enphaseenergy.com/a/b?key=SECRET&start_at=5")
	if strings.Contains(got, "SECRET") || !strings.Contains(got, "start_at=5") {
		t.Errorf("RedactURL = %q", got)
	}
}

func TestImportEnphaseCache_IdempotentAndByteExact(t *testing.T) {
	Dir = filepath.Join(t.TempDir(), "raw")
	cacheDir := t.TempDir()
	body := []byte(`{"system_id":1,"granularity":"day"}`)
	entry := map[string]any{
		"status_code": 200, "body": body, "cached_at": "2026-10-14T11:07:34.677653-07:00",
		"queried_date": "2025-10-04", "endpoint": "telemetry/production_meter", "system_id": "1",
	}
	for name, e := range map[string]any{
		"a.json": entry,
		"b.json": map[string]any{"month": "2026-10"}, // not an API response
	} {
		data, _ := json.Marshal(e)
		if err := os.WriteFile(filepath.Join(cacheDir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	res, err := ImportEnphaseCache(cacheDir)
	if err != nil || res.Imported != 1 || res.Skipped != 1 {
		t.Fatalf("first import = %+v, %v; want 1 imported, 1 skipped", res, err)
	}
	res, err = ImportEnphaseCache(cacheDir)
	if err != nil || res.Imported != 0 || res.Existing != 1 {
		t.Fatalf("second import = %+v, %v; want 0 imported, 1 existing", res, err)
	}
	files, _ := filepath.Glob(filepath.Join(Dir, "enphase", "1_telemetry_production_meter_2025-10-04_*.json"))
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	if got, _ := os.ReadFile(files[0]); string(got) != string(body) {
		t.Errorf("imported bytes = %q, want the original body", got)
	}
	m, _ := os.ReadFile(filepath.Join(Dir, "manifest.jsonl"))
	if n := strings.Count(string(m), "\n"); n != 1 || !strings.Contains(string(m), "imported from cache") || !strings.Contains(string(m), `"fetched_at":"2026-10-14T18:07:34Z"`) {
		t.Errorf("manifest = %q", m)
	}
}

func TestVerifySource(t *testing.T) {
	Dir = filepath.Join(t.TempDir(), "raw")
	good, err := Save("pge", "good", ".xml", []byte("good"), "")
	if err != nil {
		t.Fatal(err)
	}
	bad, _ := Save("pge", "bad", ".xml", []byte("original"), "")
	gone, _ := Save("pge", "gone", ".xml", []byte("x"), "")
	if _, err := Save("weather", "other", ".json", []byte("{}"), ""); err != nil {
		t.Fatal(err)
	}
	if rep, err := VerifySource("pge"); err != nil || !rep.OK() || rep.Checked != 3 {
		t.Fatalf("clean verify = %+v, %v", rep, err)
	}

	os.WriteFile(bad, []byte("tampered"), 0o600)
	os.Remove(gone)
	os.WriteFile(filepath.Join(Dir, "pge", "stray.xml"), []byte("?"), 0o600)
	rep, err := VerifySource("pge")
	if err != nil || rep.OK() {
		t.Fatalf("verify = %+v, %v; want problems", rep, err)
	}
	if len(rep.Mismatched) != 1 || len(rep.Missing) != 1 || len(rep.Unlisted) != 1 || rep.Checked != 2 {
		t.Errorf("report = %+v, want 1 mismatched, 1 missing, 1 unlisted, 2 checked", rep)
	}
	_ = good
}

func TestEntriesSinceAndVerifyEntry(t *testing.T) {
	Dir = filepath.Join(t.TempDir(), "raw")
	if _, err := Save("enphase", "a", ".json", []byte("{}"), ""); err != nil {
		t.Fatal(err)
	}
	n, err := ManifestLen()
	if err != nil || n != 1 {
		t.Fatalf("ManifestLen = %d, %v", n, err)
	}
	p, _ := Save("enphase", "b", ".json", []byte(`{"x":1}`), "req")
	got, err := EntriesSince(n)
	if err != nil || len(got) != 1 || got[0].Request != "req" {
		t.Fatalf("EntriesSince = %+v, %v", got, err)
	}
	if err := VerifyEntry(got[0]); err != nil {
		t.Errorf("intact entry: %v", err)
	}
	os.WriteFile(p, []byte(`{"x":2}`), 0o600)
	if err := VerifyEntry(got[0]); err == nil {
		t.Error("altered file should fail VerifyEntry")
	}
}
