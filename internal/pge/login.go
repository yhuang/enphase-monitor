// login.go opens a headed Chrome session against PG&E on a *persistent* profile
// and blocks until the user has signed in. The persistent profile is the key
// efficiency lever: a session established (including any MFA) on the first run is
// reused on every later run, so subsequent pulls skip the login entirely and go
// straight to the Green Button download.
//
// "Signed in" is detected by URL rather than by a page element. PG&E's portal is
// a Salesforce Lightning site whose content renders inside CLOSED shadow roots
// that document.querySelector cannot see (and neither can chromedp), so probing
// for an element is unreliable. Instead we navigate to the usage page and wait
// until the browser's URL settles on it (readyURLMarker): an unauthenticated
// visit redirects to the login screen — a different URL — and a successful
// sign-in redirects back, so the marker appears exactly when we are both logged
// in and on the right page.
package pge

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"enphase-monitor/internal/browser"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// readyURLMarker is the substring of the usage-page URL that signals "signed in
// and on the Green Button page". It is matched against the live browser URL.
const readyURLMarker = "usageandconsumption"

// loginTimeout bounds how long we wait for the user to finish signing in,
// including any MFA step.
const loginTimeout = 10 * time.Minute

// loginPollInterval is the delay between checks for the signed-in/ready URL.
const loginPollInterval = 2 * time.Second

// openLoggedInSession launches headed Chrome on the persistent profileDir,
// navigates to startURL, and blocks until the browser's URL contains
// readyURLSubstr — which only happens once the user is authenticated and the
// target page has loaded. It returns the browser context plus a cleanup func the
// caller must invoke. Because the profile persists, a session from a prior run is
// reused and the marker is typically already present on the first poll (no login
// needed).
func openLoggedInSession(ctx context.Context, profileDir, startURL, readyURLSubstr string, notify func(string)) (context.Context, func(), error) {
	bctx, cancel, err := browser.LaunchHeadedWithProfile(ctx, profileDir)
	if err != nil {
		return nil, nil, err
	}

	if err := chromedp.Run(bctx, chromedp.Navigate(startURL)); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("opening PG&E usage page: %w", err)
	}

	report(notify, "Opening Chrome — sign in to PG&E if prompted (including any 2FA). I'll continue automatically once the usage page loads…")

	if err := awaitLoginScrollingToTop(bctx, readyURLSubstr, loginTimeout); err != nil {
		cancel()
		return nil, nil, err
	}
	report(notify, "Signed in — ready for the Green Button download…")
	return bctx, cancel, nil
}

// sfScrollToTopScript resets every plausible Salesforce Lightning scroll
// container to the top. Salesforce LWC renders content inside a <main> or
// .siteBody element rather than relying on window scroll, so window.scrollTo
// alone does not reach the login form.
const sfScrollToTopScript = `(function(){
	window.scrollTo(0, 0);
	try { document.documentElement.scrollTop = 0; } catch(e) {}
	try { document.body.scrollTop = 0; } catch(e) {}
	['main','[role="main"]','.siteBody','.forceSiteBody',
	 '.contentArea','.content-area','.slds-template__container',
	 'section','article'].forEach(function(sel){
		document.querySelectorAll(sel).forEach(function(el){
			el.scrollTop = 0;
		});
	});
})()`

// urlPathContains reports whether the path component of rawURL contains substr.
// This avoids false positives when substr appears only in a query parameter —
// e.g. the PG&E login URL embeds the usage-page path in its startURL param:
//
//	/myaccount/s/login/?startURL=%2Fmyaccount%2Fs%2Fusageandconsumption-homepage
//
// A plain strings.Contains check on the full URL would match the login page,
// causing the app to think the user is already signed in.
func urlPathContains(rawURL, substr string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return strings.Contains(rawURL, substr)
	}
	return strings.Contains(u.Path, substr)
}

// awaitLoginScrollingToTop polls until the path of the browser URL contains
// readyURLSubstr. On every tick while still on the login page it resets every
// Salesforce scroll container to the top so PG&E's JavaScript cannot push the
// login form below the viewport. URL detection uses Target.GetTargetInfo (a
// pure CDP call) so no JavaScript is evaluated in the page during the wait.
// Once signed in, all activity stops and the function returns.
func awaitLoginScrollingToTop(ctx context.Context, readyURLSubstr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		u, _ := targetURL(ctx)
		if urlPathContains(u, readyURLSubstr) {
			return nil
		}
		_ = chromedp.Run(ctx, chromedp.Evaluate(sfScrollToTopScript, nil))
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for PG&E sign-in", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(loginPollInterval):
		}
	}
}

// targetURL returns the current URL via CDP Target.GetTargetInfo — a pure
// protocol call that evaluates no JavaScript in the page and cannot trigger
// Salesforce's reactive rendering.
func targetURL(ctx context.Context) (string, error) {
	var info *target.Info
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		info, err = target.GetTargetInfo().Do(ctx)
		return err
	})); err != nil {
		return "", err
	}
	return info.URL, nil
}

// currentURL returns the browser's current document URL. It never blocks on a
// missing element, so it is safe to call in a poll loop.
func currentURL(ctx context.Context) (string, error) {
	var url string
	err := chromedp.Run(ctx, chromedp.Location(&url))
	return url, err
}
