package pge

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestFile creates a file with the given content in dir. It is a test
// helper that marks t.Fatal on failure so callers don't need error checks.
func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeTestFile %s: %v", name, err)
	}
}

func TestSnapshotFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	snap, err := snapshotFiles(dir)
	if err != nil {
		t.Fatalf("snapshotFiles: %v", err)
	}
	if len(snap) != 0 {
		t.Errorf("want 0 entries, got %d", len(snap))
	}
}

func TestSnapshotFiles_CapturesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.xml", "data")
	writeTestFile(t, dir, "b.xml", "data")

	snap, err := snapshotFiles(dir)
	if err != nil {
		t.Fatalf("snapshotFiles: %v", err)
	}
	if len(snap) != 2 {
		t.Errorf("want 2 entries, got %d", len(snap))
	}
	for _, name := range []string{"a.xml", "b.xml"} {
		if _, ok := snap[name]; !ok {
			t.Errorf("snapshot missing %q", name)
		}
	}
}

func TestSnapshotFiles_IgnoresSubdirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "file.xml", "data")

	snap, err := snapshotFiles(dir)
	if err != nil {
		t.Fatalf("snapshotFiles: %v", err)
	}
	if len(snap) != 1 {
		t.Errorf("want 1 entry (subdir excluded), got %d", len(snap))
	}
}

func TestNewCompletedFile_NoNewFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "existing.xml", "data")
	snap, _ := snapshotFiles(dir)

	if name := newCompletedFile(dir, snap); name != "" {
		t.Errorf("want empty (no new file), got %q", name)
	}
}

func TestNewCompletedFile_DetectsNewFile(t *testing.T) {
	dir := t.TempDir()
	snap, _ := snapshotFiles(dir)

	writeTestFile(t, dir, "pge_export.xml", "content")

	if name := newCompletedFile(dir, snap); name != "pge_export.xml" {
		t.Errorf("want pge_export.xml, got %q", name)
	}
}

func TestNewCompletedFile_IgnoresCrdownload(t *testing.T) {
	dir := t.TempDir()
	snap, _ := snapshotFiles(dir)

	writeTestFile(t, dir, "pge_export.xml.crdownload", "partial")

	if name := newCompletedFile(dir, snap); name != "" {
		t.Errorf("should ignore .crdownload partial, got %q", name)
	}
}

func TestNewCompletedFile_DetectsOverwrite(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "pge_export.xml", "old content")
	snap, _ := snapshotFiles(dir)

	// Ensure the mtime will differ.
	time.Sleep(10 * time.Millisecond)
	writeTestFile(t, dir, "pge_export.xml", "new content")

	if name := newCompletedFile(dir, snap); name != "pge_export.xml" {
		t.Errorf("want pge_export.xml (same-name overwrite detected), got %q", name)
	}
}

func TestNewCompletedFile_MultipleNewFiles(t *testing.T) {
	dir := t.TempDir()
	snap, _ := snapshotFiles(dir)

	writeTestFile(t, dir, "a.xml", "data")
	writeTestFile(t, dir, "b.xml", "data")

	name := newCompletedFile(dir, snap)
	if name != "a.xml" && name != "b.xml" {
		t.Errorf("want one of [a.xml b.xml], got %q", name)
	}
}
