package pge

import "testing"

func TestChallengeHost(t *testing.T) {
	tests := []struct {
		fqdn, zone, want string
		wantErr          bool
	}{
		{"pgesmd.duragility.com", "duragility.com", "_acme-challenge.pgesmd", false},
		{"duragility.com", "duragility.com", "_acme-challenge", false},
		{"a.b.duragility.com", "duragility.com", "_acme-challenge.a.b", false},
		{"pgesmd.example.com", "duragility.com", "", true},
	}
	for _, tt := range tests {
		got, err := challengeHost(tt.fqdn, tt.zone)
		if tt.wantErr {
			if err == nil {
				t.Errorf("challengeHost(%q,%q): want error, got %q", tt.fqdn, tt.zone, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("challengeHost(%q,%q): unexpected error %v", tt.fqdn, tt.zone, err)
		}
		if got != tt.want {
			t.Errorf("challengeHost(%q,%q) = %q, want %q", tt.fqdn, tt.zone, got, tt.want)
		}
	}
}

func TestSplitZone(t *testing.T) {
	tests := []struct {
		zone, sld, tld string
		ok             bool
	}{
		{"duragility.com", "duragility", "com", true},
		{"DuraGility.COM", "duragility", "com", true},
		{"localhost", "", "", false},
		{"trailing.", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		sld, tld, ok := splitZone(tt.zone)
		if ok != tt.ok || sld != tt.sld || tld != tt.tld {
			t.Errorf("splitZone(%q) = (%q,%q,%v), want (%q,%q,%v)", tt.zone, sld, tld, ok, tt.sld, tt.tld, tt.ok)
		}
	}
}

// existingZone models a realistic record set: apex A, www CNAME, an MX, and the
// service host. The challenge splice must preserve all of these untouched.
func existingZone() []hostRecord {
	return []hostRecord{
		{HostName: "@", RecordType: "A", Address: "203.0.113.10"},
		{HostName: "www", RecordType: "CNAME", Address: "duragility.com"},
		{HostName: "@", RecordType: "MX", Address: "mail.duragility.com", MXPref: "10"},
		{HostName: "pgesmd", RecordType: "A", Address: "203.0.113.20"},
	}
}

func TestUpsertChallengeTXT_PreservesAndAdds(t *testing.T) {
	records := upsertChallengeTXT(existingZone(), "_acme-challenge.pgesmd", "validation-1")

	if got := len(records); got != 5 {
		t.Fatalf("len = %d, want 5 (4 existing + 1 TXT)", got)
	}
	// Original four records survive unchanged at the front.
	for i, orig := range existingZone() {
		if records[i] != orig {
			t.Errorf("record %d mutated: got %+v, want %+v", i, records[i], orig)
		}
	}
	txt := records[len(records)-1]
	if txt.HostName != "_acme-challenge.pgesmd" || txt.RecordType != "TXT" || txt.Address != "validation-1" {
		t.Errorf("appended TXT = %+v, want host=_acme-challenge.pgesmd type=TXT addr=validation-1", txt)
	}
}

func TestUpsertChallengeTXT_ReplacesStale(t *testing.T) {
	zone := append(existingZone(), hostRecord{HostName: "_acme-challenge.pgesmd", RecordType: "TXT", Address: "old-value"})

	records := upsertChallengeTXT(zone, "_acme-challenge.pgesmd", "new-value")

	var txtCount int
	for _, r := range records {
		if r.RecordType == "TXT" && r.HostName == "_acme-challenge.pgesmd" {
			txtCount++
			if r.Address != "new-value" {
				t.Errorf("challenge TXT Address = %q, want new-value", r.Address)
			}
		}
	}
	if txtCount != 1 {
		t.Errorf("challenge TXT count = %d, want 1 (stale replaced, not appended)", txtCount)
	}
}

func TestRemoveChallengeTXT_LeavesOtherTXT(t *testing.T) {
	zone := append(existingZone(),
		hostRecord{HostName: "@", RecordType: "TXT", Address: "v=spf1 include:_spf.google.com ~all"},
		hostRecord{HostName: "_acme-challenge.pgesmd", RecordType: "TXT", Address: "challenge"},
	)

	records := removeChallengeTXT(zone, "_acme-challenge.pgesmd")

	for _, r := range records {
		if r.HostName == "_acme-challenge.pgesmd" {
			t.Errorf("challenge record not removed: %+v", r)
		}
	}
	// The unrelated SPF TXT at the apex must survive.
	var spf bool
	for _, r := range records {
		if r.RecordType == "TXT" && r.HostName == "@" {
			spf = true
		}
	}
	if !spf {
		t.Error("apex SPF TXT was removed; cleanup must only touch the challenge host")
	}
}
