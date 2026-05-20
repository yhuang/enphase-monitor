package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestHandleClearCache_Success(t *testing.T) {
	// This tests the error path - clear cache on empty or existing cache should not error
	err := HandleClearCache()
	if err != nil {
		t.Errorf("HandleClearCache() returned error: %v", err)
	}
}

func TestHandleClearAllCache_Success(t *testing.T) {
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

	// Read output
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	output := buf.String()

	if !strings.Contains(output, "All cache cleared successfully") {
		t.Errorf("Expected success message, got: %s", output)
	}
}

func TestHandleClearCache_Idempotent(t *testing.T) {
	// Clear cache multiple times should not error
	for i := 0; i < 3; i++ {
		err := HandleClearCache()
		if err != nil {
			t.Errorf("Iteration %d: HandleClearCache() returned error: %v", i, err)
		}
	}
}

func TestHandleClearAllCache_Idempotent(t *testing.T) {
	// Capture stdout (suppress multiple success messages)
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Clear all cache multiple times should not error
	for i := 0; i < 3; i++ {
		err := HandleClearAllCache()
		if err != nil {
			t.Errorf("Iteration %d: HandleClearAllCache() returned error: %v", i, err)
		}
	}

	w.Close()
	os.Stdout = oldStdout

	// Drain pipe
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
}
