// auth.go handles PG&E Share My Data OAuth2 over mutual TLS.
//
// PG&E uses two token types, both attached to the same mTLS certificate:
//
//   - Client Access Token  — client_credentials grant from client_id/secret.
//     Used for service-status checks. Access token lives ~1h.
//   - User Access Token    — authorization_code grant after the customer
//     completes the SMD click-through. Used for per-UsagePoint data pulls.
//     Access token lives ~1h; the refresh token lives ~1 year and is rotated on
//     each refresh, so we persist whatever the latest response returns.
//
// Tokens are persisted to a JSON store so refreshes survive across runs.
package pge

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// clientExpirySkew is subtracted from a token's lifetime so we refresh slightly
// before the server-side expiry rather than racing it.
const clientExpirySkew = 30 * time.Second

// tokenPair holds an access token and its refresh token, plus expiry metadata.
type tokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int       `json:"expires_in"`
	IssuedAt     time.Time `json:"issued_at"`
	TokenType    string    `json:"token_type"`
}

// valid reports whether the access token is present and not within the expiry
// skew of running out.
func (t *tokenPair) valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	lifetime := time.Duration(t.ExpiresIn)*time.Second - clientExpirySkew
	return time.Now().Before(t.IssuedAt.Add(lifetime))
}

// store persists both token pairs on disk so refreshes survive process restarts.
type store struct {
	ClientToken *tokenPair `json:"client_token,omitempty"`
	UserToken   *tokenPair `json:"user_token,omitempty"`
	path        string
}

func loadStore(path string) (*store, error) {
	s := &store{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	return s, json.Unmarshal(data, s)
}

func (s *store) save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// client wraps http.Client with the mTLS certificate and PG&E credential handling.
type client struct {
	clientID     string
	clientSecret string
	tokenEP      string
	httpClient   *http.Client
	store        *store
}

// newClient builds a client with the mTLS certificate loaded and the token store
// read from disk.
func newClient(cfg *Config) (*client, error) {
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading mTLS cert: %w", err)
	}

	st, err := loadStore(cfg.TokenStore)
	if err != nil {
		return nil, fmt.Errorf("loading token store: %w", err)
	}

	return &client{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		tokenEP:      cfg.TokenEndpoint,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{cert},
					MinVersion:   tls.VersionTLS12,
				},
			},
		},
		store: st,
	}, nil
}

// basicAuth returns the Base64-encoded "clientID:clientSecret" header value.
func (c *client) basicAuth() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.clientID+":"+c.clientSecret))
}

// postToken performs a token POST and parses PG&E's response. PG&E returns XML
// for the client_credentials grant but JSON for authorization_code/refresh_token,
// so both shapes are handled.
func (c *client) postToken(ctx context.Context, params url.Values) (*tokenPair, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenEP+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.basicAuth())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed %d: %s", resp.StatusCode, body)
	}

	pair := &tokenPair{IssuedAt: time.Now()}
	if strings.HasPrefix(strings.TrimSpace(string(body)), "<") {
		// XML response (client_credentials grant).
		pair.AccessToken = xmlField(string(body), "client_access_token")
		if pair.AccessToken == "" {
			pair.AccessToken = xmlField(string(body), "access_token")
		}
		pair.RefreshToken = xmlField(string(body), "refresh_token")
		pair.ExpiresIn = 3600
	} else {
		// JSON response (authorization_code / refresh_token grants).
		if err := json.Unmarshal(body, pair); err != nil {
			return nil, fmt.Errorf("parsing token response: %w", err)
		}
	}

	if pair.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response: %s", body)
	}
	return pair, nil
}

// xmlField extracts <tag>value</tag> from a simple, namespace-free XML string.
func xmlField(xml, tag string) string {
	open, closeTag := "<"+tag+">", "</"+tag+">"
	s := strings.Index(xml, open)
	if s < 0 {
		return ""
	}
	s += len(open)
	e := strings.Index(xml[s:], closeTag)
	if e < 0 {
		return ""
	}
	return strings.TrimSpace(xml[s : s+e])
}

// exchangeAuthCode exchanges a one-time authorization code for a User Access
// Token, persisting it to the store.
func (c *client) exchangeAuthCode(ctx context.Context, code, redirectURI string) error {
	pair, err := c.postToken(ctx, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	})
	if err != nil {
		return fmt.Errorf("exchanging auth code: %w", err)
	}
	c.store.UserToken = pair
	return c.store.save()
}

// userToken returns a valid User Access Token, refreshing it if needed. PG&E
// rotates the refresh token on each refresh, so the rotated pair is persisted.
func (c *client) userToken(ctx context.Context) (string, error) {
	if c.store.UserToken == nil || c.store.UserToken.RefreshToken == "" {
		return "", fmt.Errorf("no PG&E user token — run: --pge-authorize --pge-code <CODE> --pge-redirect-uri <URI>")
	}
	if c.store.UserToken.valid() {
		return c.store.UserToken.AccessToken, nil
	}
	pair, err := c.postToken(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.store.UserToken.RefreshToken},
	})
	if err != nil {
		return "", fmt.Errorf("refreshing user token: %w", err)
	}
	c.store.UserToken = pair
	if err := c.store.save(); err != nil {
		return "", fmt.Errorf("saving refreshed user token: %w", err)
	}
	return pair.AccessToken, nil
}

// get issues a GET with the given Bearer token over the mTLS transport.
func (c *client) get(ctx context.Context, rawURL, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return c.httpClient.Do(req)
}
