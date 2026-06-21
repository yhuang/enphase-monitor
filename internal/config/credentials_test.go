package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"enphase-monitor/internal/constants"
)

// TestLoadCredentials tests parsing of the credentials file.
func TestLoadCredentials(t *testing.T) {
	t.Run("valid credentials list", func(t *testing.T) {
		path := writeTempFile(t, "credentials.yaml", `
credentials:
  - name: key1
    key: test-key
    client_id: test-client
    client_secret: test-secret
    authorization_url: https://api.enphaseenergy.com/oauth/token
    refresh_token: test-refresh-token
  - name: key2
    key: test-key-2
    client_id: test-client-2
    client_secret: test-secret-2
`)
		creds, err := LoadCredentials(path)
		if err != nil {
			t.Fatalf("LoadCredentials() unexpected error = %v", err)
		}
		if len(creds) != 2 {
			t.Fatalf("LoadCredentials() returned %d credentials, want 2", len(creds))
		}
		if creds[0].Name != "key1" || creds[0].Key != "test-key" {
			t.Errorf("creds[0] = %+v, want name=key1 key=test-key", creds[0])
		}
		if creds[1].Name != "key2" {
			t.Errorf("creds[1].Name = %q, want key2", creds[1].Name)
		}
	})

	t.Run("missing file reports os.ErrNotExist", func(t *testing.T) {
		_, err := LoadCredentials(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("LoadCredentials() error = %v, want errors.Is(err, os.ErrNotExist)", err)
		}
	})

	t.Run("invalid YAML", func(t *testing.T) {
		path := writeTempFile(t, "credentials.yaml", `this is not valid yaml: [[[`)
		_, err := LoadCredentials(path)
		if err == nil || !strings.Contains(err.Error(), "failed to parse credentials file") {
			t.Errorf("LoadCredentials() error = %v, want parse error", err)
		}
	})
}

// TestApplyCredentials tests validation of the credential pool on a Config.
func TestApplyCredentials(t *testing.T) {
	validCreds := func() []*APIConfig {
		return []*APIConfig{{Name: "key1", Key: "k", ClientID: "c", ClientSecret: "s"}}
	}

	t.Run("credentials are applied and validated", func(t *testing.T) {
		cfg := &Config{}
		if err := cfg.ApplyCredentials(validCreds()); err != nil {
			t.Fatalf("ApplyCredentials() unexpected error = %v", err)
		}
		if len(cfg.Credentials) != 1 || cfg.Credentials[0].Key != "k" {
			t.Errorf("cfg.Credentials = %+v, want one entry with key=k", cfg.Credentials)
		}
	})

	t.Run("multiple credentials are accepted", func(t *testing.T) {
		creds := []*APIConfig{
			{Name: "key1", Key: "k1", ClientID: "c1", ClientSecret: "s1"},
			{Name: "key2", Key: "k2", ClientID: "c2", ClientSecret: "s2"},
		}
		cfg := &Config{}
		if err := cfg.ApplyCredentials(creds); err != nil {
			t.Fatalf("ApplyCredentials() unexpected error = %v", err)
		}
		if len(cfg.Credentials) != 2 {
			t.Errorf("len(cfg.Credentials) = %d, want 2", len(cfg.Credentials))
		}
	})

	t.Run("shared OAuth settings are applied from config.yaml api block", func(t *testing.T) {
		cfg := &Config{API: &OAuthSettings{
			AuthorizationURL: "https://shared.example/token",
			RedirectURI:      "http://localhost:9999/callback",
		}}
		if err := cfg.ApplyCredentials(validCreds()); err != nil {
			t.Fatalf("ApplyCredentials() unexpected error = %v", err)
		}
		got := cfg.Credentials[0]
		if got.AuthorizationURL != "https://shared.example/token" {
			t.Errorf("AuthorizationURL = %q, want shared value", got.AuthorizationURL)
		}
		if got.RedirectURI != "http://localhost:9999/callback" {
			t.Errorf("RedirectURI = %q, want shared value", got.RedirectURI)
		}
	})

	t.Run("authorization_url defaults to the Enphase token endpoint", func(t *testing.T) {
		cfg := &Config{} // no api: block, credential omits authorization_url
		if err := cfg.ApplyCredentials(validCreds()); err != nil {
			t.Fatalf("ApplyCredentials() unexpected error = %v", err)
		}
		if cfg.Credentials[0].AuthorizationURL != constants.EnphaseOAuthTokenURL {
			t.Errorf("AuthorizationURL = %q, want default %q", cfg.Credentials[0].AuthorizationURL, constants.EnphaseOAuthTokenURL)
		}
	})

	t.Run("per-credential OAuth settings override the shared block", func(t *testing.T) {
		cfg := &Config{API: &OAuthSettings{AuthorizationURL: "https://shared.example/token"}}
		creds := validCreds()
		creds[0].AuthorizationURL = "https://override.example/token"
		if err := cfg.ApplyCredentials(creds); err != nil {
			t.Fatalf("ApplyCredentials() unexpected error = %v", err)
		}
		if cfg.Credentials[0].AuthorizationURL != "https://override.example/token" {
			t.Errorf("AuthorizationURL = %q, want per-credential override", cfg.Credentials[0].AuthorizationURL)
		}
	})

	t.Run("refresh token is trimmed", func(t *testing.T) {
		creds := validCreds()
		creds[0].RefreshToken = "  token-with-spaces  "
		cfg := &Config{}
		if err := cfg.ApplyCredentials(creds); err != nil {
			t.Fatalf("ApplyCredentials() unexpected error = %v", err)
		}
		if cfg.Credentials[0].RefreshToken != "token-with-spaces" {
			t.Errorf("RefreshToken = %q, want %q (trimmed)", cfg.Credentials[0].RefreshToken, "token-with-spaces")
		}
	})

	errCases := []struct {
		name        string
		creds       []*APIConfig
		errContains string
	}{
		{name: "no credentials", creds: nil, errContains: "no credentials configured"},
		{name: "missing name", creds: []*APIConfig{{Key: "k", ClientID: "c", ClientSecret: "s"}}, errContains: "missing a name"},
		{name: "duplicate name", creds: []*APIConfig{
			{Name: "dup", Key: "k", ClientID: "c", ClientSecret: "s"},
			{Name: "dup", Key: "k2", ClientID: "c2", ClientSecret: "s2"},
		}, errContains: "duplicate credential name"},
		{name: "missing key", creds: []*APIConfig{{Name: "key1", ClientID: "c", ClientSecret: "s"}}, errContains: "key, client_id, and client_secret"},
		{name: "missing client_id", creds: []*APIConfig{{Name: "key1", Key: "k", ClientSecret: "s"}}, errContains: "key, client_id, and client_secret"},
		{name: "missing client_secret", creds: []*APIConfig{{Name: "key1", Key: "k", ClientID: "c"}}, errContains: "key, client_id, and client_secret"},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{}
			err := cfg.ApplyCredentials(tc.creds)
			if err == nil || !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("ApplyCredentials() error = %v, want error containing %q", err, tc.errContains)
			}
		})
	}
}

