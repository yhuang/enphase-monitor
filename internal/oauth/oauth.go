// Package oauth handles OAuth 2.0 authentication for the Enphase Cloud API.
//
// PURPOSE
// -------
// This file manages OAuth 2.0 token acquisition and refresh for authenticating
// with the Enphase Enlighten Cloud API v4.
//
// SETUP GUIDE
// -----------
// For step-by-step OAuth setup instructions, see OAUTH_SETUP.md
// To run the interactive setup wizard: ./enphase-monitor --setup-oauth
//
// AUTHENTICATION FLOW
// -------------------
// The Enphase Cloud API uses OAuth 2.0 with two supported grant types:
//
//  1. Refresh Token Grant (Developer Plan - Free Tier):
//     - One-time setup via --setup-oauth wizard
//     - Uses refresh_token to obtain access_token
//     - Access tokens expire (typically 1 hour)
//     - Refresh tokens are long-lived (do not expire)
//
//  2. Password Grant (Partner/Installer Plans):
//     - Uses username/password directly
//     - Less common, primarily for enterprise customers
//
// TOKEN CACHING
// -------------
// Access tokens are cached in memory to avoid unnecessary refresh calls:
//   - Cache key: combination of client_id and grant type
//   - Cache lifetime: until token expires (ExpiresIn seconds)
//   - Thread safety: tokenCache is accessed from main goroutine only (no mutex needed)
//
// When a token is needed:
//  1. Check in-memory cache
//  2. If cached and not expired, return cached token
//  3. If expired or missing, refresh using refresh_token
//  4. Cache new token and return
//
// ERROR HANDLING
// --------------
// Common errors and their handling:
//   - 401 Unauthorized: Token invalid/expired → attempt refresh
//   - 400 Bad Request: Invalid credentials → return error with guidance
//   - Network errors: Returned to caller for handling
//
// The GetAccessToken() function provides helpful error messages suggesting
// users run --setup-oauth if refresh token is missing or invalid.
package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"enphase-monitor/internal/config"
	"enphase-monitor/internal/constants"
)

// Token timing constants
const (
	// tokenRefreshBuffer is how early we refresh tokens before they expire (5 minutes)
	tokenRefreshBuffer = 5 * time.Minute
	// oauthRequestTimeout is the HTTP timeout for OAuth requests
	oauthRequestTimeout = 30 * time.Second
)

// OAuthTokenResponse represents the OAuth token response
type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

// TokenCache stores a cached access token and refresh token
type TokenCache struct {
	Token        string    // OAuth access token (short-lived, ~1 hour)
	RefreshToken string    // OAuth refresh token (long-lived, does not expire)
	ExpiresAt    time.Time // When the access token expires
}

var tokenCache *TokenCache

// oauthHTTPClient is reused across OAuth calls to enable connection reuse.
var oauthHTTPClient = &http.Client{
	Timeout: oauthRequestTimeout,
}

// GetAuthorizationURL generates the authorization URL for the user to visit (one-time setup)
func GetAuthorizationURL(apiConfig *config.APIConfig) (string, error) {
	if apiConfig == nil {
		return "", fmt.Errorf("API configuration is required")
	}
	if apiConfig.ClientID == "" {
		return "", fmt.Errorf("client_id is required")
	}
	if apiConfig.RedirectURI == "" {
		return "", fmt.Errorf("redirect_uri is required in API configuration")
	}

	authURL := constants.EnphaseOAuthAuthorizeURL
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", apiConfig.ClientID)
	params.Set("redirect_uri", apiConfig.RedirectURI)
	params.Set("scope", "read write")

	return fmt.Sprintf("%s?%s", authURL, params.Encode()), nil
}

