// authorization.go implements the interactive OAuth authorization flow that
// obtains a refresh token (browser consent → authorization code → token).
// Package comment and token/refresh logic are in oauth.go.
//
// PURPOSE
// -------
// Guides users through the OAuth 2.0 authorization flow to obtain a refresh token.
//
// AUTHORIZATION FLOW
// ------------------
//  1. Resolves the redirect URI (shared config or prompt)
//  2. Generates the authorization URL and opens the browser
//  3. Captures the authorization code: when the redirect URI is a loopback
//     address with a free port, a local listener receives it automatically
//     (no copy-paste); otherwise the user pastes the redirect URL back
//  4. Exchanges the code for access and refresh tokens
//  5. Returns the refresh token; the caller writes it into credentials.yaml
//     (config.UpdateRefreshToken), so the secret is never displayed.
package oauth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"enphase-monitor/internal/types"
)

// callbackTimeout bounds how long Authorize waits for the browser redirect when
// using the local callback listener before giving up.
const callbackTimeout = 5 * time.Minute

// openBrowser opens the specified URL in the default browser. It is a package
// variable so tests can stub it (and drive the local callback) without launching
// a real browser.
var openBrowser = func(url string) error {
	// Map of OS to command
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// Authorize runs the one-time OAuth authorization flow for a single credential
// set and returns the obtained refresh token. ctx is used for cancellation
// (e.g. Ctrl+C during token exchange). The caller selects which credential to
// authorize (--update-refresh-token <name>) and is responsible for persisting
// the returned token (config.UpdateRefreshToken).
func Authorize(ctx context.Context, api *types.APIConfig) (string, error) {
	if api == nil {
		return "", errors.New("API configuration is required")
	}

	if api.ClientID == "" {
		return "", errors.New("client_id is required in API configuration")
	}

	// Get redirect URI from config or prompt
	redirectURI := api.RedirectURI
	if redirectURI == "" {
		fmt.Print("Enter your redirect URI (e.g., http://localhost:8080/callback): ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read redirect URI from stdin: %w", err)
		}
		redirectURI = strings.TrimSpace(input)
		if redirectURI == "" {
			redirectURI = "http://localhost:8080/callback"
			fmt.Printf("Using default: %s\n", redirectURI)
		}
		// Update config with the redirect URI (needed for GetAuthorizationURL)
		api.RedirectURI = redirectURI
	}

	// Generate authorization URL
	authURL, err := GetAuthorizationURL(api)
	if err != nil {
		return "", fmt.Errorf("failed to generate authorization URL: %w", err)
	}

	fmt.Println("\n==========================================")
	fmt.Println("Enphase OAuth Refresh Token Setup")
	fmt.Println("==========================================")
	fmt.Println()
	fmt.Println("Loaded configuration:")
	clientIDPreview := api.ClientID
	if len(clientIDPreview) > 20 {
		clientIDPreview = clientIDPreview[:20] + "..."
	}
	apiKeyPreview := api.Key
	if len(apiKeyPreview) > 20 {
		apiKeyPreview = apiKeyPreview[:20] + "..."
	}
	fmt.Printf("  Client ID: %s\n", clientIDPreview)
	fmt.Printf("  API Key: %s\n", apiKeyPreview)
	fmt.Printf("  Redirect URI: %s\n", redirectURI)
	fmt.Println()

	// Obtain the authorization code. When the redirect URI is a loopback address
	// and the port is free, a local listener captures the code automatically; the
	// user just authorizes in the browser. Otherwise fall back to manual paste.
	code, err := obtainAuthorizationCode(ctx, authURL, redirectURI)
	if err != nil {
		return "", err
	}

	// Exchange code for tokens
	fmt.Println("\nExchanging authorization code for tokens...")
	tokenResp, err := ExchangeAuthorizationCode(ctx, api, code)
	if err != nil {
		return "", fmt.Errorf("failed to exchange authorization code: %w", err)
	}

	if tokenResp.RefreshToken == "" {
		return "", errors.New("no refresh token in response - authorization may have failed")
	}

	// The full refresh token is intentionally not printed; the caller writes it
	// straight into credentials.yaml. Show only a truncated preview for assurance.
	refreshTokenPreview := tokenResp.RefreshToken
	if len(refreshTokenPreview) > 12 {
		refreshTokenPreview = refreshTokenPreview[:12] + "..."
	}
	fmt.Printf("\n✓ Success! Received a refresh token (%s) for credential %q.\n", refreshTokenPreview, api.Name)

	return tokenResp.RefreshToken, nil
}

// obtainAuthorizationCode returns the OAuth authorization code. It prefers a
// local callback listener — when the redirect URI is a loopback address with a
// bindable port, the browser redirect is captured automatically and the user
// never copies anything. When that isn't possible (non-loopback redirect URI or
// the port is in use) it falls back to the manual paste flow.
func obtainAuthorizationCode(ctx context.Context, authURL, redirectURI string) (string, error) {
	if ln, path, ok := listenForCallback(redirectURI); ok {
		defer ln.Close()
		return captureCodeViaCallback(ctx, ln, path, authURL)
	}
	return pasteAuthorizationCode(authURL, redirectURI)
}

