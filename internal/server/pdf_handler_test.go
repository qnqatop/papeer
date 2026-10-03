package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writePDF(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "paper.pdf")
	body := []byte("%PDF-1.4\n0123456789abcdefghij\n%%EOF\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func serve(h http.Handler, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPDFHandler_ServesFile(t *testing.T) {
	path := writePDF(t)
	var gotID int64
	h := NewPDFHandler(func(id int64) (string, error) {
		gotID = id
		return path, nil
	})

	rec := serve(h, http.MethodGet, "/api/pdf/42", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotID != 42 {
		t.Errorf("resolver got id %d, want 42", gotID)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff header")
	}
}

func TestPDFHandler_RangeRequest(t *testing.T) {
	path := writePDF(t)
	h := NewPDFHandler(func(int64) (string, error) { return path, nil })

	rec := serve(h, http.MethodGet, "/api/pdf/1", map[string]string{"Range": "bytes=0-7"})
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", rec.Code)
	}
	if got := rec.Body.String(); got != "%PDF-1.4" {
		t.Errorf("body = %q, want first 8 bytes", got)
	}
}

func TestPDFHandler_BadID(t *testing.T) {
	called := false
	h := NewPDFHandler(func(int64) (string, error) { called = true; return "", nil })
	for _, target := range []string{"/api/pdf/abc", "/api/pdf/", "/api/pdf/-1", "/api/pdf/1/../2"} {
		if rec := serve(h, http.MethodGet, target, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, rec.Code)
		}
	}
	if called {
		t.Error("resolver must not be called for an invalid id")
	}
}

func TestPDFHandler_NotFound(t *testing.T) {
	h := NewPDFHandler(func(int64) (string, error) { return "", errors.New("no download") })
	if rec := serve(h, http.MethodGet, "/api/pdf/7", nil); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestPDFHandler_Unavailable(t *testing.T) {
	if rec := serve(NewPDFHandler(nil), http.MethodGet, "/api/pdf/7", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("nil resolver: status = %d, want 503", rec.Code)
	}
	h := NewPDFHandler(func(int64) (string, error) { return "", ErrUnavailable })
	if rec := serve(h, http.MethodGet, "/api/pdf/7", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("ErrUnavailable: status = %d, want 503", rec.Code)
	}
}

func TestPDFHandler_RejectsNonGet(t *testing.T) {
	h := NewPDFHandler(func(int64) (string, error) { return writePDF(t), nil })
	if rec := serve(h, http.MethodPost, "/api/pdf/7", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
