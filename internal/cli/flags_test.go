package cli

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
)

// resetFlags is a test helper that resets the flag.CommandLine state.
// This ensures tests are isolated and don't affect each other.
func resetFlags(t *testing.T) {
	t.Helper()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
}

func TestParseFlags_Defaults(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd"}

	flags := ParseFlags()

	if flags.ConfigFile != "config.yaml" {
		t.Errorf("ParseFlags() ConfigFile = %v, want config.yaml", flags.ConfigFile)
	}
}

func TestParseFlags_ConfigFile(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--config-file", "custom.yaml"}

	flags := ParseFlags()

	if flags.ConfigFile != "custom.yaml" {
		t.Errorf("ParseFlags() ConfigFile = %v, want custom.yaml", flags.ConfigFile)
	}
}

func TestParseFlags_TestDate(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--date", "2026-01-15"}

	flags := ParseFlags()

	if flags.Date != "2026-01-15" {
		t.Errorf("TestDate = %v, want 2026-01-15", flags.Date)
	}
}

func TestParseFlags_MultipleFlags(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--config-file", "test.yaml", "--date", "2026-01-20"}

	flags := ParseFlags()

	if flags.ConfigFile != "test.yaml" {
		t.Errorf("ConfigFile = %v, want test.yaml", flags.ConfigFile)
	}
	if flags.Date != "2026-01-20" {
		t.Errorf("TestDate = %v, want 2026-01-20", flags.Date)
	}
}

func TestParseFlags_UpdateRefreshToken(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--update-refresh-tokens"}

	flags := ParseFlags()

	if !flags.UpdateRefreshTokens {
		t.Error("UpdateRefreshToken should be true")
	}
}

func TestParseFlags_CredentialsFile(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--update-refresh-tokens", "--credentials-file", "credentials-mobile.yaml"}

	flags := ParseFlags()

	if !flags.UpdateRefreshTokens || flags.CredentialsFile != "credentials-mobile.yaml" {
		t.Errorf("UpdateRefreshTokens=%v CredentialsFile=%q, want true and credentials-mobile.yaml", flags.UpdateRefreshTokens, flags.CredentialsFile)
	}
}

func TestParseFlags_SeedCredentials(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--seed-credentials"}

	flags := ParseFlags()

	if !flags.SeedCredentials {
		t.Error("SeedCredentials should be true")
	}
}

func TestParseFlags_ClearCache(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--clear-cache"}

	flags := ParseFlags()

	if !flags.ClearCache {
		t.Error("ClearCache should be true")
	}
}

func TestParseFlags_ClearAllCache(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--clear-all-cache"}

	flags := ParseFlags()

	if !flags.ClearAllCache {
		t.Error("ClearAllCache should be true")
	}
}

func TestParseFlags_NoCache(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--no-cache"}

	flags := ParseFlags()

	if !flags.NoCache {
		t.Error("NoCache should be true")
	}
}

func TestParseFlags_CacheManagementFlags(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--clear-cache", "--no-cache"}

	flags := ParseFlags()

	if !flags.ClearCache {
		t.Error("ClearCache should be true")
	}
	if !flags.NoCache {
		t.Error("NoCache should be true")
	}
}

func TestParseFlags_AllCacheFlags(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--clear-all-cache"}

	flags := ParseFlags()

	if !flags.ClearAllCache {
		t.Error("ClearAllCache should be true")
	}
}

func TestParseFlags_ComplexCombination(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{
		"cmd",
		"--config-file", "production.yaml",
		"--date", "2026-01-20",
		"--no-cache",
	}

	flags := ParseFlags()

	if flags.ConfigFile != "production.yaml" {
		t.Errorf("ConfigFile = %v, want production.yaml", flags.ConfigFile)
	}
	if flags.Date != "2026-01-20" {
		t.Errorf("TestDate = %v, want 2026-01-20", flags.Date)
	}
	if !flags.NoCache {
		t.Error("NoCache should be true")
	}
}

// TestPrintUsage_ListsEveryFlagOnceAndGroupsRaw guards --help: no registered flag
// may be dropped or duplicated by the grouping, and the raw-archive/verification
// flags must come under their own heading before the rest.
func TestPrintUsage_ListsEveryFlagOnceAndGroupsRaw(t *testing.T) {
	resetFlags(t)
	os.Args = []string{"enphase-monitor"}
	ParseFlags()
	var buf bytes.Buffer
	flag.CommandLine.SetOutput(&buf)
	printUsage()
	out := buf.String()

	rawHeading := strings.Index(out, "Raw archive, reprocessing and verification")
	otherHeading := strings.Index(out, "All other options:")
	if rawHeading < 0 || otherHeading < rawHeading {
		t.Fatalf("headings missing or out of order (raw=%d, other=%d)", rawHeading, otherHeading)
	}
	flag.VisitAll(func(f *flag.Flag) {
		line := "\n  -" + f.Name
		if n := strings.Count(out, line+"\n") + strings.Count(out, line+" "); n != 1 {
			t.Errorf("flag -%s appears %d times in usage, want 1", f.Name, n)
		}
	})
	for _, name := range rawFlagNames {
		if i := strings.Index(out, "\n  -"+name); i < rawHeading || i > otherHeading {
			t.Errorf("flag -%s is not under the raw-archive heading", name)
		}
	}
	if !strings.Contains(out, "bounds --raw-refetch-weather and --raw-apply-weather") {
		t.Error("--start-date/--end-date help should mention the raw weather flags they bound")
	}
}

func TestValidateMaintenance(t *testing.T) {
	cases := []struct {
		name    string
		flags   Flags
		wantErr string // substring; "" means valid
	}{
		{"none", Flags{}, ""},
		{"one command", Flags{VerifyPGERaw: true}, ""},
		{"two commands", Flags{VerifyPGERaw: true, VerifyWeatherRaw: true}, "mutually exclusive"},
		{"string flag counts", Flags{PGEReprocess: "raw/pge/x.zip", RawImportCache: true}, "--raw-import-cache, --pge-reprocess"},
		{"verify takes no range", Flags{VerifyEnphaseRaw: true, StartDate: "2025-10-06"}, "does not take --start-date/--end-date"},
		{"live check takes no range", Flags{VerifyEnphaseArchive: "2025-11-08", EndDate: "2025-11-09"}, "does not take"},
		{"apply weather takes a range", Flags{RawApplyWeather: true, StartDate: "2025-10-06", EndDate: "2025-11-09"}, ""},
		{"refetch weather end bound alone", Flags{RawRefetchWeather: true, EndDate: "2025-11-09"}, ""},
	}
	for _, c := range cases {
		err := c.flags.ValidateMaintenance()
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: error = %v, want it to contain %q", c.name, err, c.wantErr)
		}
	}
}