// TestUpdateRefreshToken verifies the refresh_token is written into the right
// credential entry while the rest of the file (comments, other entries) survives.
func TestUpdateRefreshToken(t *testing.T) {
	const content = `# Top comment
credentials:
  - name: key1  # inline comment
    key: "k1"
    client_id: "c1"
    client_secret: "s1"
    refresh_token: "old-token"
  - name: key2
    key: "k2"
    client_id: "c2"
    client_secret: "s2"
`

	t.Run("updates existing refresh_token and preserves comments", func(t *testing.T) {
		path := writeTempFile(t, "credentials.yaml", content)
		if err := UpdateRefreshToken(path, "key1", "new-token"); err != nil {
			t.Fatalf("UpdateRefreshToken() error = %v", err)
		}
		got := readFile(t, path)
		if !strings.Contains(got, "refresh_token: new-token\n") {
			t.Errorf("file missing unquoted new token:\n%s", got)
		}
		if strings.Contains(got, `refresh_token: "new-token"`) {
			t.Errorf("new token should be written unquoted:\n%s", got)
		}
		if strings.Contains(got, "old-token") {
			t.Errorf("old token still present:\n%s", got)
		}
		for _, want := range []string{"# Top comment", "# inline comment", "name: key2"} {
			if !strings.Contains(got, want) {
				t.Errorf("file lost %q:\n%s", want, got)
			}
		}
		// 2-space indentation style is preserved (list item at 2, fields at 4).
		if !strings.Contains(got, "\n  - name: key1") || !strings.Contains(got, "\n    refresh_token:") {
			t.Errorf("2-space indentation not preserved:\n%s", got)
		}
		// Reload through the normal path to confirm it still parses and validates.
		creds, err := LoadCredentials(path)
		if err != nil {
			t.Fatalf("LoadCredentials() after update error = %v", err)
		}
		if creds[0].RefreshToken != "new-token" {
			t.Errorf("reloaded key1 refresh_token = %q, want new-token", creds[0].RefreshToken)
		}
	})

	t.Run("adds refresh_token when the entry has none", func(t *testing.T) {
		path := writeTempFile(t, "credentials.yaml", content)
		if err := UpdateRefreshToken(path, "key2", "fresh"); err != nil {
			t.Fatalf("UpdateRefreshToken() error = %v", err)
		}
		creds, err := LoadCredentials(path)
		if err != nil {
			t.Fatalf("LoadCredentials() error = %v", err)
		}
		if creds[1].RefreshToken != "fresh" {
			t.Errorf("key2 refresh_token = %q, want fresh", creds[1].RefreshToken)
		}
	})

	t.Run("unknown credential name errors", func(t *testing.T) {
		path := writeTempFile(t, "credentials.yaml", content)
		if err := UpdateRefreshToken(path, "nope", "x"); err == nil || !strings.Contains(err.Error(), "no credential named") {
			t.Errorf("UpdateRefreshToken() error = %v, want 'no credential named'", err)
		}
	})
}

// readFile returns a temp file's contents, failing the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(b)
}

// writeTempFile writes content to a uniquely-named temp file and returns its path.
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	return path
}
