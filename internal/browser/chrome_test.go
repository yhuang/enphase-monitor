package browser

import (
	"os"
	"runtime"
	"testing"
)

func TestFindChromeExecPath(t *testing.T) {
	path := findChromeExecPath()
	if runtime.GOOS == "darwin" {
		if path == "" {
			t.Skip("Google Chrome not installed at default macOS path")
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("findChromeExecPath() = %q, stat error: %v", path, err)
		}
	}
}

func TestFindChromeExecPathOverride(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("override test uses macOS Chrome path")
	}
	const macChrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	if _, err := os.Stat(macChrome); err != nil {
		t.Skip("Google Chrome not installed")
	}
	t.Setenv("ENPHASE_CHROME_PATH", macChrome)
	if got := findChromeExecPath(); got != macChrome {
		t.Errorf("findChromeExecPath() = %q, want %q", got, macChrome)
	}
}
