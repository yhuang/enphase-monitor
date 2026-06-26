// Package pge fetches the user's own electric interval data from PG&E's Share My
// Data (Green Button Connect) platform and writes per-day history records.
//
// PG&E is a wholly separate data source from the Enphase Cloud API the rest of
// this application uses: it speaks OAuth2 over mutual TLS (a client certificate
// PG&E has on file), returns ESPI XML rather than JSON, and is metered at the
// single utility service meter rather than per System. It is wired onto the same
// binary through the --pge-* flags, but shares none of the Enphase OAuth,
// credential-pool, or API Budget machinery.
//
// Two data flows live here; both use the same ESPI XML parser (ParseReadings)
// and the same aggregation pipeline (AggregateReadingsByDay → WriteHistory):
//
//   - Pull: a synchronous per-UsagePoint GET against the Share My Data API for a
//     date range.  Requires OAuth2 + mTLS (client cert registered with PG&E).
//
//   - BrowserPull: opens a headed Chrome session, downloads the Green Button XML
//     via the PG&E web portal, then parses the same ESPI feed.  Requires only an
//     interactive sign-in (no cert or API registration).
//
//   - Authorize: a one-time exchange of the authorization_code from the SMD
//     click-through for a User Access Token, persisted to the token store.
//
// Settings are split across the two existing config files to match the rest of
// the app: non-secret values (subscription/usage-point IDs, cert paths, output
// dir) under a pge: block in config.yaml, and the client_id/client_secret
// secrets under a pge: block in credentials.yaml.
package pge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Default PG&E endpoints and local paths. Endpoints are the production Green
// Button Connect / DataCustodian URLs; PG&E exposes no per-customer variation.
const (
	defaultAPIBase       = "https://api.pge.com/GreenButtonConnect/espi/1_1/resource"
	defaultTokenEndpoint = "https://api.pge.com/datacustodian/oauth/v2/token"
	defaultCertFile      = "certs/client.crt"
	defaultKeyFile       = "certs/client.key"
	defaultTokenStore    = "pge-tokens.json"
	defaultOutputDir     = "pge-data"
	defaultHistoryDir    = "history"

	// Cert-renewal defaults.
	defaultEnomAPIURL  = "https://reseller.enom.com/interface.asp"
	defaultCertbotPath = "certbot"
)

// Config holds every setting needed to talk to PG&E, composed from the pge:
// blocks of config.yaml (non-secret) and credentials.yaml (secret).
type Config struct {
	// Secret, from credentials.yaml.
	ClientID     string
	ClientSecret string

	// Non-secret, from config.yaml.
	SubscriptionID string // path component after /Subscription/ in the OAuth token response
	UsagePointID   string // discovered via GET /Subscription/{id}/UsagePoint
	CertFile       string // PEM client certificate registered with PG&E (mTLS)
	KeyFile        string // PEM private key for the client certificate
	TokenStore     string // where access/refresh tokens are persisted
	OutputDir      string // directory for intermediate data files (raw downloads, etc.)
	HistoryDir     string // directory per-day pge-<date>.json records are written to
	APIBase        string // Green Button Connect resource base URL
	TokenEndpoint  string // OAuth2 token endpoint

	// Certificate renewal (Enom DNS-01 + certbot). Secrets from credentials.yaml.
	EnomUser     string
	EnomPassword string
	// Non-secret cert-renewal settings, from config.yaml pge.cert.
	CertDomain  string // FQDN the certificate is issued for, e.g. pgesmd.duragility.com
	EnomZone    string // registrable domain managed at Enom, e.g. duragility.com
	ACMEEmail   string // Let's Encrypt account email
	LEDir       string // base dir for certbot --config-dir/--work-dir/--logs-dir (user-owned, no sudo)
	CertbotPath string // certbot executable
	EnomAPIURL  string // Enom interface.asp endpoint
}

// settingsFile mirrors the pge: block in config.yaml (non-secret).
type settingsFile struct {
	PGE *struct {
		SubscriptionID string `yaml:"subscription_id"`
		UsagePointID   string `yaml:"usage_point_id"`
		CertFile       string `yaml:"cert_file"`
		KeyFile        string `yaml:"key_file"`
		TokenStore     string `yaml:"token_store"`
		OutputDir      string `yaml:"output_dir"`
		HistoryDir     string `yaml:"history_dir"`
		APIBase        string `yaml:"api_base"`
		TokenEndpoint  string `yaml:"token_endpoint"`
		Cert           *struct {
			CertDomain  string `yaml:"cert_domain"`
			EnomZone    string `yaml:"enom_zone"`
			Email       string `yaml:"email"`
			LEDir       string `yaml:"le_dir"`
			CertbotPath string `yaml:"certbot_path"`
			EnomAPIURL  string `yaml:"enom_api_url"`
		} `yaml:"cert"`
	} `yaml:"pge"`
}

// secretsFile mirrors the pge: block in credentials.yaml (secret).
type secretsFile struct {
	PGE *struct {
		ClientID     string `yaml:"client_id"`
		ClientSecret string `yaml:"client_secret"`
		EnomUser     string `yaml:"enom_user"`
		EnomPassword string `yaml:"enom_password"`
	} `yaml:"pge"`
}

