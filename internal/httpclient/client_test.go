package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDoJSON_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify UA header.
		ua := r.Header.Get("User-Agent")
		if ua == "" {
			t.Error("missing User-Agent")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := New("test@example.com")
	var result map[string]string
	if err := c.DoJSON(context.Background(), srv.URL+"/test", &result); err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("result = %v", result)
	}
}

func TestDoJSON_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	c := New("")
	var result map[string]string
	err := c.DoJSON(context.Background(), srv.URL, &result)
	if err == nil {
		t.Fatal("expected error on 404")
	}
}

func TestDoJSONWithRetry_RetriesOn429(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 2 {
			w.WriteHeader(429)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer srv.Close()

	c := New("")
	var result map[string]string
	// With maxRetries=2, it should succeed on 3rd attempt.
	err := c.DoJSONWithRetry(context.Background(), srv.URL, &result, 2)
	if err != nil {
		t.Fatalf("DoJSONWithRetry: %v", err)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("attempts = %d, want 3", atomic.LoadInt32(&attempts))
	}
}

func TestDownloadFile_UARotation(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		ua := r.Header.Get("User-Agent")

		if n == 1 {
			// First attempt: script UA → return 403.
			if ua == UAFirefox || ua == UAChrome {
				t.Error("first attempt should use polite UA")
			}
			w.WriteHeader(403)
			return
		}
		if n == 2 {
			// Second attempt: Firefox UA → succeed.
			if ua != UAFirefox {
				t.Errorf("second attempt UA = %q, want Firefox", ua)
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Write([]byte("%PDF-1.4 test content"))
			return
		}
		t.Error("unexpected 3rd attempt")
	}))
	defer srv.Close()

	c := New("test@example.com")
	body, ct, err := c.DownloadFile(context.Background(), srv.URL+"/paper.pdf")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if ct != "application/pdf" {
		t.Errorf("content-type = %q", ct)
	}
	if string(body) != "%PDF-1.4 test content" {
		t.Errorf("body = %q", string(body))
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("attempts = %d, want 2", atomic.LoadInt32(&attempts))
	}
}

func TestDownloadFileWithReferer_SetsRefererOnFirstAttempt(t *testing.T) {
	var gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.4 ok"))
	}))
	defer srv.Close()

	c := New("test@example.com")
	referer := "https://cyberleninka.ru/article/n/slug"
	if _, _, err := c.DownloadFileWithReferer(context.Background(), srv.URL+"/article/n/slug/pdf", referer); err != nil {
		t.Fatalf("DownloadFileWithReferer: %v", err)
	}
	// The explicit referer must be present on the very first (polite) attempt,
	// not only the UA-rotation retries.
	if gotReferer != referer {
		t.Errorf("Referer = %q, want %q", gotReferer, referer)
	}
}

func TestDownloadFile_NoRefererOnFirstAttempt(t *testing.T) {
	// Regression guard: plain DownloadFile keeps its old behavior — no Referer
	// on the first attempt.
	var gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.4 ok"))
	}))
	defer srv.Close()

	c := New("test@example.com")
	if _, _, err := c.DownloadFile(context.Background(), srv.URL+"/paper.pdf"); err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if gotReferer != "" {
		t.Errorf("Referer = %q, want empty on first attempt", gotReferer)
	}
}

func TestPoliteUA(t *testing.T) {
	ua := PoliteUA("user@example.com")
	if ua != "papeer/1.0 (academic research; mailto:user@example.com)" {
		t.Errorf("UA = %q", ua)
	}
	ua = PoliteUA("")
	if ua != "papeer/1.0 (academic research)" {
		t.Errorf("UA empty = %q", ua)
	}
}
