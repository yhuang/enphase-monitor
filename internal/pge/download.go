// download.go captures PG&E's Green Button export into a local file.
//
// PG&E's portal is a Salesforce Lightning site built from Lightning Web
// Components, which render inside CLOSED shadow roots. Nothing on the page is
// reachable via document.querySelector — and therefore not via chromedp's
// ByQuery or Evaluate either, since closed roots are invisible to all page
// JavaScript. That rules out auto-driving the export form by selector. So this
// flow is manual-assisted: we open the usage page in a headed, already-signed-in
// browser and the user clicks Green Button → CSV → date range → Download by
// hand.
//
// Completion is detected by WATCHING THE DOWNLOAD DIRECTORY, not via CDP download
// events. PG&E runs the actual download in a popup target whose Browser-domain
// download events do not reach a listener on the page context, so the events
// never arrive even though the file lands correctly. Polling the directory for a
// new, fully-written file is target-agnostic and reliable.
package pge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
)

// greenButtonURL is the PG&E usage page that hosts the Green Button export. PG&E
// moved to a Salesforce Lightning site; this is the post-login landing page, and
// readyURLMarker (login.go) is the substring openLoggedInSession waits for.
const greenButtonURL = "https://myaccount.pge.com/myaccount/s/usageandconsumption-homepage"

// downloadTimeout bounds the wait for the user to complete the export and for the
// file to finish downloading. It is generous because it spans the manual
// click-through of PG&E's Green Button dialog.
const downloadTimeout = 10 * time.Minute

// downloadPollInterval is how often the download directory is scanned for the
// newly exported file.
const downloadPollInterval = 1 * time.Second

// downloadGreenButton captures the Green Button export into dir and returns its
// path. Because PG&E's Lightning UI cannot be driven by selector (closed shadow
// DOM), the user performs the export by hand in the already-open, signed-in
// browser; we route downloads into dir and watch for the new file to appear. The
// caller's ctx must be the headed browser context returned by openLoggedInSession.
func downloadGreenButton(ctx context.Context, from, to time.Time, dir string, notify func(string)) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating download dir %q: %w", dir, err)
	}

	// Route downloads into dir. We detect completion by watching dir (see the file
	// header), so download events are not enabled here.
	if err := chromedp.Run(ctx,
		cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorAllow).
			WithDownloadPath(dir),
	); err != nil {
		return "", fmt.Errorf("enabling browser downloads: %w", err)
	}

	before, err := snapshotFiles(dir)
	if err != nil {
		return "", err
	}

	fd := &formDriver{notify: notify}
	if err := fd.drive(ctx, from, to); err != nil {
		// Flush buffered diagnostics before the fallback message so the user sees
		// what went wrong (frame tree, element dumps, step-by-step trace).
		fd.flushTo(os.Stderr)
		// The auto-drive failed (new form structure, unexpected element names, or a
		// popup that intercepted a click). Fall back to manual: the browser is still
		// open and signed in, so the user can finish the export by hand and we'll
		// capture the download automatically.
		report(notify, fmt.Sprintf(
			"Couldn't auto-drive the export form (%v). Please finish it manually: click \"Green Button → Download my data\", choose XML, set the date range %s to %s, and download. I'll capture the file automatically.",
			err, from.Format(dateFormat), to.Format(dateFormat)))
	}

	deadline := time.Now().Add(downloadTimeout)
	for {
		if name := newCompletedFile(dir, before); name != "" {
			report(notify, "Download captured.")
			return filepath.Join(dir, name), nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out after %s waiting for the PG&E download — was the export completed in the browser?", downloadTimeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(downloadPollInterval):
		}
	}
}

// fileSnapshot records the name and modification time of each regular file in
// dir so a later scan can detect both brand-new files and same-name overwrites
// (PG&E reuses the same UUID-based filename across repeated downloads of the
// same account).
type fileSnapshot map[string]time.Time

// snapshotFiles returns the name→mtime snapshot of all regular files in dir.
func snapshotFiles(dir string) (fileSnapshot, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading download dir %q: %w", dir, err)
	}
	snap := make(fileSnapshot, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil {
			snap[e.Name()] = info.ModTime()
		}
	}
	return snap, nil
}

// newCompletedFile returns the name of a file that is fully written and either
// did not exist in before or has a newer mtime (same-name overwrite). Chrome
// ".crdownload" partials are excluded so the caller keeps polling until the
// rename to the final name occurs.
func newCompletedFile(dir string, before fileSnapshot) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasSuffix(name, ".crdownload") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if priorMtime, seen := before[name]; !seen || info.ModTime().After(priorMtime) {
			return name
		}
	}
	return ""
}