// LoadConfig composes a Config from the pge: block of configFile (non-secret) and
// the pge: block of credentialsFile (secret), filling defaults for any omitted
// path/endpoint. It does not require the secrets — Authorize needs them but a
// caller may validate later — so a missing credentials file or pge: block yields
// empty ClientID/ClientSecret rather than an error.
func LoadConfig(configFile, credentialsFile string) (*Config, error) {
	cfg := &Config{
		CertFile:      defaultCertFile,
		KeyFile:       defaultKeyFile,
		TokenStore:    defaultTokenStore,
		OutputDir:     defaultOutputDir,
		HistoryDir:    defaultHistoryDir,
		APIBase:       defaultAPIBase,
		TokenEndpoint: defaultTokenEndpoint,
		LEDir:         defaultLEDir(),
		CertbotPath:   defaultCertbotPath,
		EnomAPIURL:    defaultEnomAPIURL,
	}

	var settings settingsFile
	if err := readYAML(configFile, &settings); err != nil {
		return nil, err
	}
	if settings.PGE == nil {
		return nil, fmt.Errorf("no pge: block in %s — add one (see config.yaml.example)", configFile)
	}
	s := settings.PGE
	cfg.SubscriptionID = s.SubscriptionID
	cfg.UsagePointID = s.UsagePointID
	overrideIf(&cfg.CertFile, s.CertFile)
	overrideIf(&cfg.KeyFile, s.KeyFile)
	overrideIf(&cfg.TokenStore, s.TokenStore)
	overrideIf(&cfg.OutputDir, s.OutputDir)
	overrideIf(&cfg.HistoryDir, s.HistoryDir)
	overrideIf(&cfg.APIBase, s.APIBase)
	overrideIf(&cfg.TokenEndpoint, s.TokenEndpoint)
	if c := s.Cert; c != nil {
		cfg.CertDomain = c.CertDomain
		cfg.EnomZone = c.EnomZone
		cfg.ACMEEmail = c.Email
		overrideIf(&cfg.LEDir, c.LEDir)
		overrideIf(&cfg.CertbotPath, c.CertbotPath)
		overrideIf(&cfg.EnomAPIURL, c.EnomAPIURL)
	}

	// Secrets are optional at load time: a missing credentials file is not an
	// error here (Authorize/Pull validate what they actually need).
	var secrets secretsFile
	if err := readYAML(credentialsFile, &secrets); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if secrets.PGE != nil {
		cfg.ClientID = secrets.PGE.ClientID
		cfg.ClientSecret = secrets.PGE.ClientSecret
		cfg.EnomUser = secrets.PGE.EnomUser
		cfg.EnomPassword = secrets.PGE.EnomPassword
	}

	return cfg, nil
}

// defaultLEDir returns the user-owned base directory for certbot state, so cert
// renewal needs no sudo and writes no root-owned files. Falls back to a relative
// path if the home directory cannot be determined.
func defaultLEDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".enphase-monitor", "letsencrypt")
	}
	return filepath.Join(home, ".enphase-monitor", "letsencrypt")
}

// requireAuth verifies the fields every token request needs: the client
// credentials and the mTLS certificate pair.
func (c *Config) requireAuth() error {
	if c.ClientID == "" || c.ClientSecret == "" {
		return errors.New("missing PG&E client_id/client_secret: add a pge: block to credentials.yaml (see credentials.yaml.example)")
	}
	if c.CertFile == "" || c.KeyFile == "" {
		return errors.New("missing PG&E cert_file/key_file: set them under pge: in config.yaml")
	}
	return nil
}

// requirePull verifies the fields a data pull needs on top of requireAuth: the
// subscription and usage-point identifiers.
func (c *Config) requirePull() error {
	if err := c.requireAuth(); err != nil {
		return err
	}
	if c.SubscriptionID == "" || c.UsagePointID == "" {
		return errors.New("missing PG&E subscription_id/usage_point_id: set them under pge: in config.yaml (discover usage_point_id via GET /Subscription/{id}/UsagePoint)")
	}
	return nil
}

// requireEnom verifies the fields the Enom DNS API needs: account credentials,
// the API endpoint, and the registrable zone.
func (c *Config) requireEnom() error {
	if c.EnomUser == "" || c.EnomPassword == "" {
		return errors.New("missing Enom credentials: add enom_user/enom_password under pge: in credentials.yaml")
	}
	if c.EnomZone == "" {
		return errors.New("missing pge.cert.enom_zone in config.yaml (the registrable domain managed at Enom, e.g. duragility.com)")
	}
	if c.EnomAPIURL == "" {
		return errors.New("missing Enom API URL")
	}
	return nil
}

// requireCertRenew verifies everything a full certbot renewal needs: the Enom DNS
// API access, the certificate domain and account email, and a destination for the
// renewed cert/key.
func (c *Config) requireCertRenew() error {
	if err := c.requireEnom(); err != nil {
		return err
	}
	if c.CertDomain == "" {
		return errors.New("missing pge.cert.cert_domain in config.yaml (the FQDN the certificate is issued for, e.g. pgesmd.duragility.com)")
	}
	if c.ACMEEmail == "" {
		return errors.New("missing pge.cert.email in config.yaml (Let's Encrypt account email for expiry notices)")
	}
	if c.CertFile == "" || c.KeyFile == "" {
		return errors.New("missing pge.cert_file/key_file in config.yaml (where the renewed certificate and key are written)")
	}
	return nil
}

// readYAML reads and unmarshals a YAML file into dst. A missing file is returned
// as a wrapped os.ErrNotExist so callers can treat it as "no settings yet".
func readYAML(filename string, dst any) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filename, err)
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("parsing %s: %w", filename, err)
	}
	return nil
}

// overrideIf sets *dst to v only when v is non-empty, so a blank YAML field keeps
// the default rather than clearing it.
func overrideIf(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}
