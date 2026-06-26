// cert.go orchestrates the unattended certificate renewal that feeds PG&E's
// mutual-TLS requirement:
//
//	RenewCert  — runs certbot (DNS-01) in a user-owned dir, wiring certbot's
//	             auth/cleanup hooks back to this same binary, then installs the
//	             renewed leaf cert + key where the PG&E pull expects them.
//	DNSHook    — the process certbot invokes as --manual-auth-hook /
//	             --manual-cleanup-hook: it sets or clears the Enom challenge TXT
//	             record and waits for DNS propagation.
//	CheckEnom  — read-only: lists the current Enom records and the computed
//	             challenge host, so the destructive SetHosts path can be trusted
//	             before it ever runs.
//
// certbot runs entirely under LEDir with no sudo, so nothing is written as root.
package pge

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RenewOptions carries the per-invocation details RenewCert needs that are not
// part of the persisted config: how to re-invoke this binary for certbot's hooks,
// and where to stream certbot's (chatty) output.
type RenewOptions struct {
	SelfPath        string    // path to this executable, for the certbot hooks (os.Executable())
	ConfigFile      string    // config.yaml path the hook subprocess should read
	CredentialsFile string    // credentials.yaml path the hook subprocess should read
	Staging         bool      // use Let's Encrypt's staging environment (for testing; not rate-limited)
	Force           bool      // renew even if the current certificate is not near expiry
	LogOutput       io.Writer // certbot stdout/stderr sink (nil = discard)
	Notify          func(string)
}

// EnomCheck is the read-only result of CheckEnom: a human-readable line per DNS
// record plus the challenge host that a renewal would create.
type EnomCheck struct {
	Records       []string
	ChallengeHost string // e.g. "_acme-challenge.pgesmd" (empty if cert_domain unset)
}

// CheckEnom fetches the current Enom record set and computes the challenge host,
// without modifying anything. Run this before the first renewal to confirm the
// API credentials work and that GetHosts parses the full zone — the guard that
// makes the later SetHosts safe.
func (c *Config) CheckEnom(ctx context.Context) (*EnomCheck, error) {
	if err := c.requireEnom(); err != nil {
		return nil, err
	}
	ec, err := newEnomClient(c)
	if err != nil {
		return nil, err
	}
	records, err := ec.getHosts(ctx)
	if err != nil {
		return nil, err
	}

	out := &EnomCheck{Records: make([]string, 0, len(records))}
	for _, r := range records {
		line := fmt.Sprintf("%-28s %-6s %s", r.HostName, r.RecordType, r.Address)
		if r.MXPref != "" {
			line += "  (MXPref " + r.MXPref + ")"
		}
		out.Records = append(out.Records, strings.TrimRight(line, " "))
	}
	if c.CertDomain != "" {
		if host, err := challengeHost(c.CertDomain, c.EnomZone); err == nil {
			out.ChallengeHost = host
		}
	}
	return out, nil
}

// DNSHook is the certbot --manual-auth-hook (cleanup=false) and
// --manual-cleanup-hook (cleanup=true). It reads CERTBOT_DOMAIN / CERTBOT_VALIDATION
// from the environment certbot sets, then adds or removes the challenge TXT
// record at Enom. On add it also waits for the record to propagate so certbot
// does not hand Let's Encrypt a name that is not yet live.
func (c *Config) DNSHook(ctx context.Context, cleanup bool, notify func(string)) error {
	if err := c.requireEnom(); err != nil {
		return err
	}
	domain := os.Getenv("CERTBOT_DOMAIN")
	if domain == "" {
		return errors.New("CERTBOT_DOMAIN is not set — this command is meant to be run by certbot as a --manual-auth-hook")
	}
	host, err := challengeHost(domain, c.EnomZone)
	if err != nil {
		return err
	}

	ec, err := newEnomClient(c)
	if err != nil {
		return err
	}
	records, err := ec.getHosts(ctx)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return errors.New("Enom GetHosts returned no records; refusing to modify the zone (run --pge-cert-check to investigate)")
	}

	if cleanup {
		report(notify, fmt.Sprintf("Removing challenge TXT %s from %s…", host, c.EnomZone))
		return ec.setHosts(ctx, removeChallengeTXT(records, host))
	}

	validation := os.Getenv("CERTBOT_VALIDATION")
	if validation == "" {
		return errors.New("CERTBOT_VALIDATION is not set")
	}
	report(notify, fmt.Sprintf("Setting challenge TXT %s.%s…", host, c.EnomZone))
	if err := ec.setHosts(ctx, upsertChallengeTXT(records, host, validation)); err != nil {
		return err
	}
	return waitForTXT(ctx, "_acme-challenge."+domain, validation, notify)
}