// ExchangeAuthorizationCode exchanges an authorization code for access and refresh tokens
func ExchangeAuthorizationCode(ctx context.Context, apiConfig *config.APIConfig, code string) (*OAuthTokenResponse, error) {
	if apiConfig == nil {
		return nil, fmt.Errorf("API configuration is required")
	}
	if apiConfig.AuthorizationURL == "" {
		return nil, fmt.Errorf("authorization_url is required in API configuration")
	}
	if apiConfig.ClientID == "" || apiConfig.ClientSecret == "" {
		return nil, fmt.Errorf("client_id and client_secret are required in API configuration")
	}
	if apiConfig.RedirectURI == "" {
		return nil, fmt.Errorf("redirect_uri is required in API configuration")
	}
	if code == "" {
		return nil, fmt.Errorf("authorization code is required")
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", apiConfig.RedirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", apiConfig.AuthorizationURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	// Use Basic Auth with client_id:client_secret as per Enphase API v4 docs
	req.SetBasicAuth(apiConfig.ClientID, apiConfig.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if apiConfig.Key != "" {
		req.Header.Set("key", apiConfig.Key)
	}

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != constants.HTTPStatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp OAuthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// GetAccessToken retrieves an OAuth access token using refresh token or other available methods
func GetAccessToken(ctx context.Context, apiConfig *config.APIConfig) (string, error) {
	// Check cache first - refresh if within buffer window of expiration
	if tokenCache != nil && time.Now().Before(tokenCache.ExpiresAt.Add(-tokenRefreshBuffer)) {
		return tokenCache.Token, nil
	}

	if apiConfig == nil {
		return "", fmt.Errorf("API configuration is required")
	}

	if apiConfig.AuthorizationURL == "" {
		return "", fmt.Errorf("authorization_url is required in API configuration")
	}

	if apiConfig.ClientID == "" || apiConfig.ClientSecret == "" {
		return "", fmt.Errorf("client_id and client_secret are required in API configuration")
	}

	// Determine which grant type to use
	var formData url.Values

	// Priority 1: Use refresh token if available (for developer plan)
	if apiConfig.RefreshToken != "" {
		formData = url.Values{}
		formData.Set("grant_type", "refresh_token")
		formData.Set("refresh_token", apiConfig.RefreshToken)
	} else if apiConfig.Username != "" && apiConfig.Password != "" {
		// Priority 2: Use password grant (for Partner/Installer plans)
		formData = url.Values{}
		formData.Set("grant_type", "password")
		formData.Set("username", apiConfig.Username)
		formData.Set("password", apiConfig.Password)
	} else {
		// No valid authentication method available
		return "", fmt.Errorf("no valid authentication method available. For developer plan: you need to complete one-time authorization to get a refresh_token. See README for instructions.")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiConfig.AuthorizationURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	// Use Basic Auth with client_id:client_secret as per Enphase API v4 docs
	req.SetBasicAuth(apiConfig.ClientID, apiConfig.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if apiConfig.Key != "" {
		req.Header.Set("key", apiConfig.Key)
	}

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != constants.HTTPStatusOK {
		body, _ := io.ReadAll(resp.Body)
		errorMsg := string(body)

		// Provide helpful error message for common OAuth errors
		if resp.StatusCode == constants.HTTPStatusUnauthorized {
			if apiConfig.RefreshToken != "" {
				return "", fmt.Errorf("token request failed with status %d: %s\n\n"+
					"Your refresh token appears to be invalid or expired. Please regenerate it by running:\n"+
					"  ./enphase-monitor --setup-oauth\n\n"+
					"Then update the refresh_token in your config.yaml file.", resp.StatusCode, errorMsg)
			} else {
				return "", fmt.Errorf("token request failed with status %d: %s\n\n"+
					"Authentication failed. Please check your credentials in config.yaml.", resp.StatusCode, errorMsg)
			}
		}

		return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, errorMsg)
	}

	var tokenResp OAuthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no access token in response")
	}

	// Cache the token and refresh token
	expiresIn := tokenResp.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 3600 // Default to 1 hour if not specified
	}

	refreshToken := tokenResp.RefreshToken
	if refreshToken == "" && tokenCache != nil {
		// Keep existing refresh token if new one not provided
		refreshToken = tokenCache.RefreshToken
	}

	tokenCache = &TokenCache{
		Token:        tokenResp.AccessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
	}

	return tokenResp.AccessToken, nil
}
