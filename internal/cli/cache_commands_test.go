package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
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
	io.Copy(&buf, r)
	output := buf.String()
	
	if !strings.Contains(output, "All cache cleared successfully") {
		t.Errorf("Expected success message, got: %s", output)
	}
}

func TestHandleListCache_Execution(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	err := HandleListCache()
	
	w.Close()
	os.Stdout = oldStdout
	
	if err != nil {
		t.Errorf("HandleListCache() returned error: %v", err)
	}
	
	// Read output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()
	
	// Should either show entries or "no cached responses found"
	hasOutput := strings.Contains(output, "cached responses") || 
		strings.Contains(output, "inspect-cache") ||
		strings.Contains(output, "Hash:")
	
	if !hasOutput && output != "" {
		t.Errorf("Unexpected output format: %s", output)
	}
}

func TestHandleInspectCache_ByDate_NotFound(t *testing.T) {
	// Use a future date that definitely won't have cache
	testDate := "2099-12-31"
	
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	err := HandleInspectCache(testDate, "config.yaml")
	
	w.Close()
	os.Stdout = oldStdout
	
	if err != nil {
		t.Errorf("HandleInspectCache() should not error on missing date: %v", err)
	}
	
	// Read output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()
	
	// Should show "No cached responses found" or show available dates
	if !strings.Contains(output, "No cached responses") && 
	   !strings.Contains(output, "Available dates") &&
	   output != "" {
		t.Errorf("Expected appropriate message, got: %s", output)
	}
}

func TestHandleInspectCache_ByDate_WithConfig(t *testing.T) {
	// Use a past date that might have cache (but works even if empty)
	testDate := "2026-01-15"
	
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	err := HandleInspectCache(testDate, "config.yaml")
	
	w.Close()
	os.Stdout = oldStdout
	
	// Should not error even with missing config
	if err != nil {
		t.Errorf("HandleInspectCache() returned error: %v", err)
	}
	
	// Read output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	// Output is context-dependent (may have cache or not), so just verify no crash
}

func TestHandleInspectCache_InvalidHash(t *testing.T) {
	// Try to inspect non-existent hash
	err := HandleInspectCache("nonexistent_hash_12345_definitely_not_real", "config.yaml")
	
	if err == nil {
		t.Error("Expected error for non-existent hash, got nil")
	}
	
	if !strings.Contains(err.Error(), "failed to inspect cache entry") {
		t.Errorf("Expected 'failed to inspect cache entry' error, got: %v", err)
	}
}

func TestHandleInspectCache_WithInvalidConfig(t *testing.T) {
	// Test that invalid config doesn't crash - uses default timezone
	testDate := "2026-01-15"
	
	// Capture stdout (suppress output)
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	// Should not panic with invalid config
	err := HandleInspectCache(testDate, "nonexistent_config_file_xyz.yaml")
	
	w.Close()
	os.Stdout = oldStdout
	
	// Drain pipe
	var buf bytes.Buffer
	io.Copy(&buf, r)
	
	// Should not error - just use default timezone
	if err != nil {
		t.Errorf("HandleInspectCache() should work with invalid config: %v", err)
	}
}

func TestHandleInspectCache_DateParsing(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
		desc      string
	}{
		{
			name:      "valid date format",
			input:     "2026-01-15",
			expectErr: false,
			desc:      "YYYY-MM-DD format should be parsed as date",
		},
		{
			name:      "future date",
			input:     "2099-12-31",
			expectErr: false,
			desc:      "Future dates should work (may return no results)",
		},
		{
			name:      "hash-like string",
			input:     "abc123xyz456",
			expectErr: true,
			desc:      "Non-date strings treated as hash (likely error if not found)",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout to suppress output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			
			err := HandleInspectCache(tt.input, "config.yaml")
			
			w.Close()
			os.Stdout = oldStdout
			
			// Drain pipe
			var buf bytes.Buffer
			io.Copy(&buf, r)
			
			if tt.expectErr && err == nil {
				t.Errorf("%s: expected error but got nil", tt.desc)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.desc, err)
			}
		})
	}
}

func TestHandleInspectCache_DateBoundaryValues(t *testing.T) {
	tests := []struct {
		date string
		desc string
	}{
		{"2000-01-01", "Y2K date"},
		{"2026-12-31", "End of year"},
		{"2026-02-28", "Non-leap year Feb"},
		{"2024-02-29", "Leap year Feb 29"},
	}
	
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			
			// Should not crash on any valid date format
			_ = HandleInspectCache(tt.date, "config.yaml")
			
			w.Close()
			os.Stdout = oldStdout
			
			// Drain pipe
			var buf bytes.Buffer
			io.Copy(&buf, r)
			// Just verify no panic - output depends on actual cache
		})
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
	io.Copy(&buf, r)
}

func TestShowAvailableDates_NoOutput(t *testing.T) {
	// Private function - test via HandleInspectCache with future date
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	// Future date with no cache triggers showAvailableDates
	_ = HandleInspectCache("2099-12-31", "config.yaml")
	
	w.Close()
	os.Stdout = oldStdout
	
	var buf bytes.Buffer
	io.Copy(&buf, r)
	// Just verify no crash
}

func TestHandleInspectCacheByDate_Integration(t *testing.T) {
	// Test the actual date inspection flow
	testDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	err := HandleInspectCache(testDate, "config.yaml")
	
	w.Close()
	os.Stdout = oldStdout
	
	if err != nil {
		t.Errorf("Date inspection failed: %v", err)
	}
	
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()
	
	// Should handle date format properly (either show results or "no cache" message)
	if output == "" {
		t.Error("Expected some output from date inspection")
	}
}

