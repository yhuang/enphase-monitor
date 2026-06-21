package oauth

import "testing"

func TestParseRedirect(t *testing.T) {
	const redirect = "https://localhost:8080/callback"

	t.Run("captures code from the redirect", func(t *testing.T) {
		code, authErr, matched := parseRedirect(redirect+"?code=ABC123&state=x", redirect)
		if !matched || authErr != nil || code != "ABC123" {
			t.Errorf("got code=%q err=%v matched=%v, want code=ABC123 err=nil matched=true", code, authErr, matched)
		}
	})

	t.Run("reports an OAuth error", func(t *testing.T) {
		_, authErr, matched := parseRedirect(redirect+"?error=access_denied", redirect)
		if !matched || authErr == nil {
			t.Errorf("got err=%v matched=%v, want non-nil err and matched=true", authErr, matched)
		}
	})

	t.Run("ignores unrelated URLs", func(t *testing.T) {
		for _, u := range []string{
			"https://api.enphaseenergy.com/oauth/authorize?client_id=x",
			"https://localhost:8080/callback", // redirect host but no code yet
			"https://other.example/callback?code=ZZ",
		} {
			if code, _, matched := parseRedirect(u, redirect); matched || code != "" {
				t.Errorf("parseRedirect(%q) matched unexpectedly (code=%q)", u, code)
			}
		}
	})

	t.Run("empty redirect URI never matches", func(t *testing.T) {
		if _, _, matched := parseRedirect("https://x/?code=Y", ""); matched {
			t.Error("empty redirectURI should not match")
		}
	})
}
