// dns.go waits for an ACME challenge TXT record to become visible in public DNS
// before letting certbot ask Let's Encrypt to validate it. Enom's nameservers
// take a little while to publish a SetHosts change; returning from the auth-hook
// too early makes Let's Encrypt query a record that isn't live yet and fail the
// whole renewal.
package pge

import (
	"context"
	"fmt"
	"net"
	"time"
)

// DNS-propagation polling bounds. The interval keeps the auth-hook responsive
// without hammering the resolver; the timeout is generous because Enom can lag.
const (
	dnsPropagationTimeout  = 8 * time.Minute
	dnsPropagationInterval = 15 * time.Second
)

// publicResolver queries Cloudflare directly rather than the system resolver, so
// results reflect authoritative DNS state instead of a stale local cache.
var publicResolver = &net.Resolver{
	PreferGo: true,
	Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "udp", "1.1.1.1:53")
	},
}

// waitForTXT polls public DNS until fqdn has a TXT record equal to want, or the
// timeout elapses. fqdn is the full challenge name, e.g.
// "_acme-challenge.pgesmd.duragility.com". notify, when non-nil, receives a
// short status line on each attempt.
func waitForTXT(ctx context.Context, fqdn, want string, notify func(string)) error {
	deadline := time.Now().Add(dnsPropagationTimeout)
	attempt := 0
	for {
		attempt++
		txts, err := publicResolver.LookupTXT(ctx, fqdn)
		if err == nil && containsValue(txts, want) {
			report(notify, fmt.Sprintf("DNS challenge for %s is live (attempt %d).", fqdn, attempt))
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for TXT %s to propagate (last lookup: %v)", dnsPropagationTimeout, fqdn, txts)
		}
		report(notify, fmt.Sprintf("waiting for DNS propagation of %s (attempt %d)…", fqdn, attempt))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(dnsPropagationInterval):
		}
	}
}

// containsValue reports whether want is among the TXT values returned.
func containsValue(txts []string, want string) bool {
	for _, t := range txts {
		if t == want {
			return true
		}
	}
	return false
}

// report is defined in the enphase package's login.go for this module; re-declare
// a local helper here to avoid a cross-package dependency.
func report(notify func(string), msg string) {
	if notify != nil {
		notify(msg)
	}
}
