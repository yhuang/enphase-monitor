// flags.go handles command-line flag parsing and definitions for the cli package.
// Package comment and supported flags are documented in cache_commands.go.
package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Flags holds all command-line flag values.
type Flags struct {
	ConfigFile           string
	CredentialsFile      string
	Init                 bool
	Force                bool
	UpdateRefreshTokens  bool
	ClearCache           bool
	ClearCacheDate       string
	ClearAllCache        bool
	Date                 string
	StartDate            string
	EndDate              string
	TrueUp               string
	NoCache              bool
	RefreshQuota         bool
	SeedCredentials      bool
	QuotaPrefix          string
	Debug                bool
	PGEWebOnly           bool
	PGEReprocess         string
	RawImportCache       bool
	RawRefetchWeather    bool
	RawApplyWeather      bool
	VerifyPGERaw         bool
	VerifyWeatherRaw     bool
	VerifyEnphaseRaw     bool
	VerifyEnphaseArchive string
	EnphaseAPIOnly       bool
}

// ParseFlags parses command-line flags and returns the flag values.
func ParseFlags() *Flags {
	flags := &Flags{}

	flag.StringVar(&flags.ConfigFile, "config-file", "config.yaml", "Path to configuration file")
	flag.BoolVar(&flags.Init, "init", false, "Initialize: resolve system location for weather, seed monthly API quota from the developer portal into cache/monthly-quota.json, and write the weather-code legend (run once before normal use)")
	flag.BoolVar(&flags.Force, "force", false, "With --init, re-resolve location and re-sync monthly quota from the portal. With --start-date, re-fetch and overwrite Enphase history records that already exist instead of skipping them; has no effect on PG&E records, which are always overwritten.")
	flag.StringVar(&flags.CredentialsFile, "credentials-file", "", "Path to the credentials file to use, overriding credentials_file in config.yaml (default credentials.yaml). E.g. --credentials-file credentials-mobile.yaml")
	flag.BoolVar(&flags.UpdateRefreshTokens, "update-refresh-tokens", false, "Run the OAuth wizard to re-authorize every credential in the credentials file (credentials.yaml, or the file named by --credentials-file) and save each refresh token back into it. Logs in once; each app's consent is approved automatically.")
	flag.BoolVar(&flags.ClearCache, "clear-cache", false, "Clear cached API responses for today's date only (preserves yesterday and earlier)")
	flag.StringVar(&flags.ClearCacheDate, "clear-cache-date", "", "Clear cached API responses whose queried date matches YYYY-MM-DD (e.g. 2026-01-19). Matches the query start date exactly: a day query for that date, plus any month/year aggregate that starts on it.")
	flag.BoolVar(&flags.ClearAllCache, "clear-all-cache", false, "Clear all cached API responses (all dates)")
	flag.StringVar(&flags.Date, "date", "", "Query a specific date/period. Formats: YYYY-MM-DD (day), YYYY-MM (month), YYYY (year). Examples: 2026-01-19, 2026-01, 2026. Defaults to today if not specified.")
	flag.StringVar(&flags.StartDate, "start-date", "", "Start date (YYYY-MM-DD) for range pulls. Fetches from both Enphase API and PG&E web by default; use --enphase-api-only or --pge-web-only to restrict the source. Skips days already written unless --force is given. Cannot be combined with --true-up or --init. Also bounds --raw-refetch-weather and --raw-apply-weather (without it they cover every day in history/).")
	flag.StringVar(&flags.EndDate, "end-date", "", "End date (YYYY-MM-DD), inclusive. Defaults to yesterday. A run that includes PG&E is capped at one year after --start-date (PG&E's export limit); the tool prints the --start-date to continue from. Also bounds --raw-refetch-weather and --raw-apply-weather (there, either bound may be given alone).")
	flag.StringVar(&flags.TrueUp, "true-up", "", "Activate True-Up Mode. Provide your utility True-Up Start Date in YYYY-MM-DD format (e.g. 2025-01-15). Covers the 12-month True-Up Window: full calendar months from that month through yesterday (Current Period) or the last day of the 12-month window (Past True-Up Period). Takes precedence over --date.")
	flag.BoolVar(&flags.NoCache, "no-cache", false, "Bypass cache and always make live API calls. No effect in range-pull mode (--start-date), which is always live.")
	flag.BoolVar(&flags.Debug, "debug", false, "Print debug information: last run time, API budget, and cache/API decisions")
	flag.BoolVar(&flags.RefreshQuota, "refresh-quota", false, "Out-of-band refresh: read each application's monthly API hit total from the Enphase developer portal and update cache/monthly-quota.json")
	flag.BoolVar(&flags.SeedCredentials, "seed-credentials", false, "Seed credentials.yaml from the Enphase developer portal (name, key, client_id, client_secret). Use --quota-prefix to filter applications.")
	flag.StringVar(&flags.QuotaPrefix, "quota-prefix", "enphase-monitor-", "Application/credential name prefix filter (--seed-credentials) and quota sync scope (--init, --refresh-quota)")
	flag.BoolVar(&flags.RawImportCache, "raw-import-cache", false, "Copy the raw Enphase API responses held in cache/ into raw/enphase/ (with their original fetch times) and record them in raw/manifest.jsonl. Offline, no API calls; safe to repeat.")
	flag.BoolVar(&flags.RawRefetchWeather, "raw-refetch-weather", false, "Re-fetch Open-Meteo weather for every day in history/ (or the --start-date..--end-date range) bypassing the weather cache, archiving each raw payload in raw/weather/ and reporting days whose values differ from the history record. No API key or quota; needs --init to have cached the location.")
	flag.BoolVar(&flags.RawApplyWeather, "raw-apply-weather", false, "Rebuild the weather block of every history/enphase-*.json record (or the --start-date..--end-date range) from the newest archived payload in raw/weather/. Offline; rewrites only records whose weather changes and never touches energy fields. Records where each day came from (rebuilt_from_raw or matches_raw, with the raw file and hash) in history/.weather-provenance.json.")
	flag.BoolVar(&flags.VerifyPGERaw, "verify-pge-raw", false, "Offline integrity check of the PG&E raw archive: re-hash raw/pge against raw/manifest.jsonl (altered, missing, unlisted files), re-derive every archived download with the current parser and diff it against history/pge-*.json. Exits non-zero on any finding. No network access.")
	flag.StringVar(&flags.VerifyEnphaseArchive, "verify-enphase-archive", "", "Live check (YYYY-MM-DD) that Enphase raw archiving works: fetches that one past day from the API (about 10 calls from the key quota), confirms each response was archived under raw/enphase with a matching manifest line and hash, and that the day's totals re-derived from the fetch match the stored history record. Never writes history/. The date must be a completed past day that already has a record.")
	flag.BoolVar(&flags.VerifyWeatherRaw, "verify-weather-raw", false, "Offline integrity check of the weather raw archive: re-hash raw/weather against raw/manifest.jsonl and confirm each history/enphase-*.json weather block equals what the newest archived payload yields. Exits non-zero on any finding. No network access.")
	flag.BoolVar(&flags.VerifyEnphaseRaw, "verify-enphase-raw", false, "Offline integrity check of the Enphase raw archive: re-hash raw/enphase against raw/manifest.jsonl, then re-derive every history/enphase-*.json day's per-system import, export, production and consumption from the archived API responses and compare. Exits non-zero on any finding. No network access, no API quota. (The live archiving check is --verify-enphase-archive.)")
	flag.StringVar(&flags.PGEReprocess, "pge-reprocess", "", "Rebuild PG&E history records from an archived raw download (e.g. raw/pge/pge-2025-10-01_2026-09-30.zip) without any network access. The date range is read from the file name. Use after a parser fix instead of re-pulling.")
	flag.BoolVar(&flags.PGEWebOnly, "pge-web-only", false, "Pull only from the PG&E website (Green Button browser download) for the --start-date..--end-date range. Does not require Enphase credentials.")
	flag.BoolVar(&flags.EnphaseAPIOnly, "enphase-api-only", false, "Pull only from the Enphase API for the --start-date..--end-date range. Skips the PG&E browser download.")

	flag.Usage = printUsage
	flag.Parse()

	return flags
}

