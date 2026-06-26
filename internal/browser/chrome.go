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

// LaunchHeaded returns a chromedp context backed by a visible Chrome window with
// a throwaway profile that is deleted on cleanup, so concurrent or repeated runs
// never share Chrome state. Suitable for one-shot scrapes that re-authenticate
// every time. The caller must invoke cancel when done.
func LaunchHeaded(parent context.Context) (context.Context, func(), error) {
	return LaunchHeadedWithProfile(parent, "")
}

// LaunchHeadedWithProfile is LaunchHeaded with control over the Chrome profile
// directory. When profileDir is empty, a throwaway temp profile is created and
// removed on cleanup (the LaunchHeaded behavior). When profileDir is non-empty,
// that directory is used and left in place on cleanup, so a session logged in
// once (including MFA) persists across runs and later runs skip the login.
func LaunchHeadedWithProfile(parent context.Context, profileDir string) (context.Context, func(), error) {
	persistent := profileDir != ""
	if persistent {
		if err := os.MkdirAll(profileDir, 0o700); err != nil {
			return nil, nil, fmt.Errorf("failed to create Chrome profile directory %q: %w", profileDir, err)
		}
	} else {
		var err error
		profileDir, err = os.MkdirTemp("", "enphase-monitor-chromedp-")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Chrome profile directory: %w", err)
		}
	}

	opts := headedAllocatorOptions(profileDir)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(parent, opts...)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)

	cleanup := func() {
		cancelBrowser()
		cancelAlloc()
		if !persistent {
			_ = os.RemoveAll(profileDir)
		}
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
	opts := append(
		append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...),
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