// listenForCallback binds a TCP listener on the redirect URI's host:port when it
// is a loopback address with an explicit port. It returns the listener, the
// callback path to handle, and ok=false (no listener) when auto-capture isn't
// applicable or the port can't be bound.
func listenForCallback(redirectURI string) (net.Listener, string, bool) {
	u, err := url.Parse(redirectURI)
	if err != nil || !isLoopbackHost(u.Hostname()) || u.Port() == "" {
		return nil, "", false
	}
	ln, err := net.Listen("tcp", u.Host)
	if err != nil {
		return nil, "", false
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	return ln, path, true
}

// isLoopbackHost reports whether host is localhost or a loopback IP.
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// captureCodeViaCallback opens the browser to authURL and serves the callback
// path on ln, returning the authorization code as soon as the browser is
// redirected back. It honors ctx cancellation (Ctrl+C) and a hard timeout.
func captureCodeViaCallback(ctx context.Context, ln net.Listener, callbackPath, authURL string) (string, error) {
	type result struct {
		code string
		err  error
	}
	resCh := make(chan result, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			writeCallbackPage(w, false)
			trySend(resCh, result{err: fmt.Errorf("authorization failed: %s (check that redirect_uri and scope match your Developer Portal app)", e)})
			return
		}
		code := q.Get("code")
		if code == "" {
			// Stray request (e.g. /favicon.ico) — not the redirect we want.
			http.NotFound(w, r)
			return
		}
		writeCallbackPage(w, true)
		trySend(resCh, result{code: code})
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Close()

	fmt.Println("Opening your browser to authorize...")
	if err := openBrowser(authURL); err != nil {
		fmt.Println("Could not open the browser automatically. Open this URL manually:")
	} else {
		fmt.Println("If the browser did not open, use this URL:")
	}
	fmt.Println()
	fmt.Println(authURL)
	fmt.Println()
	fmt.Println("Waiting for you to log in and authorize (Ctrl+C to cancel)...")

	timer := time.NewTimer(callbackTimeout)
	defer timer.Stop()

	select {
	case res := <-resCh:
		if res.err != nil {
			return "", res.err
		}
		fmt.Println("✓ Authorization received.")
		return res.code, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
		return "", fmt.Errorf("timed out after %s waiting for browser authorization", callbackTimeout)
	}
}

// writeCallbackPage renders a minimal page telling the user to return to the terminal.
func writeCallbackPage(w http.ResponseWriter, success bool) {
	msg := "Authorization complete — you can close this tab and return to the terminal."
	if !success {
		msg = "Authorization failed — check the terminal for details."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!doctype html><html><body style=\"font-family:sans-serif;text-align:center;padding-top:3em\"><h2>Enphase Monitor</h2><p>%s</p></body></html>", msg)
}

// trySend delivers a result without blocking if the channel is already full.
func trySend[T any](ch chan T, v T) {
	select {
	case ch <- v:
	default:
	}
}

// pasteAuthorizationCode is the manual fallback: it opens the browser, prints
// the authorization URL and instructions, and reads the redirect URL the user
// pastes back, extracting the authorization code from it.
func pasteAuthorizationCode(authURL, redirectURI string) (string, error) {
	fmt.Println("STEP 1: Opening your browser to the authorization page...")
	fmt.Println()
	msg := "Browser opened! If it did not open, copy and paste this URL:"
	if err := openBrowser(authURL); err != nil {
		msg = "Could not open browser automatically.\nPlease copy and paste this URL into your browser:"
	}
	fmt.Println(msg)
	fmt.Println()
	fmt.Println(authURL)
	fmt.Println()
	fmt.Println("STEP 2: Log in with your Enlighten account and authorize the application")
	fmt.Println()
	fmt.Println("STEP 3: After authorization, you will be redirected to:")
	fmt.Printf("  %s?code=ABC123...\n", redirectURI)
	fmt.Println()
	fmt.Println("STEP 4: IMPORTANT - The browser will show 'Site not found' - THIS IS NORMAL!")
	fmt.Println("        Just look at the address bar and copy the ENTIRE URL")
	fmt.Println("        The URL will contain either 'code=' (success) or 'error=' (failure)")
	fmt.Println()
	fmt.Print("Paste the ENTIRE redirect URL from your browser address bar: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read authorization URL from stdin: %w", err)
	}
	code := strings.TrimSpace(input)

	if code == "" {
		return "", errors.New("authorization code is required")
	}

	// Check if user pasted a URL with an error
	if strings.Contains(code, "error=") {
		fmt.Println()
		fmt.Println("ERROR: Authorization failed. The redirect URL contains an error.")
		fmt.Printf("URL: %s\n", code)
		fmt.Println()
		fmt.Println("Common issues:")
		fmt.Println("1. The redirect_uri does not match your app settings in Developer Portal")
		fmt.Println("2. You used the wrong authorization URL (should have 'read write' scope)")
		fmt.Println()
		return "", errors.New("authorization failed - please check the error in the URL and try again")
	}

	// Extract code from URL if user pasted full URL
	if strings.Contains(code, "code=") {
		if parsedURL, err := url.Parse(code); err == nil {
			code = parsedURL.Query().Get("code")
		}
	}

	if code == "" {
		fmt.Println()
		fmt.Println("Could not find 'code=' or 'error=' in the URL you provided.")
		fmt.Println("Please paste the complete URL from your browser address bar.")
		return "", errors.New("invalid redirect URL - no authorization code found")
	}

	return code, nil
}
