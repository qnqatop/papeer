package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsDisallowedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "127.8.9.10", "::1",
		"10.0.0.1", "172.16.5.4", "192.168.1.1", "fc00::1", "fd12:3456::1",
		"169.254.169.254", "fe80::1",
		"0.0.0.0", "::",
		"224.0.0.1", "ff02::1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1",
	}
	for _, s := range blocked {
		if !IsDisallowedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be disallowed", s)
		}
	}
	allowed := []string{"8.8.8.8", "93.184.216.34", "2606:4700:4700::1111", "172.32.0.1"}
	for _, s := range allowed {
		if IsDisallowedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestIsDisallowedHost(t *testing.T) {
	for _, h := range []string{"localhost", "LOCALHOST.", "foo.localhost", "127.0.0.1", "[::1]", "169.254.169.254", ""} {
		if !IsDisallowedHost(h) {
			t.Errorf("%q should be disallowed", h)
		}
	}
	for _, h := range []string{"example.com", "api.openalex.org", "1.1.1.1"} {
		if IsDisallowedHost(h) {
			t.Errorf("%q should be allowed", h)
		}
	}
}

func TestSSRFDialControl(t *testing.T) {
	if err := ssrfDialControl("tcp", "127.0.0.1:80", nil); !errors.Is(err, ErrBlockedDestination) {
		t.Errorf("loopback dial: err = %v, want ErrBlockedDestination", err)
	}
	if err := ssrfDialControl("tcp", "[fe80::1]:443", nil); !errors.Is(err, ErrBlockedDestination) {
		t.Errorf("link-local dial: err = %v, want ErrBlockedDestination", err)
	}
	if err := ssrfDialControl("tcp", "8.8.8.8:443", nil); err != nil {
		t.Errorf("public dial: unexpected err %v", err)
	}
}

// TestSSRFGuard_DialTimeCheck exercises the dial-time check with a hostname
// (not a literal IP) that resolves to loopback, i.e. the DNS-rebinding case
// that the URL pre-check cannot see.
func TestSSRFGuard_DialTimeCheck(t *testing.T) {
	d := &net.Dialer{Control: ssrfDialControl}
	_, err := d.DialContext(context.Background(), "tcp", "localhost:1")
	if !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("err = %v, want ErrBlockedDestination", err)
	}
}

func TestSSRFGuard_RefusesLoopback(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New("")
	c.EnableSSRFGuard()
	var out map[string]any
	if err := c.DoJSON(context.Background(), srv.URL, &out); !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("DoJSON: err = %v, want ErrBlockedDestination", err)
	}
	if _, _, err := c.DownloadFile(context.Background(), "file:///etc/passwd"); !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("DownloadFile file://: err = %v, want ErrBlockedDestination", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Errorf("server was hit %d times", hits)
	}

	// Same with a proxy configured: the literal-IP pre-check still applies.
	pc, err := NewWithProxy("", "http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	pc.EnableSSRFGuard()
	if err := pc.DoJSON(context.Background(), srv.URL, &out); !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("proxied DoJSON: err = %v, want ErrBlockedDestination", err)
	}
}

func TestCheckRedirect_StripsHostHeadersAcrossHosts(t *testing.T) {
	var gotKey atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey.Store(r.Header.Get("x-api-key"))
		w.Write([]byte(`{}`))
	}))
	defer other.Close()
	// Redirect via "localhost" so the target host differs from 127.0.0.1.
	otherURL := strings.Replace(other.URL, "127.0.0.1", "localhost", 1)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, otherURL, http.StatusFound)
	}))
	defer origin.Close()

	c := New("")
	c.SetHostHeader(strings.TrimPrefix(origin.URL, "http://"), "x-api-key", "secret")
	var out map[string]any
	if err := c.DoJSON(context.Background(), origin.URL, &out); err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if v, _ := gotKey.Load().(string); v != "" {
		t.Errorf("x-api-key leaked to redirect target: %q", v)
	}
}

func TestCheckRedirect_LimitsAndSchemes(t *testing.T) {
	var n int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := atomic.AddInt32(&n, 1)
		if r.URL.Path == "/ftp" {
			http.Redirect(w, r, "ftp://example.com/x", http.StatusFound)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("%s/loop%d", srv.URL, i), http.StatusFound)
	}))
	defer srv.Close()

	c := New("")
	if _, err := c.DoText(context.Background(), srv.URL+"/loop"); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Errorf("redirect loop: err = %v", err)
	}
	if got := atomic.LoadInt32(&n); got != maxRedirects {
		t.Errorf("hops = %d, want %d", got, maxRedirects)
	}
	if _, err := c.DoText(context.Background(), srv.URL+"/ftp"); err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Errorf("ftp redirect: err = %v", err)
	}
}

func TestReadLimited(t *testing.T) {
	big := strings.Repeat("x", 11)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(big)), ContentLength: -1}
	if _, err := readLimited(resp, 10); !errors.Is(err, ErrBodyTooLarge) {
		t.Errorf("unknown length: err = %v, want ErrBodyTooLarge", err)
	}
	resp = &http.Response{Body: io.NopCloser(strings.NewReader("")), ContentLength: 1 << 40}
	if _, err := readLimited(resp, 10); !errors.Is(err, ErrBodyTooLarge) {
		t.Errorf("declared length: err = %v, want ErrBodyTooLarge", err)
	}
	resp = &http.Response{Body: io.NopCloser(strings.NewReader("0123456789")), ContentLength: -1}
	if b, err := readLimited(resp, 10); err != nil || len(b) != 10 {
		t.Errorf("exact limit: len=%d err=%v", len(b), err)
	}
}

func TestIsTransient(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{&StatusError{Code: 503}, true},
		{fmt.Errorf("wrapped: %w", &StatusError{Code: 429}), true},
		{&StatusError{Code: 404}, false},
		{io.ErrUnexpectedEOF, true},
		{context.DeadlineExceeded, true},
		{context.Canceled, false},
		// Text-only matches must not count.
		{errors.New("HTTP 404 from doi.org/10.1503/cmaj.1"), false},
		{errors.New("PDF truncated (no %%EOF marker) at https://x.org/geoffrey.pdf"), false},
	}
	for _, tc := range cases {
		if got := IsTransient(tc.err); got != tc.want {
			t.Errorf("IsTransient(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestDoJSONWithRetry_NoRetryOnURLContaining503(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(404)
	}))
	defer srv.Close()

	c := New("")
	var out map[string]any
	err := c.DoJSONWithRetry(context.Background(), srv.URL+"/works/10.1503/geoffrey", &out, 3)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("attempts = %d, want 1 (404 must not be retried)", got)
	}
}

func TestDoJSONWithRetry_HonoursRetryAfter(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New("")
	start := time.Now()
	var out map[string]any
	if err := c.DoJSONWithRetry(context.Background(), srv.URL, &out, 2); err != nil {
		t.Fatalf("DoJSONWithRetry: %v", err)
	}
	// Default first backoff is 3s; Retry-After: 1 should shorten it.
	if el := time.Since(start); el < time.Second || el > 2500*time.Millisecond {
		t.Errorf("elapsed %v, want ~1s from Retry-After", el)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter("5"); d != 5*time.Second {
		t.Errorf("5 -> %v", d)
	}
	if d := parseRetryAfter("3600"); d != maxRetryAfter {
		t.Errorf("3600 -> %v, want cap", d)
	}
	if d := parseRetryAfter("Wed, 21 Oct 2015 07:28:00 GMT"); d != 0 {
		t.Errorf("http-date -> %v, want 0", d)
	}
}
