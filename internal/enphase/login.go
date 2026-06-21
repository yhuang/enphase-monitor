// login.go drives a real Chrome window so the user can authenticate to the
// Enphase portal themselves (including any MFA), then captures the resulting
// session cookie via the DevTools Protocol. Nothing is stored: Chrome runs with
// a throwaway profile, and the cookie lives only in memory for the scrape.
package enphase

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// loginTimeout bounds how long we wait for the user to finish logging in.
const loginTimeout = 5 * time.Minute

// LoginAndGetCookie opens a visible Chrome window at the portal's applications
// page, waits for the user to authenticate, and returns the session as a Cookie
// header string scoped to baseURL's host (suitable for FetchAllAppCredentials).
// notify, when non-nil, receives short status lines for the user. Chrome uses a
// disposable profile, so no credentials or session are persisted to disk.
func LoginAndGetCookie(ctx context.Context, baseURL string, notify func(string)) (string, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	host, err := hostOf(baseURL)
	if err != nil {
		return "", err
	}

	// Headed Chrome (override the default headless flag) with a throwaway profile.
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", false))
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	report(notify, "Opening Chrome — log in to Enphase (including any 2FA). I'll continue automatically once you're signed in…")

	if err := chromedp.Run(browserCtx, network.Enable(), chromedp.Navigate(baseURL+"/admin/applications")); err != nil {
		return "", fmt.Errorf("failed to open Chrome (is Google Chrome installed?): %w", err)
	}

	// Poll the browser's cookies until the portal session cookie appears, which
	// happens only after a successful login.
	deadline := time.Now().Add(loginTimeout)
	for {
		var cookies []*network.Cookie
		if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = storage.GetCookies().Do(ctx)
			return err
		})); err != nil {
			return "", fmt.Errorf("failed to read browser cookies: %w", err)
		}

		header := cookieHeaderForHost(cookies, host)
		if strings.Contains(header, "user_session=") {
			report(notify, "Signed in — capturing session and scraping…")
			return header, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out after %s waiting for login", loginTimeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// cookieHeaderForHost builds a "name=value; ..." Cookie header from the cookies
// that apply to host.
func cookieHeaderForHost(cookies []*network.Cookie, host string) string {
	var parts []string
	for _, c := range cookies {
		if cookieAppliesToHost(host, c.Domain) {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	return strings.Join(parts, "; ")
}

// cookieAppliesToHost reports whether a cookie with cookieDomain is sent to host
// (exact match, or host is a subdomain of a dotted cookie domain).
func cookieAppliesToHost(host, cookieDomain string) bool {
	d := strings.TrimPrefix(cookieDomain, ".")
	return host == d || strings.HasSuffix(host, "."+d)
}

// hostOf extracts the host from a base URL.
func hostOf(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid base URL %q", baseURL)
	}
	return u.Hostname(), nil
}

// report sends a status line to notify when it is set.
func report(notify func(string), msg string) {
	if notify != nil {
		notify(msg)
	}
}
