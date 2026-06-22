// flags.go handles command-line flag parsing and definitions for the cli package.
// Package comment and supported flags are documented in cache_commands.go.
package cli

import "flag"

// Flags holds all command-line flag values.
type Flags struct {
	ConfigFile         string
	CredentialsFile    string
	Credential         string
	Continuous         bool
	Initialize         bool
	Force              bool
	UpdateRefreshToken bool
	All                bool
	NewOnly            bool
	ClearCache         bool
	ClearCacheDate     string
	ClearAllCache      bool
	TestDate           string
	BackfillFrom       string
	TrueUp             string
	Validation         bool
	NoCache            bool
	CachedMode         bool
	RefreshQuota       bool
	SeedCredentials    bool
	QuotaNamePrefix    string
	Debug              bool
}

// ParseFlags parses command-line flags and returns the flag values.
func ParseFlags() *Flags {
	flags := &Flags{}

	flag.StringVar(&flags.ConfigFile, "config", "config.yaml", "Path to configuration file")
	flag.StringVar(&flags.CredentialsFile, "credentials", "credentials.yaml", "Path to credentials file holding the credentials: list (OAuth key/secret/refresh token per credential set)")
	flag.BoolVar(&flags.Continuous, "continuous", false, "Run continuously with periodic refresh (default is run once and exit)")
	flag.BoolVar(&flags.Initialize, "init", false, "Initialize: resolve system location for weather, seed monthly API quota from the developer portal into cache/monthly-quota.json, and write the weather-code legend (run once before normal use)")
	flag.BoolVar(&flags.Force, "force", false, "With --init, re-resolve location and re-sync monthly quota from the portal. With --backfill-from, re-fetch and overwrite history records that already exist instead of skipping them.")
	flag.BoolVar(&flags.UpdateRefreshToken, "update-refresh-tokens", false, "Run the OAuth wizard to obtain a refresh token and save it into credentials.yaml. Pass the credential name as an argument when more than one is configured: --update-refresh-tokens <name>")
	flag.BoolVar(&flags.All, "all", false, "With --update-refresh-tokens, re-authorize every configured credential in turn")
	flag.BoolVar(&flags.NewOnly, "new-only", false, "With --update-refresh-tokens --all, authorize only credentials that have no refresh_token yet (skip already-authorized ones, e.g. after seeding new apps)")
	flag.BoolVar(&flags.ClearCache, "clear-cache", false, "Clear cached API responses for today's date only (preserves yesterday and earlier)")
	flag.StringVar(&flags.ClearCacheDate, "clear-cache-date", "", "Clear cached API responses whose queried date matches YYYY-MM-DD (e.g. 2026-01-19). Matches the query start date exactly: a day query for that date, plus any month/year aggregate that starts on it.")
	flag.BoolVar(&flags.ClearAllCache, "clear-all-cache", false, "Clear all cached API responses (all dates)")
	flag.StringVar(&flags.TestDate, "date", "", "Query a specific date/period. Formats: YYYY-MM-DD (day), YYYY-MM (month), YYYY (year). Examples: 2026-01-19, 2026-01, 2026. Defaults to today if not specified.")
	flag.StringVar(&flags.BackfillFrom, "backfill-from", "", "Backfill Mode: fetch each day from this date (YYYY-MM-DD) through --date (or yesterday) with live API calls, writing one JSON record per day into history/. Skips days already written unless --force is given. Cannot be combined with --continuous, --true-up, or --init.")
	flag.StringVar(&flags.TrueUp, "true-up", "", "Activate True-Up Mode. Provide your utility True-Up Start Date in YYYY-MM-DD format (e.g. 2025-01-15). Covers the 12-month True-Up Window: full calendar months from that month through yesterday (Current Period) or the last day of the 12-month window (Past True-Up Period). Takes precedence over --date.")
	flag.BoolVar(&flags.Validation, "test", false, "Validation Mode: use cache only, no live API calls, validate against expected values")
	flag.BoolVar(&flags.NoCache, "no-cache", false, "Bypass cache and always make live API calls")
	flag.BoolVar(&flags.CachedMode, "cache", false, "Serve report from cache only; list missing endpoints if cache is incomplete")
	flag.BoolVar(&flags.Debug, "debug", false, "Print debug information: last run time, API budget, and cache/API decisions")
	flag.BoolVar(&flags.RefreshQuota, "refresh-quota", false, "Out-of-band refresh: read each application's monthly API hit total from the Enphase developer portal and update cache/monthly-quota.json")
	flag.BoolVar(&flags.SeedCredentials, "seed-credentials", false, "Seed credentials.yaml from the Enphase developer portal (name, key, client_id, client_secret). Use --name-prefix to filter applications.")
	flag.StringVar(&flags.QuotaNamePrefix, "name-prefix", "enphase-monitor-", "Application/credential name prefix filter (--seed-credentials) and quota sync scope (--init, --refresh-quota)")

	flag.Parse()

	// The first positional argument names the credential for --update-refresh-tokens
	// (e.g. `enphase-monitor --update-refresh-tokens app-011`). Optional when only one
	// credential is configured.
	flags.Credential = flag.Arg(0)

	return flags
}
