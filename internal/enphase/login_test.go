package enphase

import (
	"testing"

	"github.com/chromedp/cdproto/network"
)

func TestCookieAppliesToHost(t *testing.T) {
	const host = "developer-v4.enphase.com"
	cases := []struct {
		domain string
		want   bool
	}{
		{"developer-v4.enphase.com", true}, // exact host
		{".enphase.com", true},             // dotted parent domain
		{"enphase.com", true},              // bare parent domain
		{".enphaseenergy.com", false},      // different registrable domain
		{"api.enphaseenergy.com", false},   // unrelated host
		{"evil.com", false},
	}
	for _, c := range cases {
		if got := cookieAppliesToHost(host, c.domain); got != c.want {
			t.Errorf("cookieAppliesToHost(%q, %q) = %v, want %v", host, c.domain, got, c.want)
		}
	}
}

func TestCookieHeaderForHost(t *testing.T) {
	cookies := []*network.Cookie{
		{Name: "user_session", Value: "abc", Domain: "developer-v4.enphase.com"},
		{Name: "_ga", Value: "1", Domain: ".enphase.com"},
		{Name: "SESSION", Value: "skip", Domain: "api.enphaseenergy.com"}, // wrong host
	}
	got := cookieHeaderForHost(cookies, "developer-v4.enphase.com")
	want := "user_session=abc; _ga=1"
	if got != want {
		t.Errorf("cookieHeaderForHost() = %q, want %q", got, want)
	}
}

func TestHostOf(t *testing.T) {
	h, err := hostOf("https://developer-v4.enphase.com")
	if err != nil || h != "developer-v4.enphase.com" {
		t.Errorf("hostOf() = %q, %v; want developer-v4.enphase.com, nil", h, err)
	}
	if _, err := hostOf("not a url"); err == nil {
		t.Error("hostOf(invalid) expected error")
	}
}
