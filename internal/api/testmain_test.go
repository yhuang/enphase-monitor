package api

import (
	"os"
	"testing"
)

// TestMain redirects cache I/O to a temp directory so api tests never
// read from or write to the production cache/ directory.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "enphase-api-test-*")
	if err != nil {
		panic("failed to create temp cache dir: " + err.Error())
	}
	defer os.RemoveAll(dir)
	os.Setenv("ENPHASE_CACHE_DIR", dir)
	os.Exit(m.Run())
}
