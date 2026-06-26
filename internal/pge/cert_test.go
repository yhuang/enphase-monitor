package pge

import (
	"strings"
	"testing"
)

// A two-cert fullchain.pem as certbot writes it: leaf first, issuer second. The
// PEM bodies are not valid DER, but extractLeafCert works at the block level so
// it only cares about block type and ordering.
const fakeFullchain = `-----BEGIN CERTIFICATE-----
TEAFcertificateBODYline1
TEAFcertificateBODYline2
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
ISSUERcertificateBODY
-----END CERTIFICATE-----
`

func TestExtractLeafCert_TakesFirstOnly(t *testing.T) {
	leaf, err := extractLeafCert([]byte(fakeFullchain))
	if err != nil {
		t.Fatalf("extractLeafCert: %v", err)
	}
	s := string(leaf)
	if strings.Count(s, "BEGIN CERTIFICATE") != 1 {
		t.Errorf("leaf should contain exactly one CERTIFICATE block, got:\n%s", s)
	}
	if !strings.Contains(s, "TEAFcertificateBODYline1") {
		t.Errorf("leaf missing the first (end-entity) certificate body:\n%s", s)
	}
	if strings.Contains(s, "ISSUERcertificateBODY") {
		t.Errorf("leaf should not include the issuer cert:\n%s", s)
	}
}

func TestExtractLeafCert_NoCert(t *testing.T) {
	if _, err := extractLeafCert([]byte("not a pem file")); err == nil {
		t.Error("expected an error when no CERTIFICATE block is present")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/usr/local/bin/enphase-monitor", `'/usr/local/bin/enphase-monitor'`},
		{"/path/with space/cfg.yaml", `'/path/with space/cfg.yaml'`},
		{"it's", `'it'\''s'`},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
