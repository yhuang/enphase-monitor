package pge

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClampToOneYear(t *testing.T) {
	d := func(s string) time.Time {
		v, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	tests := []struct {
		name        string
		from, to    string
		want        string
		wantClamped bool
	}{
		{"within a year is untouched", "2025-10-01", "2026-03-01", "2026-03-01", false},
		{"exactly one year inclusive is untouched", "2025-10-01", "2026-09-30", "2026-09-30", false},
		{"one day over is cut back", "2025-10-01", "2026-10-01", "2026-09-30", true},
		{"far beyond is cut back", "2025-10-01", "2026-10-13", "2026-09-30", true},
		{"leap day start", "2024-02-29", "2026-01-01", "2025-02-28", true},
		{"from january 1 ends december 31", "2025-01-01", "2026-06-01", "2025-12-31", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, clamped := clampToOneYear(d(tt.from), d(tt.to))
			if g := got.Format("2006-01-02"); g != tt.want || clamped != tt.wantClamped {
				t.Errorf("clampToOneYear(%s, %s) = %s, %v; want %s, %v", tt.from, tt.to, g, clamped, tt.want, tt.wantClamped)
			}
		})
	}
}

func TestArchiveRawDownload_KeepsBytesAndNeverOverwrites(t *testing.T) {
	loc := time.UTC
	src := filepath.Join(t.TempDir(), "dl.xml")
	if err := os.WriteFile(src, []byte("<feed>one</feed>"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "raw", "pge")
	from := time.Date(2025, 11, 8, 0, 0, 0, 0, loc)
	to := time.Date(2025, 11, 10, 0, 0, 0, 0, loc)

	first, err := archiveRawDownload(src, dir, from, to, loc)
	if err != nil {
		t.Fatal(err)
	}
	second, err := archiveRawDownload(src, dir, from, to, loc)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first) != "pge-2025-11-08_2025-11-10.xml" || filepath.Base(second) != "pge-2025-11-08_2025-11-10-1.xml" {
		t.Errorf("unexpected names %q, %q", first, second)
	}
	if got, _ := os.ReadFile(first); string(got) != "<feed>one</feed>" {
		t.Errorf("archived bytes = %q", got)
	}
}
