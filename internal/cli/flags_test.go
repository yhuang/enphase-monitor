package cli

import (
	"flag"
	"os"
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

func TestParseFlags_UpdateRefreshTokenAll(t *testing.T) {
	resetFlags(t)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--update-refresh-tokens", "--all"}

	flags := ParseFlags()

	if !flags.UpdateRefreshTokens || !flags.All {
		t.Errorf("UpdateRefreshTokens=%v All=%v, want both true", flags.UpdateRefreshTokens, flags.All)
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
