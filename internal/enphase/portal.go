// Package enphase reads application credentials from the Enphase developer
// portal (developer-v4.enphase.com), which exposes no management API.
//
// The portal renders each application's name and API Key into the page HTML, but
// the Client ID and Client Secret are NOT in the server HTML — the page's
// JavaScript fetches them from a separate JSON endpoint
// (api.enphaseenergy.com/api/get_client_appication) keyed by the app's API Key
// and the account email. This package mirrors that flow:
//
//  1. GET the applications list (session-cookie authed) to enumerate apps and
//     their names.
//  2. GET each application's detail page (authed) to read its API Key and the
//     account email from the inline `var key` / `var email` script values.
//  3. GET the get_client_appication JSON endpoint (no auth — keyed by API Key +
//     email) to obtain the Client ID and Client Secret.
//
// The regexes below are the single place to adjust if the portal markup changes.
package enphase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"enphase-monitor/internal/config"
)

// DefaultBaseURL is the production Enphase developer portal origin.
const DefaultBaseURL = "https://developer-v4.enphase.com"

// clientAppAPIURL returns an application's Client ID and Client Secret as JSON,
// keyed by its API Key and the account email. The endpoint name is misspelled
// ("appication") in the portal's own JavaScript; it must be matched exactly.
const clientAppAPIURL = "https://api.enphaseenergy.com/api/get_client_appication"

// httpTimeout bounds each HTTP request.
const httpTimeout = 30 * time.Second

// appAnchorPattern matches each application's detail link and visible name on the
// applications list page: <a href="/admin/applications/<id>">name</a>. It excludes
// deeper links (.../edit, .../alerts) by requiring an all-digit final segment.
var appAnchorPattern = regexp.MustCompile(`<a [^>]*href="(/admin/applications/\d+)"[^>]*>\s*([^<]+?)\s*</a>`)

// keyPattern and emailPattern extract the inline script values the detail page
// uses to call the client-app endpoint: `var key = "..."` and `var email = "..."`.
var (
	keyPattern   = regexp.MustCompile(`var key\s*=\s*"([^"]+)"`)
	emailPattern = regexp.MustCompile(`var email\s*=\s*"([^"]+)"`)
)

// portalApp is one application discovered on the list page.
type portalApp struct {
	name string
	path string // e.g. /admin/applications/1409625149340
}

// ProgressFunc reports per-credential scrape progress: done counts apps as each
// begins processing (1-based), total is how many will be processed, and name is
// the current app. It is called once before each app's network requests.
type ProgressFunc func(done, total int, name string)

// FetchAllAppCredentials reads applications' credentials from the Enphase
// developer portal. sessionCookie is the raw Cookie request-header value from a
// logged-in browser session; baseURL defaults to DefaultBaseURL when empty. When
// namePrefix is non-empty, only applications whose name starts with it are
// fetched (the rest are skipped before any per-app request). progress, when
// non-nil, is called as each app is processed. For each included application it
// returns name, key, client_id, and client_secret.
func FetchAllAppCredentials(ctx context.Context, baseURL, sessionCookie, namePrefix string, progress ProgressFunc) ([]config.SeedCredential, error) {
	if strings.TrimSpace(sessionCookie) == "" {
		return nil, errors.New("a portal session cookie is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	client := &http.Client{Timeout: httpTimeout}

	listHTML, err := get(ctx, client, baseURL+"/admin/applications", sessionCookie)
	if err != nil {
		return nil, fmt.Errorf("failed to load applications list: %w", err)
	}
	apps := parseAppList(listHTML)
	if namePrefix != "" {
		kept := apps[:0]
		for _, app := range apps {
			if strings.HasPrefix(app.name, namePrefix) {
				kept = append(kept, app)
			}
		}
		apps = kept
	}
	if len(apps) == 0 {
		return nil, fmt.Errorf("no applications found on %s/admin/applications — the session cookie may be expired, the page layout changed, or none matched the name prefix", baseURL)
	}

	creds := make([]config.SeedCredential, 0, len(apps))
	for i, app := range apps {
		if progress != nil {
			progress(i+1, len(apps), app.name)
		}
		detailHTML, err := get(ctx, client, baseURL+app.path, sessionCookie)
		if err != nil {
			return nil, fmt.Errorf("failed to load %s (%s): %w", app.name, app.path, err)
		}
		key := firstSubmatch(keyPattern, detailHTML)
		email := firstSubmatch(emailPattern, detailHTML)
		if key == "" || email == "" {
			return nil, fmt.Errorf("could not read API key/email from %s detail page — the portal markup likely changed (update keyPattern/emailPattern in internal/enphase/portal.go)", app.name)
		}

		clientID, clientSecret, err := fetchClientApp(ctx, client, key, email)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch client id/secret for %s: %w", app.name, err)
		}
		creds = append(creds, config.SeedCredential{
			Name:         app.name,
			Key:          key,
			ClientID:     clientID,
			ClientSecret: clientSecret,
		})
	}
	return creds, nil
}

// parseAppList extracts the applications and their names from the list page,
// preserving first-seen order and de-duplicating repeated links to the same app.
func parseAppList(listHTML string) []portalApp {
	seen := make(map[string]bool)
	var apps []portalApp
	for _, m := range appAnchorPattern.FindAllStringSubmatch(listHTML, -1) {
		path, name := m[1], strings.TrimSpace(m[2])
		if name == "" || seen[path] {
			continue
		}
		seen[path] = true
		apps = append(apps, portalApp{name: name, path: path})
	}
	return apps
}

// fetchClientApp calls the get_client_appication endpoint and returns the
// application's Client ID and Client Secret. The endpoint needs no session
// cookie — it is keyed by the API key and account email.
func fetchClientApp(ctx context.Context, client *http.Client, key, email string) (clientID, clientSecret string, err error) {
	u := clientAppAPIURL + "?key=" + url.QueryEscape(key) + "&email=" + url.QueryEscape(email)
	body, err := get(ctx, client, u, "")
	if err != nil {
		return "", "", err
	}
	var r struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Message      string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return "", "", fmt.Errorf("failed to parse client-app response: %w", err)
	}
	if r.Message != "" {
		return "", "", fmt.Errorf("client-app endpoint returned: %s", r.Message)
	}
	if r.ClientID == "" || r.ClientSecret == "" {
		return "", "", errors.New("client-app response missing client_id/client_secret")
	}
	return r.ClientID, r.ClientSecret, nil
}

// firstSubmatch returns the first capture group of re in s, or "".
func firstSubmatch(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// get performs a GET and returns the body. sessionCookie, when non-empty, is sent
// as the Cookie header (omitted for the cookie-less client-app endpoint so the
// portal session is never leaked cross-host).
func get(ctx context.Context, client *http.Client, rawURL, sessionCookie string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	if sessionCookie != "" {
		req.Header.Set("Cookie", sessionCookie)
	}
	req.Header.Set("Accept", "text/html,application/json")
	// The portal's 3scale/openresty front end can reject the default Go user
	// agent; present a browser-like one.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) enphase-monitor")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request to %s returned status %d (session cookie may be invalid or expired)", rawURL, resp.StatusCode)
	}
	return string(body), nil
}
