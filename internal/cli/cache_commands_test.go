package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// useTempCacheDir redirects cache I/O to a temp directory for the test.
func useTempCacheDir(t *testing.T) {
	t.Helper()
	t.Setenv("ENPHASE_CACHE_DIR", t.TempDir())
}

func TestHandleClearCache_Success(t *testing.T) {
	useTempCacheDir(t)
	if err := HandleClearCache(); err != nil {
		t.Errorf("HandleClearCache() returned error: %v", err)
	}
}

func TestHandleClearAllCache_Success(t *testing.T) {
	useTempCacheDir(t)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := HandleClearAllCache()

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("HandleClearAllCache() returned error: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	if !strings.Contains(buf.String(), "All cache cleared successfully") {
		t.Errorf("Expected success message, got: %s", buf.String())
	}
}

func TestHandleClearCacheForDate_ValidPastDate(t *testing.T) {
	useTempCacheDir(t)
	if err := HandleClearCacheForDate("2026-01-19"); err != nil {
		t.Errorf("HandleClearCacheForDate() returned error: %v", err)
	}
}

func TestHandleClearCacheForDate_InvalidFormat(t *testing.T) {
	useTempCacheDir(t)
	for _, bad := range []string{"2026-1-9", "01-19-2026", "not-a-date", ""} {
		if err := HandleClearCacheForDate(bad); err == nil {
			t.Errorf("HandleClearCacheForDate(%q) = nil, want error", bad)
		}
	}
}

func TestHandleClearCacheForDate_FutureDate(t *testing.T) {
	useTempCacheDir(t)
	if err := HandleClearCacheForDate("2099-12-31"); err == nil {
		t.Error("HandleClearCacheForDate() for future date = nil, want error")
	}
}

func TestHandleClearCache_Idempotent(t *testing.T) {
	useTempCacheDir(t)
	for i := 0; i < 3; i++ {
		if err := HandleClearCache(); err != nil {
			t.Errorf("Iteration %d: HandleClearCache() returned error: %v", i, err)
		}
	}
}

func TestHandleClearAllCache_Idempotent(t *testing.T) {
	useTempCacheDir(t)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	for i := 0; i < 3; i++ {
		if err := HandleClearAllCache(); err != nil {
			t.Errorf("Iteration %d: HandleClearAllCache() returned error: %v", i, err)
		}
	}

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
}
