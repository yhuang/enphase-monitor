// browser.go drives a single headed Chrome session to auto-approve the OAuth
// consent ("Allow Access") for many credentials with only one login — used by
// --update-refresh-tokens --all so the user never clicks Allow Access per app.
//
// For each credential it navigates to the authorization URL, waits for the
// consent page's button (during which the user logs in on the first credential),
// clicks it, and captures the ?code=… from the redirect via the DevTools Protocol
// (so the redirect URI need not be reachable). The code is then exchanged for a
// refresh token by the existing ExchangeAuthorizationCode path.
package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"enphase-monitor/internal/types"

	"enphase-monitor/internal/browser"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	// allowWaitTimeout bounds the wait for the consent button to appear. It is
	// generous because the first credential includes the one-time login.
	allowWaitTimeout = 5 * time.Minute
	// redirectWaitTimeout bounds the wait for the post-approval redirect.
	redirectWaitTimeout = 30 * time.Second
)

// allowAccessSelector matches the "Allow Access" consent button across the likely
// markups (submit input, button, or link). Adjust here if the portal changes.
const allowAccessSelector = `//input[@value='Allow Access'] | //button[contains(normalize-space(.),'Allow Access')] | //a[contains(normalize-space(.),'Allow Access')]`

// BrowserAuthorizer drives one headed Chrome session to approve OAuth consent for
// multiple credentials with a single login. It is not safe for concurrent use;
// call GetCode sequentially. Chrome launches lazily on first use and is torn down
// by Close.
type BrowserAuthorizer struct {
	parent    context.Context
	ctx       context.Context
	cancel    func()
	redirectURI string
	codeCh      chan string
	errCh       chan error
	started     bool

	mu    sync.Mutex // guards armed
	armed bool       // capture redirect events only while true (post-consent click)
}

// NewBrowserAuthorizer returns an authorizer whose Chrome session is governed by
// parent (e.g. a Ctrl+C signal context).
func NewBrowserAuthorizer(parent context.Context) *BrowserAuthorizer {
	return &BrowserAuthorizer{
		parent: parent,
		codeCh: make(chan string, 1),
		errCh:  make(chan error, 1),
	}
}

// Close shuts down the Chrome session.
func (b *BrowserAuthorizer) Close() {
	if b.cancel != nil {
		b.cancel()
	}
}

// AuthorizeViaBrowser obtains a refresh token for api by driving the shared
// browser session b to auto-approve consent, then exchanging the captured code.
func AuthorizeViaBrowser(ctx context.Context, b *BrowserAuthorizer, api *types.APIConfig) (string, error) {
	if api == nil {
		return "", errors.New("API configuration is required")
	}
	if api.ClientID == "" {
		return "", errors.New("client_id is required in API configuration")
	}
	if api.RedirectURI == "" {
		return "", errors.New("a redirect_uri is required (set redirect_uri under api: in config.yaml)")
	}

	authURL, err := GetAuthorizationURL(api)
	if err != nil {
		return "", fmt.Errorf("failed to generate authorization URL: %w", err)
	}
	code, err := b.GetCode(authURL, api.RedirectURI)
	if err != nil {
		return "", err
	}
	tokenResp, err := ExchangeAuthorizationCode(ctx, api, code)
	if err != nil {
		return "", fmt.Errorf("failed to exchange authorization code: %w", err)
	}
	if tokenResp.RefreshToken == "" {
		return "", errors.New("no refresh token in response - authorization may have failed")
	}
	return tokenResp.RefreshToken, nil
}

// GetCode navigates to authURL, waits for and clicks the "Allow Access" button
// (the user logs in during this wait on the first call), and returns the
// authorization code captured from the redirect to redirectURI.
func (b *BrowserAuthorizer) GetCode(authURL, redirectURI string) (string, error) {
	if err := b.ensureStarted(redirectURI); err != nil {
		return "", err
	}

	if err := chromedp.Run(b.ctx, chromedp.Navigate(authURL)); err != nil {
		return "", fmt.Errorf("failed to open authorization page: %w", err)
	}

	// Wait for the consent page. On the first credential the user logs in during
	// this wait; capture stays disarmed so any redirect during the login dance
	// (which can carry a spurious access_denied) is ignored.
	waitCtx, cancel := context.WithTimeout(b.ctx, allowWaitTimeout)
	defer cancel()
	if err := chromedp.Run(waitCtx, chromedp.WaitVisible(allowAccessSelector, chromedp.BySearch)); err != nil {
		return "", fmt.Errorf("could not find the Allow Access button (the consent page markup may have changed): %w", err)
	}

	// Arm capture only now, so only the redirect from this click is recorded.
	b.arm()
	defer b.disarm()

	if err := chromedp.Run(b.ctx,
		chromedp.Sleep(300*time.Millisecond), // let the consent page settle
		chromedp.Click(allowAccessSelector, chromedp.BySearch),
	); err != nil {
		return "", fmt.Errorf("could not click the Allow Access button: %w", err)
	}

	select {
	case code := <-b.codeCh:
		return code, nil
	case err := <-b.errCh:
		return "", err
	case <-b.parent.Done():
		return "", b.parent.Err()
	case <-time.After(redirectWaitTimeout):
		return "", errors.New("timed out waiting for the authorization redirect")
	}
}

// ensureStarted launches Chrome (headed) and registers the redirect listener on
// first use.
func (b *BrowserAuthorizer) ensureStarted(redirectURI string) error {
	if b.started {
		return nil
	}
	b.redirectURI = redirectURI

	ctx, cancel, err := browser.LaunchHeaded(b.parent)
	if err != nil {
		return err
	}

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		e, ok := ev.(*network.EventRequestWillBeSent)
		if !ok {
			return
		}
		b.mu.Lock()
		armed := b.armed
		b.mu.Unlock()
		if !armed {
			return
		}
		switch code, authErr, matched := parseRedirect(e.Request.URL, b.redirectURI); {
		case authErr != nil:
			trySend(b.errCh, authErr)
		case matched:
			trySend(b.codeCh, code)
		}
	})

	b.ctx, b.cancel, b.started = ctx, cancel, true
	return nil
}

// arm enables redirect capture, first discarding any events buffered before now
// (e.g. from a prior credential or the pre-consent login dance).
func (b *BrowserAuthorizer) arm() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for {
		select {
		case <-b.codeCh:
		case <-b.errCh:
		default:
			b.armed = true
			return
		}
	}
}

// disarm stops redirect capture between credentials.
func (b *BrowserAuthorizer) disarm() {
	b.mu.Lock()
	b.armed = false
	b.mu.Unlock()
}

// parseRedirect extracts the authorization code (or an OAuth error) from a
// request URL when it is the redirect to redirectURI. matched is true once the
// request is the redirect carrying a code or error.
func parseRedirect(rawURL, redirectURI string) (code string, authErr error, matched bool) {
	if redirectURI == "" || !strings.HasPrefix(rawURL, redirectURI) {
		return "", nil, false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", nil, false
	}
	if e := u.Query().Get("error"); e != "" {
		return "", fmt.Errorf("authorization failed: %s (check redirect_uri and scope in the Developer Portal)", e), true
	}
	if c := u.Query().Get("code"); c != "" {
		return c, nil, true
	}
	return "", nil, false
}
