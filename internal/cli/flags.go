// flags.go handles command-line flag parsing and definitions for the cli package.
// Package comment and supported flags are documented in cache_commands.go.
package cli

import "flag"

// Flags holds all command-line flag values.
type Flags struct {
	ConfigFile          string
	Credential          string
	Init                bool
	Force               bool
	UpdateRefreshTokens bool
	All                 bool
	ClearCache          bool
	ClearCacheDate      string
	ClearAllCache       bool
	Date                string
	StartDate           string
	EndDate             string
	TrueUp              string
	NoCache             bool
	RefreshQuota        bool
	SeedCredentials     bool
	QuotaPrefix         string
	Debug               bool
	PGEWebOnly          bool
	EnphaseAPIOnly      bool
}

// ParseFlags parses command-line flags and returns the flag values.
func ParseFlags() *Flags {
	flags := &Flags{}

	flag.StringVar(&flags.ConfigFile, "config-file", "config.yaml", "Path to configuration file")
	flag.BoolVar(&flags.Init, "init", false, "Initialize: resolve system location for weather, seed monthly API quota from the developer portal into cache/monthly-quota.json, and write the weather-code legend (run once before normal use)")
	flag.BoolVar(&flags.Force, "force", false, "With --init, re-resolve location and re-sync monthly quota from the portal. With --start-date, re-fetch and overwrite Enphase history records that already exist instead of skipping them; has no effect on PG&E records, which are always overwritten.")
	flag.BoolVar(&flags.UpdateRefreshTokens, "update-refresh-tokens", false, "Run the OAuth wizard to obtain a refresh token and save it into credentials.yaml. Pass the credential name as an argument when more than one is configured: --update-refresh-tokens <name>. To re-authorize every configured credential in one run, use --update-refresh-tokens --all")
	flag.BoolVar(&flags.All, "all", false, "With --update-refresh-tokens, re-authorize every configured credential in turn (--update-refresh-tokens --all)")
	flag.BoolVar(&flags.ClearCache, "clear-cache", false, "Clear cached API responses for today's date only (preserves yesterday and earlier)")
	flag.StringVar(&flags.ClearCacheDate, "clear-cache-date", "", "Clear cached API responses whose queried date matches YYYY-MM-DD (e.g. 2026-01-19). Matches the query start date exactly: a day query for that date, plus any month/year aggregate that starts on it.")
	flag.BoolVar(&flags.ClearAllCache, "clear-all-cache", false, "Clear all cached API responses (all dates)")
	flag.StringVar(&flags.Date, "date", "", "Query a specific date/period. Formats: YYYY-MM-DD (day), YYYY-MM (month), YYYY (year). Examples: 2026-01-19, 2026-01, 2026. Defaults to today if not specified.")
	flag.StringVar(&flags.StartDate, "start-date", "", "Start date (YYYY-MM-DD) for range pulls. Fetches from both Enphase API and PG&E web by default; use --enphase-api-only or --pge-web-only to restrict the source. Skips days already written unless --force is given. Cannot be combined with --true-up or --init.")
	flag.StringVar(&flags.EndDate, "end-date", "", "End date (YYYY-MM-DD), inclusive. Defaults to yesterday.")
	flag.StringVar(&flags.TrueUp, "true-up", "", "Activate True-Up Mode. Provide your utility True-Up Start Date in YYYY-MM-DD format (e.g. 2025-01-15). Covers the 12-month True-Up Window: full calendar months from that month through yesterday (Current Period) or the last day of the 12-month window (Past True-Up Period). Takes precedence over --date.")
	flag.BoolVar(&flags.NoCache, "no-cache", false, "Bypass cache and always make live API calls. No effect in range-pull mode (--start-date), which is always live.")
	flag.BoolVar(&flags.Debug, "debug", false, "Print debug information: last run time, API budget, and cache/API decisions")
	flag.BoolVar(&flags.RefreshQuota, "refresh-quota", false, "Out-of-band refresh: read each application's monthly API hit total from the Enphase developer portal and update cache/monthly-quota.json")
	flag.BoolVar(&flags.SeedCredentials, "seed-credentials", false, "Seed credentials.yaml from the Enphase developer portal (name, key, client_id, client_secret). Use --quota-prefix to filter applications.")
	flag.StringVar(&flags.QuotaPrefix, "quota-prefix", "enphase-monitor-", "Application/credential name prefix filter (--seed-credentials) and quota sync scope (--init, --refresh-quota)")
	flag.BoolVar(&flags.PGEWebOnly, "pge-web-only", false, "Pull only from the PG&E website (Green Button browser download) for the --start-date..--end-date range. Does not require Enphase credentials.")
	flag.BoolVar(&flags.EnphaseAPIOnly, "enphase-api-only", false, "Pull only from the Enphase API for the --start-date..--end-date range. Skips all PG&E data sources (browser download and Share My Data API).")

	flag.Parse()

	// The first positional argument names the credential for --update-refresh-tokens
	// (e.g. `enphase-monitor --update-refresh-tokens app-011`). Optional when only one
	// credential is configured.
	flags.Credential = flag.Arg(0)

	return flags
}
