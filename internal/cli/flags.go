// flags.go handles command-line flag parsing and definitions for the cli package.
// Package comment and supported flags are documented in cache_commands.go.
package cli

import "flag"

// Flags holds all command-line flag values.
type Flags struct {
	ConfigFile    string
	Continuous    bool
	OAuthSetup    bool
	ClearCache    bool
	ClearAllCache bool
	TestDate      string
	TrueUp        string
	Validation    bool
	NoCache       bool
	CachedMode    bool
	Debug         bool
}

// ParseFlags parses command-line flags and returns the flag values.
func ParseFlags() *Flags {
	flags := &Flags{}

	flag.StringVar(&flags.ConfigFile, "config", "config.yaml", "Path to configuration file")
	flag.BoolVar(&flags.Continuous, "continuous", false, "Run continuously with periodic refresh (default is run once and exit)")
	flag.BoolVar(&flags.OAuthSetup, "oauth-setup", false, "Run OAuth setup wizard (one-time for developer plan)")
	flag.BoolVar(&flags.ClearCache, "clear-cache", false, "Clear cached API responses for today's date only (preserves yesterday and earlier)")
	flag.BoolVar(&flags.ClearAllCache, "clear-all-cache", false, "Clear all cached API responses (all dates)")
	flag.StringVar(&flags.TestDate, "date", "", "Query a specific date/period. Formats: YYYY-MM-DD (day), YYYY-MM (month), YYYY (year). Examples: 2026-01-19, 2026-01, 2026. Defaults to today if not specified.")
	flag.StringVar(&flags.TrueUp, "true-up", "", "Calculate true-up year energy report. Provide your utility true-up start date in YYYY-MM-DD format (e.g. 2025-01-15). Covers the 12-month True-Up Window: full calendar months from that month through yesterday (Current Period) or the last day of the 12-month window (Past True-Up Period). Takes precedence over --date.")
	flag.BoolVar(&flags.Validation, "test", false, "Validation Mode: use cache only, no live API calls, validate against expected values")
	flag.BoolVar(&flags.NoCache, "no-cache", false, "Bypass cache and always make live API calls")
	flag.BoolVar(&flags.CachedMode, "cache", false, "Serve report from cache only; list missing endpoints if cache is incomplete")
	flag.BoolVar(&flags.Debug, "debug", false, "Print debug information: last run time, API budget, and cache/API decisions")

	flag.Parse()

	return flags
}