// MaintenanceCommands returns the names (without the leading "--") of the
// raw-archive, reprocessing and verification commands that are set. Each does one
// job and exits. Most are offline; --raw-refetch-weather calls Open-Meteo (no key or
// quota) and --verify-enphase-archive calls the Enphase API. They are dispatched
// before any report mode (see ValidateMaintenance).
func (f *Flags) MaintenanceCommands() []string {
	var cmds []string
	add := func(on bool, name string) {
		if on {
			cmds = append(cmds, name)
		}
	}
	add(f.RawImportCache, "raw-import-cache")
	add(f.RawRefetchWeather, "raw-refetch-weather")
	add(f.RawApplyWeather, "raw-apply-weather")
	add(f.PGEReprocess != "", "pge-reprocess")
	add(f.VerifyPGERaw, "verify-pge-raw")
	add(f.VerifyWeatherRaw, "verify-weather-raw")
	add(f.VerifyEnphaseRaw, "verify-enphase-raw")
	add(f.VerifyEnphaseArchive != "", "verify-enphase-archive")
	return cmds
}

// ValidateMaintenance rejects ambiguous combinations of the maintenance commands
// instead of silently running whichever is dispatched first: at most one may be
// set, and only --raw-refetch-weather and --raw-apply-weather accept
// --start-date/--end-date (as an optional range over history/; either bound may
// be given alone). It returns nil when no maintenance command is set.
func (f *Flags) ValidateMaintenance() error {
	cmds := f.MaintenanceCommands()
	switch {
	case len(cmds) == 0:
		return nil
	case len(cmds) > 1:
		return fmt.Errorf("--%s are mutually exclusive; run one at a time", strings.Join(cmds, ", --"))
	}
	takesRange := f.RawRefetchWeather || f.RawApplyWeather
	if !takesRange && (f.StartDate != "" || f.EndDate != "") {
		return fmt.Errorf("--%s does not take --start-date/--end-date", cmds[0])
	}
	return nil
}