// RenewCert runs certbot to obtain/renew the certificate via the Enom DNS-01
// hooks, then installs the renewed leaf certificate and private key at the
// configured CertFile/KeyFile so the PG&E pull picks them up. The renewed cert is
// also left in LEDir/config/live/<domain>/ for the (future) PG&E portal upload.
func (c *Config) RenewCert(ctx context.Context, opts RenewOptions) error {
	if err := c.requireCertRenew(); err != nil {
		return err
	}
	if opts.SelfPath == "" {
		return errors.New("internal: RenewOptions.SelfPath is required (path to this executable for certbot hooks)")
	}

	configDir := filepath.Join(c.LEDir, "config")
	for _, dir := range []string{configDir, filepath.Join(c.LEDir, "work"), filepath.Join(c.LEDir, "logs")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating certbot dir %s: %w", dir, err)
		}
	}

	hook := func(cleanup bool) string {
		parts := []string{
			shellQuote(opts.SelfPath),
			"--config", shellQuote(opts.ConfigFile),
			"--credentials", shellQuote(opts.CredentialsFile),
			"--pge-cert-dns-hook",
		}
		if cleanup {
			parts = append(parts, "--cleanup")
		}
		return strings.Join(parts, " ")
	}

	args := []string{
		"certonly",
		"--non-interactive", "--agree-tos", "-m", c.ACMEEmail,
		"--manual", "--preferred-challenges", "dns",
		"--manual-auth-hook", hook(false),
		"--manual-cleanup-hook", hook(true),
		"--config-dir", configDir,
		"--work-dir", filepath.Join(c.LEDir, "work"),
		"--logs-dir", filepath.Join(c.LEDir, "logs"),
		"--cert-name", c.CertDomain,
		"-d", c.CertDomain,
	}
	if opts.Staging {
		args = append(args, "--staging")
	}
	if opts.Force {
		args = append(args, "--force-renewal")
	}

	report(opts.Notify, fmt.Sprintf("Running certbot for %s (this opens no browser; DNS is handled via the Enom API)…", c.CertDomain))
	cmd := exec.CommandContext(ctx, c.CertbotPath, args...)
	if opts.LogOutput != nil {
		cmd.Stdout = opts.LogOutput
		cmd.Stderr = opts.LogOutput
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("certbot failed: %w (see logs in %s)", err, filepath.Join(c.LEDir, "logs"))
	}

	if err := c.installRenewedCert(configDir); err != nil {
		return err
	}
	report(opts.Notify, fmt.Sprintf("Installed renewed certificate to %s and key to %s.", c.CertFile, c.KeyFile))
	return nil
}

// installRenewedCert reads the freshly issued fullchain/privkey from certbot's
// live directory, extracts the leaf certificate, and writes the cert + key to the
// configured paths the PG&E mTLS client loads.
func (c *Config) installRenewedCert(configDir string) error {
	liveDir := filepath.Join(configDir, "live", c.CertDomain)
	fullchain, err := os.ReadFile(filepath.Join(liveDir, "fullchain.pem"))
	if err != nil {
		return fmt.Errorf("reading renewed fullchain: %w", err)
	}
	privkey, err := os.ReadFile(filepath.Join(liveDir, "privkey.pem"))
	if err != nil {
		return fmt.Errorf("reading renewed private key: %w", err)
	}

	leaf, err := extractLeafCert(fullchain)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(c.CertFile), 0o755); err != nil {
		return fmt.Errorf("creating cert dir: %w", err)
	}
	if err := os.WriteFile(c.CertFile, leaf, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", c.CertFile, err)
	}
	if err := os.MkdirAll(filepath.Dir(c.KeyFile), 0o700); err != nil {
		return fmt.Errorf("creating key dir: %w", err)
	}
	if err := os.WriteFile(c.KeyFile, privkey, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", c.KeyFile, err)
	}
	return nil
}

// extractLeafCert returns the first CERTIFICATE block of a PEM bundle re-encoded
// on its own — the leaf (end-entity) certificate certbot writes first in
// fullchain.pem. PG&E's portal wants exactly this single cert, and presenting it
// (rather than the full chain) is what the mTLS client registers against. It
// operates at the PEM-block level and does not parse the DER, so a structurally
// odd-but-valid cert still round-trips unchanged.
func extractLeafCert(fullchain []byte) ([]byte, error) {
	rest := fullchain
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, errors.New("no CERTIFICATE block found in fullchain")
		}
		if block.Type == "CERTIFICATE" {
			return pem.EncodeToMemory(block), nil
		}
	}
}

// shellQuote wraps s in single quotes for safe inclusion in the hook command
// string certbot runs through a shell, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// certRenewTimeout bounds a full certbot run, including DNS propagation waits in
// the hooks. Exposed for callers that want to derive a context deadline.
const certRenewTimeout = 15 * time.Minute
