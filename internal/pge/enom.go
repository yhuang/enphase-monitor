// enom.go talks to Enom's classic command API (interface.asp) to manage the DNS
// host records of the registrable zone, so certbot's DNS-01 challenge can be
// satisfied without browser automation.
//
// IMPORTANT — SetHosts replaces the ENTIRE record set for the zone. Enom's API
// has no per-record add/delete, so updating one record means: GetHosts, splice
// the change into the full set, then SetHosts the whole thing back. A GetHosts
// that silently parsed too few records would therefore DELETE the rest of the
// zone on the next SetHosts. Two guards defend against that:
//
//   - setHosts refuses to send a record set that GetHosts reported as empty.
//   - the read-only --pge-cert-check command prints the parsed record set so it
//     can be eyeballed against the Enom dashboard before any renewal runs.
package pge

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// hostRecord is one Enom DNS host record. HostName is relative to the zone
// ("@" for the apex, "_acme-challenge.pgesmd" for a challenge). MXPref applies
// only to MX records; TTL is preserved when Enom returns it.
type hostRecord struct {
	HostName   string
	RecordType string
	Address    string
	MXPref     string
	TTL        string
}

// enomResponse is the shared envelope of an interface.asp XML response.
type enomResponse struct {
	XMLName  xml.Name `xml:"interface-response"`
	ErrCount int      `xml:"ErrCount"`
	Errors   struct {
		Inner []byte `xml:",innerxml"`
	} `xml:"errors"`
	RRPCode  int `xml:"RRPCode"`
	GetHosts struct {
		Hosts []struct {
			HostName   string `xml:"HostName"`
			RecordType string `xml:"RecordType"`
			Address    string `xml:"Address"`
			MXPref     string `xml:"MXPref"`
			TTL        string `xml:"TTL"`
		} `xml:"host"`
	} `xml:"GetHosts"`
}

// enomClient issues commands against one Enom account and zone.
type enomClient struct {
	apiURL   string
	user     string
	password string
	sld      string // second-level domain, e.g. "duragility"
	tld      string // top-level domain, e.g. "com"
	http     *http.Client
}

// newEnomClient builds a client for the configured Enom account and splits the
// zone into the SLD/TLD pair the API addresses records by.
func newEnomClient(cfg *Config) (*enomClient, error) {
	sld, tld, ok := splitZone(cfg.EnomZone)
	if !ok {
		return nil, fmt.Errorf("invalid enom_zone %q: expected a registrable domain like duragility.com", cfg.EnomZone)
	}
	return &enomClient{
		apiURL:   cfg.EnomAPIURL,
		user:     cfg.EnomUser,
		password: cfg.EnomPassword,
		sld:      sld,
		tld:      tld,
		http:     &http.Client{},
	}, nil
}

// baseParams returns the credential + addressing params common to every command.
func (e *enomClient) baseParams(command string) url.Values {
	return url.Values{
		"command":      {command},
		"uid":          {e.user},
		"pw":           {e.password},
		"sld":          {e.sld},
		"tld":          {e.tld},
		"responsetype": {"xml"},
	}
}

// do issues an interface.asp request and decodes the XML envelope, surfacing any
// API-level errors (ErrCount > 0).
func (e *enomClient) do(ctx context.Context, params url.Values) (*enomResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("enom request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("enom HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out enomResponse
	if err := xml.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parsing enom response: %w (body: %s)", err, truncate(string(body), 300))
	}
	if out.ErrCount > 0 {
		return nil, fmt.Errorf("enom API error (%s): %s", params.Get("command"), strings.TrimSpace(string(out.Errors.Inner)))
	}
	return &out, nil
}

// getHosts returns the full set of DNS host records for the zone.
func (e *enomClient) getHosts(ctx context.Context) ([]hostRecord, error) {
	out, err := e.do(ctx, e.baseParams("GetHosts"))
	if err != nil {
		return nil, err
	}
	records := make([]hostRecord, 0, len(out.GetHosts.Hosts))
	for _, h := range out.GetHosts.Hosts {
		// Enom pads the response with blank trailing host slots; skip them.
		if strings.TrimSpace(h.HostName) == "" && strings.TrimSpace(h.RecordType) == "" {
			continue
		}
		records = append(records, hostRecord{
			HostName:   h.HostName,
			RecordType: h.RecordType,
			Address:    h.Address,
			MXPref:     h.MXPref,
			TTL:        h.TTL,
		})
	}
	return records, nil
}

// setHosts replaces the zone's entire record set with records. It refuses an
// empty set, since SetHosts with no records would wipe the zone — almost
// certainly the symptom of a failed/garbled GetHosts rather than user intent.
func (e *enomClient) setHosts(ctx context.Context, records []hostRecord) error {
	if len(records) == 0 {
		return fmt.Errorf("refusing to SetHosts with an empty record set (would erase the zone); aborting")
	}
	params := e.baseParams("SetHosts")
	for i, r := range records {
		n := strconv.Itoa(i + 1)
		params.Set("HostName"+n, r.HostName)
		params.Set("RecordType"+n, r.RecordType)
		params.Set("Address"+n, r.Address)
		if r.MXPref != "" {
			params.Set("MXPref"+n, r.MXPref)
		}
		if r.TTL != "" {
			params.Set("TTL"+n, r.TTL)
		}
	}
	_, err := e.do(ctx, params)
	return err
}

// challengeHost returns the host record name (relative to the zone) for the ACME
// DNS-01 challenge of fqdn, e.g. fqdn "pgesmd.duragility.com" in zone
// "duragility.com" yields "_acme-challenge.pgesmd".
func challengeHost(fqdn, zone string) (string, error) {
	full := "_acme-challenge." + fqdn
	if full == "_acme-challenge."+zone {
		return "_acme-challenge", nil
	}
	suffix := "." + zone
	if !strings.HasSuffix(full, suffix) {
		return "", fmt.Errorf("%q is not within zone %q", fqdn, zone)
	}
	return strings.TrimSuffix(full, suffix), nil
}

// upsertChallengeTXT returns records with a single TXT record for host set to
// value: any existing TXT records at host are dropped first so re-runs don't
// accumulate stale challenge values.
func upsertChallengeTXT(records []hostRecord, host, value string) []hostRecord {
	out := removeChallengeTXT(records, host)
	return append(out, hostRecord{HostName: host, RecordType: "TXT", Address: value, TTL: "60"})
}

// removeChallengeTXT returns records with every TXT record at host removed.
func removeChallengeTXT(records []hostRecord, host string) []hostRecord {
	out := make([]hostRecord, 0, len(records))
	for _, r := range records {
		if strings.EqualFold(r.RecordType, "TXT") && strings.EqualFold(r.HostName, host) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// splitZone splits "duragility.com" into ("duragility", "com"). It returns
// ok=false for anything without exactly the host.tld shape it needs; multi-label
// public suffixes (e.g. co.uk) are out of scope for this single-domain tool.
func splitZone(zone string) (sld, tld string, ok bool) {
	zone = strings.TrimSpace(strings.ToLower(zone))
	i := strings.LastIndex(zone, ".")
	if i <= 0 || i == len(zone)-1 {
		return "", "", false
	}
	sld, tld = zone[:i], zone[i+1:]
	if strings.Contains(sld, " ") {
		return "", "", false
	}
	return sld, tld, true
}

// truncate shortens s to at most n runes for error messages.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
