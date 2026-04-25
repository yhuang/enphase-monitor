// flags.go handles command-line flag parsing and definitions for the cli package.
// Package comment and supported flags are documented in cache_commands.go.
package cli

import "flag"

// Flags holds all command-line flag values.
type Flags struct {
	ConfigFile    string
	Continuous    bool
	SetupOAuth    bool
	ClearCache    bool
	ClearAllCache bool
	TestDate      string
	TrueUp        string
	TestMode      bool
	NoCache       bool
	ListCache     bool
	InspectCache  string
}

// ParseFlags parses command-line flags and returns the flag values.
func ParseFlags() *Flags {
	flags := &Flags{}

	flag.StringVar(&flags.ConfigFile, "config", "config.yaml", "Path to configuration file")
	flag.BoolVar(&flags.Continuous, "continuous", false, "Run continuously with periodic refresh (default is run once and exit)")
	flag.BoolVar(&flags.SetupOAuth, "setup-oauth", false, "Run OAuth setup wizard (one-time for developer plan)")
	flag.BoolVar(&flags.ClearCache, "clear-cache", false, "Clear cached API responses for today's date only (preserves yesterday and earlier)")
	flag.BoolVar(&flags.ClearAllCache, "clear-all-cache", false, "Clear all cached API responses (all dates)")
	flag.StringVar(&flags.TestDate, "date", "", "Query a specific date/period. Formats: YYYY-MM-DD (day), YYYY-MM (month), YYYY (year). Examples: 2026-01-19, 2026-01, 2026. Defaults to today if not specified.")
	flag.StringVar(&flags.TrueUp, "true-up", "", "Calculate true-up year energy report. Provide your utility true-up start date in YYYY-MM-DD format (e.g. 2025-01-15). Accumulates full calendar months from that month through yesterday. Takes precedence over --date.")
	flag.BoolVar(&flags.TestMode, "test", false, "Test mode: use cache only, no live API calls, validate against expected values")
	flag.BoolVar(&flags.NoCache, "no-cache", false, "Bypass cache and always make live API calls")
	flag.BoolVar(&flags.ListCache, "list-cache", false, "List all cached API responses")
	flag.StringVar(&flags.InspectCache, "inspect-cache", "", "Inspect cached responses by hash or date (YYYY-MM-DD format). Use --list-cache to see hashes.")

	flag.Parse()

	return flags
}