// rawFlagNames are the raw-archive, reprocessing and verification flags, listed
// together in --help under their own heading (see printUsage).
var rawFlagNames = []string{
	"raw-import-cache", "raw-refetch-weather", "raw-apply-weather", "pge-reprocess",
	"verify-pge-raw", "verify-weather-raw", "verify-enphase-raw", "verify-enphase-archive",
}

// printUsage writes --help: the raw-archive and verification flags grouped first,
// then every other flag, each group in alphabetical order. Every registered flag
// appears exactly once (the second group is "whatever is not in the first").
func printUsage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, "Usage of %s:\n", os.Args[0])

	inRaw := make(map[string]bool, len(rawFlagNames))
	for _, n := range rawFlagNames {
		inRaw[n] = true
	}
	printFlag := func(f *flag.Flag) {
		name, usage := flag.UnquoteUsage(f)
		fmt.Fprintf(out, "  -%s", f.Name)
		if name != "" {
			fmt.Fprintf(out, " %s", name)
		}
		fmt.Fprintf(out, "\n    \t%s\n", usage)
	}

	fmt.Fprintf(out, "\nRaw archive, reprocessing and verification:\n")
	fmt.Fprintf(out, "  (raw/ keeps an untouched copy of every source response; none of these spends API quota except -verify-enphase-archive)\n")
	for _, n := range rawFlagNames {
		if f := flag.Lookup(n); f != nil {
			printFlag(f)
		}
	}
	fmt.Fprintf(out, "\nAll other options:\n")
	flag.VisitAll(func(f *flag.Flag) {
		if !inRaw[f.Name] {
			printFlag(f)
		}
	})
}
