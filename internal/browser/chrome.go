// Package browser launches headed Chrome for portal automation (chromedp).
package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// chromeStartupTimeout bounds how long we wait for Chrome to open its DevTools port.
const chromeStartupTimeout = 90 * time.Second

// LaunchHeaded returns a chromedp context backed by a visible Chrome window.
// The caller must invoke cancel when done (cancels context then allocator).
func LaunchHeaded(parent context.Context) (ctx context.Context, cancel func(), err error) {
	// Each session gets its own throwaway profile so concurrent or repeated runs
	// never share Chrome state; cleanup removes it so temp does not accumulate.
	profileDir, err := os.MkdirTemp("", "enphase-monitor-chromedp-")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Chrome profile directory: %w", err)
	}

	opts := headedAllocatorOptions(profileDir)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(parent, opts...)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)

	cleanup := func() {
		cancelBrowser()
		cancelAlloc()
		_ = os.RemoveAll(profileDir)
	}

	// Run startup on browserCtx, not a child timeout context. chromedp ties the
	// browser session to the context passed to Run; canceling a child afterward
	// (e.g. defer cancelStart()) tears down the session and breaks later calls.
	errCh := make(chan error, 1)
	go func() {
		errCh <- chromedp.Run(browserCtx, network.Enable(), chromedp.Navigate("about:blank"))
	}()

	select {
	case err := <-errCh:
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("failed to launch Chrome (%s): %w", chromeHint(), err)
		}
	case <-time.After(chromeStartupTimeout):
		cleanup()
		return nil, nil, fmt.Errorf("failed to launch Chrome (%s): timed out after %s waiting for Chrome to start", chromeHint(), chromeStartupTimeout)
	case <-parent.Done():
		cleanup()
		return nil, nil, parent.Err()
	}

	return browserCtx, cleanup, nil
}

func headedAllocatorOptions(profileDir string) []chromedp.ExecAllocatorOption {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.UserDataDir(profileDir),
	)
	if path := findChromeExecPath(); path != "" {
		opts = append(opts, chromedp.ExecPath(path))
	}
	return opts
}

func findChromeExecPath() string {
	if override := os.Getenv("ENPHASE_CHROME_PATH"); override != "" {
		if _, err := os.Stat(override); err == nil {
			return override
		}
	}
	for _, path := range chromeExecCandidates() {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func chromeExecCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "linux":
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		}
	case "windows":
		return []string{
			filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
		}
	default:
		return nil
	}
}

func chromeHint() string {
	if path := findChromeExecPath(); path != "" {
		return "using " + path
	}
	return "set ENPHASE_CHROME_PATH to your Chrome binary if it is installed in a non-standard location"
}
