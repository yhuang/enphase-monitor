// seed.go seeds credentials.yaml from the Enphase developer portal (no public
// management API). Used by enphase-monitor --seed-credentials.
package enphase

import (
	"context"
	"fmt"

	"enphase-monitor/internal/config"
)

// SeedCredentials logs into the developer portal, scrapes each application's
// identity fields, and merges them into credentialsPath. namePrefix filters
// applications when non-empty. status receives phase lines; progress is called
// per application (same contract as ProgressFunc). Returns updated, added, and
// scanned counts.
func SeedCredentials(ctx context.Context, credentialsPath, namePrefix string, status func(string), progress ProgressFunc) (updated, added, scanned int, err error) {
	cookie, err := LoginAndGetCookie(ctx, DefaultBaseURL, status)
	if err != nil {
		return 0, 0, 0, err
	}

	seeds, err := FetchAllAppCredentials(ctx, DefaultBaseURL, cookie, namePrefix, progress)
	if err != nil {
		return 0, 0, 0, err
	}
	if len(seeds) == 0 {
		return 0, 0, 0, fmt.Errorf("no credentials found to seed")
	}

	for _, s := range seeds {
		if s.Name == "" || s.Key == "" || s.ClientID == "" || s.ClientSecret == "" {
			return 0, 0, 0, fmt.Errorf("incomplete credential %q (need name, key, client_id, client_secret)", s.Name)
		}
	}

	updated, added, err = config.MergeSeedCredentials(credentialsPath, seeds)
	if err != nil {
		return 0, 0, 0, err
	}
	return updated, added, len(seeds), nil
}
