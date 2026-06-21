package oauth

import (
	"context"
	"net/http"
	"testing"
)

func TestIsLoopbackHost(t *testing.T) {
	cases := map[string]bool{
		"localhost":   true,
		"127.0.0.1":   true,
		"::1":         true,
		"0.0.0.0":     false,
		"example.com": false,
		"":            false,
	}
	for host, want := range cases {
		if got := isLoopbackHost(host); got != want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestListenForCallback(t *testing.T) {
	t.Run("loopback with port binds", func(t *testing.T) {
		ln, path, ok := listenForCallback("http://127.0.0.1:0/callback")
		if !ok {
			t.Fatal("listenForCallback() ok = false, want true for loopback with port")
		}
		defer ln.Close()
		if path != "/callback" {
			t.Errorf("path = %q, want /callback", path)
		}
	})

	t.Run("non-loopback falls back", func(t *testing.T) {
		if _, _, ok := listenForCallback("https://example.com/callback"); ok {
			t.Error("listenForCallback() ok = true, want false for non-loopback host")
		}
	})

	t.Run("missing port falls back", func(t *testing.T) {
		if _, _, ok := listenForCallback("http://localhost/callback"); ok {
			t.Error("listenForCallback() ok = true, want false when no port")
		}
	})

	t.Run("empty path defaults to root", func(t *testing.T) {
		ln, path, ok := listenForCallback("http://127.0.0.1:0")
		if !ok {
			t.Fatal("listenForCallback() ok = false, want true")
		}
		defer ln.Close()
		if path != "/" {
			t.Errorf("path = %q, want /", path)
		}
	})

	t.Run("busy port falls back", func(t *testing.T) {
		// Bind a port, then ask listenForCallback to bind the same one.
		ln, _, ok := listenForCallback("http://127.0.0.1:0/callback")
		if !ok {
			t.Fatal("setup listener failed")
		}
		defer ln.Close()
		busyURL := "http://" + ln.Addr().String() + "/callback"
		if _, _, ok := listenForCallback(busyURL); ok {
			t.Error("listenForCallback() ok = true on a busy port, want false")
		}
	})
}

func TestCaptureCodeViaCallback(t *testing.T) {
	stubBrowser := func(t *testing.T, fn func(authURL string)) {
		t.Helper()
		orig := openBrowser
		t.Cleanup(func() { openBrowser = orig })
		openBrowser = func(authURL string) error {
			go fn(authURL)
			return nil
		}
	}

	t.Run("captures code from redirect", func(t *testing.T) {
		ln, path, ok := listenForCallback("http://127.0.0.1:0/callback")
		if !ok {
			t.Fatal("listener setup failed")
		}
		callbackURL := "http://" + ln.Addr().String() + path

		// When the browser "opens", simulate the redirect back with a code.
		stubBrowser(t, func(string) {
			resp, err := http.Get(callbackURL + "?code=abc123")
			if err == nil {
				resp.Body.Close()
			}
		})

		code, err := captureCodeViaCallback(context.Background(), ln, path, "http://auth.example")
		if err != nil {
			t.Fatalf("captureCodeViaCallback() error = %v", err)
		}
		if code != "abc123" {
			t.Errorf("code = %q, want abc123", code)
		}
	})

	t.Run("surfaces error= from redirect", func(t *testing.T) {
		ln, path, ok := listenForCallback("http://127.0.0.1:0/callback")
		if !ok {
			t.Fatal("listener setup failed")
		}
		callbackURL := "http://" + ln.Addr().String() + path

		stubBrowser(t, func(string) {
			resp, err := http.Get(callbackURL + "?error=access_denied")
			if err == nil {
				resp.Body.Close()
			}
		})

		_, err := captureCodeViaCallback(context.Background(), ln, path, "http://auth.example")
		if err == nil {
			t.Fatal("captureCodeViaCallback() error = nil, want authorization error")
		}
	})

	t.Run("honors context cancellation", func(t *testing.T) {
		ln, path, ok := listenForCallback("http://127.0.0.1:0/callback")
		if !ok {
			t.Fatal("listener setup failed")
		}
		stubBrowser(t, func(string) {}) // browser opens but no redirect arrives

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := captureCodeViaCallback(ctx, ln, path, "http://auth.example"); err == nil {
			t.Error("captureCodeViaCallback() error = nil, want context cancellation error")
		}
	})
}
