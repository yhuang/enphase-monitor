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

func TestMergeSeedCredentials(t *testing.T) {
	const content = `# Top comment
credentials:
  - name: app-001  # inline comment
    key: "old-key"
    client_id: "old-cid"
    client_secret: "old-secret"
    refresh_token: "keep-me"
`

	t.Run("resyncs existing secrets but preserves refresh_token and comments", func(t *testing.T) {
		path := writeTempFile(t, "credentials.yaml", content)
		updated, added, err := MergeSeedCredentials(path, []SeedCredential{
			{Name: "app-001", Key: "new-key", ClientID: "new-cid", ClientSecret: "new-secret"},
		})
		if err != nil {
			t.Fatalf("MergeSeedCredentials() error = %v", err)
		}
		if updated != 1 || added != 0 {
			t.Errorf("updated=%d added=%d, want updated=1 added=0", updated, added)
		}
		creds, err := LoadCredentials(path)
		if err != nil {
			t.Fatalf("LoadCredentials() error = %v", err)
		}
		got := creds[0]
		if got.Key != "new-key" || got.ClientID != "new-cid" || got.ClientSecret != "new-secret" {
			t.Errorf("secrets not resynced: %+v", got)
		}
		if got.RefreshToken != "keep-me" {
			t.Errorf("refresh_token = %q, want preserved keep-me", got.RefreshToken)
		}
		if raw := readFile(t, path); !strings.Contains(raw, "# Top comment") || !strings.Contains(raw, "# inline comment") {
			t.Errorf("comments lost:\n%s", raw)
		}
	})

	t.Run("appends new entry with empty refresh_token", func(t *testing.T) {
		path := writeTempFile(t, "credentials.yaml", content)
		updated, added, err := MergeSeedCredentials(path, []SeedCredential{
			{Name: "app-002", Key: "k2", ClientID: "c2", ClientSecret: "s2"},
		})
		if err != nil {
			t.Fatalf("MergeSeedCredentials() error = %v", err)
		}
		if updated != 0 || added != 1 {
			t.Errorf("updated=%d added=%d, want updated=0 added=1", updated, added)
		}
		creds, err := LoadCredentials(path)
		if err != nil {
			t.Fatalf("LoadCredentials() error = %v", err)
		}
		if len(creds) != 2 {
			t.Fatalf("got %d credentials, want 2", len(creds))
		}
		if creds[1].Name != "app-002" || creds[1].Key != "k2" {
			t.Errorf("appended entry = %+v, want app-002/k2", creds[1])
		}
		if creds[1].RefreshToken != "" {
			t.Errorf("new entry refresh_token = %q, want empty", creds[1].RefreshToken)
		}
		// The empty token is written bare (refresh_token:), not as "".
		if raw := readFile(t, path); strings.Contains(raw, `refresh_token: ""`) {
			t.Errorf("new entry should have a bare refresh_token, not empty quotes:\n%s", raw)
		}
		// The untouched first entry keeps its token.
		if creds[0].RefreshToken != "keep-me" {
			t.Errorf("app-001 refresh_token = %q, want keep-me", creds[0].RefreshToken)
		}
	})

	t.Run("blank scraped field does not clobber a stored secret", func(t *testing.T) {
		path := writeTempFile(t, "credentials.yaml", content)
		if _, _, err := MergeSeedCredentials(path, []SeedCredential{
			{Name: "app-001", Key: "", ClientID: "new-cid", ClientSecret: ""},
		}); err != nil {
			t.Fatalf("MergeSeedCredentials() error = %v", err)
		}
		creds, _ := LoadCredentials(path)
		if creds[0].Key != "old-key" || creds[0].ClientSecret != "old-secret" {
			t.Errorf("blank fields overwrote stored secrets: %+v", creds[0])
		}
		if creds[0].ClientID != "new-cid" {
			t.Errorf("non-blank client_id not applied: %+v", creds[0])
		}
	})

	t.Run("creates the file when it does not exist", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "new-credentials.yaml")
		updated, added, err := MergeSeedCredentials(path, []SeedCredential{
			{Name: "app-001", Key: "k", ClientID: "c", ClientSecret: "s"},
		})
		if err != nil {
			t.Fatalf("MergeSeedCredentials() error = %v", err)
		}
		if updated != 0 || added != 1 {
			t.Errorf("updated=%d added=%d, want updated=0 added=1", updated, added)
		}
		creds, err := LoadCredentials(path)
		if err != nil {
			t.Fatalf("LoadCredentials() after create error = %v", err)
		}
		if len(creds) != 1 || creds[0].Name != "app-001" {
			t.Errorf("seeded file = %+v, want one app-001 entry", creds)
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
