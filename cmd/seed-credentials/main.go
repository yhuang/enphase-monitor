// Command seed-credentials fills (seeds) the application identity fields —
// name, key, client_id, client_secret — into credentials.yaml from the Enphase
// developer portal, so the main app's `--update-refresh-token --all --new-only`
// can then obtain each entry's refresh token.
//
// The Enphase portal has no management API; credentials are only in its
// session-authenticated web UI. seed-credentials opens a Chrome window so you can
// log in (MFA included), captures the session via the DevTools Protocol, and
// scrapes every application — nothing is stored. Usage:
//
//	seed-credentials                                # all apps
//	seed-credentials -name-prefix enphase-monitor-  # only matching names
//
// It MERGES into credentials.yaml: existing entries have their secrets resynced
// in place while their refresh_token is preserved, and newly-discovered apps are
// appended with an empty refresh_token. No working refresh token is ever lost.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"enphase-monitor/internal/config"
	"enphase-monitor/internal/enphase"
)

func main() {
	var (
		credsPath  = flag.String("credentials", "credentials.yaml", "Path to the credentials file to seed/merge into")
		baseURL    = flag.String("base-url", enphase.DefaultBaseURL, "Enphase developer portal base URL")
		namePrefix = flag.String("name-prefix", "", `Only include apps whose name starts with this prefix (e.g. "enphase-monitor-")`)
	)
	flag.Parse()

	seeds, err := scrapeSeeds(*baseURL, *namePrefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(seeds) == 0 {
		fmt.Fprintln(os.Stderr, "error: no credentials found to seed")
		os.Exit(1)
	}

	// Refuse to seed entries that are missing a secret field — a half-populated
	// entry would only fail later at load time.
	for _, s := range seeds {
		if s.Name == "" || s.Key == "" || s.ClientID == "" || s.ClientSecret == "" {
			fmt.Fprintf(os.Stderr, "error: incomplete credential %q (need name, key, client_id, client_secret)\n", s.Name)
			os.Exit(1)
		}
	}

	updated, added, err := config.MergeSeedCredentials(*credsPath, seeds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to write %s: %v\n", *credsPath, err)
		os.Exit(1)
	}

	fmt.Printf("Seeded %s: %d updated, %d added (%d apps scanned).\n", *credsPath, updated, added, len(seeds))
	if added > 0 {
		fmt.Println("Next: obtain refresh tokens for the new entries with")
		fmt.Println("  ./enphase-monitor --update-refresh-token --all --new-only")
	}
}

// scrapeSeeds logs into the portal interactively, scrapes every application
// (optionally filtered by namePrefix), and returns the seed credentials, printing
// per-credential progress on a single self-clearing line.
func scrapeSeeds(baseURL, namePrefix string) ([]config.SeedCredential, error) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cookie, err := enphase.LoginAndGetCookie(ctx, baseURL, func(s string) { fmt.Fprintln(os.Stderr, s) })
	if err != nil {
		return nil, err
	}

	// Report progress on a single, self-clearing line (\r + ANSI clear-to-EOL).
	printed := false
	progress := func(done, total int, name string) {
		printed = true
		fmt.Fprintf(os.Stderr, "\r\033[K[%d/%d] %s", done, total, name)
	}
	seeds, err := enphase.FetchAllAppCredentials(ctx, baseURL, cookie, namePrefix, progress)
	if printed {
		fmt.Fprintln(os.Stderr) // end the progress line before any summary
	}
	return seeds, err
}
